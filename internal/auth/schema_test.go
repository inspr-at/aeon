// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/aeon/internal/db"
)

const coreDDL = `
CREATE TABLE tenants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issuer text NOT NULL,
    subject text NOT NULL,
    email text,
    display_name text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject)
);
CREATE TABLE principals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants (id),
    kind text NOT NULL CHECK (kind IN ('person', 'agent')),
    identity_id uuid REFERENCES identities (id),
    name text NOT NULL,
    roles text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX principals_tenant_identity ON principals (tenant_id, identity_id) WHERE identity_id IS NOT NULL;
ALTER TABLE principals ENABLE ROW LEVEL SECURITY;
ALTER TABLE principals FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON principals
    USING (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('aeon.tenant_id', true), '')::uuid);
`

const roleDDL = `
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'aeon_app') THEN
        CREATE ROLE aeon_app NOSUPERUSER NOBYPASSRLS NOINHERIT;
    ELSE
        ALTER ROLE aeon_app NOSUPERUSER NOBYPASSRLS NOINHERIT;
    END IF;
END $$;
GRANT aeon_app TO CURRENT_USER;
GRANT USAGE ON SCHEMA public TO aeon_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON tenants, identities, sessions, principals, agent_keys TO aeon_app;
ALTER TABLE principals OWNER TO aeon_app;
ALTER TABLE agent_keys OWNER TO aeon_app;
`

var (
	adminPool *pgxpool.Pool
	appPool   *pgxpool.Pool
	setupOnce sync.Once
	setupErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if adminPool != nil {
		adminPool.Close()
	}
	if appPool != nil {
		appPool.Close()
	}
	os.Exit(code)
}

func useDB(t *testing.T) {
	t.Helper()
	setupOnce.Do(func() { setupErr = setupDB() })
	if setupErr != nil {
		t.Fatalf("database: %v", setupErr)
	}
}

func setupDB() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := testDatabaseURL()
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, raw)
	if err != nil {
		return errors.New("cannot connect to aeon_p03")
	}
	defer conn.Close(ctx)

	sessions, err := os.ReadFile(filepath.Join("..", "db", "migrations", "0020_sessions.sql"))
	if err != nil {
		return err
	}
	keys, err := os.ReadFile(filepath.Join("..", "db", "migrations", "0021_agent_keys.sql"))
	if err != nil {
		return err
	}
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS agent_keys, sessions, principals, identities, tenants CASCADE`,
		coreDDL,
		string(sessions),
		string(keys),
		roleDDL,
	} {
		if err := conn.PgConn().Exec(ctx, stmt).Close(); err != nil {
			return err
		}
	}
	conn.Close(ctx)

	adminCfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		return errors.New("cannot parse aeon_p03 url")
	}
	adminCfg.MaxConns = 4
	adminPool, err = pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		return errors.New("cannot connect to aeon_p03")
	}

	appCfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		return errors.New("cannot parse aeon_p03 url")
	}
	appCfg.MaxConns = 4
	appCfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE aeon_app")
		return err
	}
	appPool, err = pgxpool.NewWithConfig(ctx, appCfg)
	if err != nil {
		return errors.New("cannot connect to aeon_p03")
	}

	var user string
	var super, bypass bool
	if err := appPool.QueryRow(ctx, `
		SELECT current_user, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user
	`).Scan(&user, &super, &bypass); err != nil {
		return err
	}
	if user != "aeon_app" || super || bypass {
		return errors.New("app connection is not the nosuperuser role aeon_app")
	}
	return nil
}

func testDatabaseURL() (string, error) {
	raw := os.Getenv("AEON_TEST_DATABASE_URL")
	if raw == "" {
		raw = "postgres://aeon:aeon@127.0.0.1:55432/aeon_p03?sslmode=disable"
	}
	u, err := url.Parse(raw)
	if err != nil || strings.TrimPrefix(u.Path, "/") != "aeon_p03" {
		return "", errors.New("AEON_TEST_DATABASE_URL must use database aeon_p03")
	}
	return raw, nil
}

func testInTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", db.TenantSetting, tenantID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func reset(t *testing.T) {
	t.Helper()
	useDB(t)
	if _, err := adminPool.Exec(t.Context(), `TRUNCATE TABLE agent_keys, sessions, principals, identities, tenants CASCADE`); err != nil {
		t.Fatalf("reset: %v", err)
	}
}

func insertTenant(t *testing.T, slug, name string) string {
	t.Helper()
	var id string
	if err := appPool.QueryRow(t.Context(), `
		INSERT INTO tenants (slug, name) VALUES ($1, $2) RETURNING id::text
	`, slug, name).Scan(&id); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	return id
}
