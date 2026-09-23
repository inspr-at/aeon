// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	rlsRole     = "aeon_p02_rls"
	rlsPassword = "aeon_p02_rls"
)

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	raw := os.Getenv("AEON_TEST_DATABASE_URL")
	if raw == "" {
		t.Fatal("AEON_TEST_DATABASE_URL is not set")
	}
	return raw
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := Open(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestMigrationsApplyAndReapply(t *testing.T) {
	ctx := context.Background()
	if err := testPool(t).Ping(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := Open(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again.Close)

	var n int
	if err := again.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	names, err := migrationNames()
	if err != nil {
		t.Fatal(err)
	}
	if n != len(names) {
		t.Fatalf("schema_migrations rows = %d, want %d (one per embedded migration)", n, len(names))
	}

	var versions string
	if err := again.QueryRow(ctx, `SELECT coalesce(string_agg(version, ',' ORDER BY version), '') FROM schema_migrations`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != "0001_tenants.sql,0002_identities.sql,0003_principals.sql" {
		t.Fatalf("versions %s", versions)
	}

	var vector int
	if err := again.QueryRow(ctx, `SELECT count(*) FROM pg_extension WHERE extname = 'vector'`).Scan(&vector); err != nil {
		t.Fatal(err)
	}
	if vector != 1 {
		t.Fatal("vector extension missing")
	}

	var rls, force bool
	if err := again.QueryRow(ctx, `SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE oid = 'public.principals'::regclass`).Scan(&rls, &force); err != nil {
		t.Fatal(err)
	}
	if !rls || !force {
		t.Fatalf("rls=%v force=%v", rls, force)
	}

	var qual, check string
	if err := again.QueryRow(ctx, `
		SELECT pg_get_expr(polqual, polrelid), pg_get_expr(polwithcheck, polrelid)
		FROM pg_policy WHERE polname = 'tenant_isolation'`).Scan(&qual, &check); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(qual, "aeon.tenant_id") || !strings.Contains(check, "aeon.tenant_id") {
		t.Fatalf("policy qual=%s check=%s", qual, check)
	}

	suffix := randSuffix(t)
	var tenantID string
	if err := again.QueryRow(ctx, `INSERT INTO tenants (slug, name) VALUES ($1, 'DDL') RETURNING id::text`, "p02-ddl-"+suffix).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = again.Exec(context.Background(), `DELETE FROM principals WHERE tenant_id = $1::uuid`, tenantID)
		_, _ = again.Exec(context.Background(), `DELETE FROM identities WHERE issuer = $1`, "p02-ddl-"+suffix)
		_, _ = again.Exec(context.Background(), `DELETE FROM tenants WHERE id = $1::uuid`, tenantID)
	})

	var roles []string
	if err := again.QueryRow(ctx, `
		INSERT INTO principals (tenant_id, kind, name) VALUES ($1::uuid, 'person', 'defaults')
		RETURNING roles`, tenantID).Scan(&roles); err != nil {
		t.Fatal(err)
	}
	if len(roles) != 0 {
		t.Fatalf("default roles %#v", roles)
	}
	if _, err := again.Exec(ctx, `INSERT INTO principals (tenant_id, kind, name) VALUES ($1::uuid, 'robot', 'nope')`, tenantID); err == nil {
		t.Fatal("expected kind check to reject robot")
	}

	var ident string
	if err := again.QueryRow(ctx, `
		INSERT INTO identities (issuer, subject) VALUES ($1, 'sub') RETURNING id::text`,
		"p02-ddl-"+suffix).Scan(&ident); err != nil {
		t.Fatal(err)
	}
	if _, err := again.Exec(ctx, `
		INSERT INTO principals (tenant_id, kind, name, identity_id) VALUES ($1::uuid, 'person', 'one', $2::uuid)`,
		tenantID, ident); err != nil {
		t.Fatal(err)
	}
	if _, err := again.Exec(ctx, `
		INSERT INTO principals (tenant_id, kind, name, identity_id) VALUES ($1::uuid, 'agent', 'two', $2::uuid)`,
		tenantID, ident); err == nil {
		t.Fatal("expected partial unique index to reject a second identity link")
	}
	if _, err := again.Exec(ctx, `
		INSERT INTO principals (tenant_id, kind, name) VALUES ($1::uuid, 'agent', 'null-identity')`, tenantID); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureTenant(t *testing.T) {
	ctx := context.Background()
	if err := EnsureTenant(ctx, nil, "", "Name"); err == nil {
		t.Fatal("expected empty slug to fail")
	}
	pool := testPool(t)
	slug := "p02-ensure-" + randSuffix(t)
	if err := EnsureTenant(ctx, pool, slug, "First"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureTenant(ctx, pool, slug, "Second"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tenants WHERE slug = $1`, slug)
	})
	var name string
	var n int
	if err := pool.QueryRow(ctx, `SELECT name, count(*) OVER () FROM tenants WHERE slug = $1`, slug).Scan(&name, &n); err != nil {
		t.Fatal(err)
	}
	if name != "First" || n != 1 {
		t.Fatalf("name=%s n=%d", name, n)
	}
}

func TestRLSIsolatesPrincipals(t *testing.T) {
	ctx := context.Background()
	admin := testPool(t)
	if err := ensureRLSRole(ctx, admin); err != nil {
		t.Fatal(err)
	}

	var owner string
	if err := admin.QueryRow(ctx, `SELECT tableowner FROM pg_tables WHERE schemaname = 'public' AND tablename = 'principals'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner == rlsRole {
		if err := admin.QueryRow(ctx, `SELECT current_user`).Scan(&owner); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := admin.Exec(ctx, fmt.Sprintf(`ALTER TABLE principals OWNER TO %s`, pgx.Identifier{rlsRole}.Sanitize())); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), fmt.Sprintf(`ALTER TABLE principals OWNER TO %s`, pgx.Identifier{owner}.Sanitize()))
	})

	suffix := randSuffix(t)
	var tenantA, tenantB string
	if err := admin.QueryRow(ctx, `INSERT INTO tenants (slug, name) VALUES ($1, 'A') RETURNING id::text`, "p02-rls-a-"+suffix).Scan(&tenantA); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `INSERT INTO tenants (slug, name) VALUES ($1, 'B') RETURNING id::text`, "p02-rls-b-"+suffix).Scan(&tenantB); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DELETE FROM principals WHERE tenant_id = ANY($1::uuid[])`, []string{tenantA, tenantB})
		_, _ = admin.Exec(context.Background(), `DELETE FROM tenants WHERE id = ANY($1::uuid[])`, []string{tenantA, tenantB})
	})

	rls := rlsPool(t)
	var idA, idB string
	if err := InTenant(ctx, rls, tenantA, func(tx pgx.Tx) error {
		var setting string
		if err := tx.QueryRow(ctx, `SELECT current_setting('aeon.tenant_id', true)`).Scan(&setting); err != nil {
			return err
		}
		if setting != tenantA {
			return fmt.Errorf("tenant setting %q", setting)
		}
		return tx.QueryRow(ctx, `
			INSERT INTO principals (tenant_id, kind, name, roles)
			VALUES ($1::uuid, 'person', $2, $3) RETURNING id::text`,
			tenantA, "p02-rls-a-"+suffix, []string{"member"}).Scan(&idA)
	}); err != nil {
		t.Fatal(err)
	}
	if err := InTenant(ctx, rls, tenantB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO principals (tenant_id, kind, name)
			VALUES ($1::uuid, 'agent', $2) RETURNING id::text`,
			tenantB, "p02-rls-b-"+suffix).Scan(&idB)
	}); err != nil {
		t.Fatal(err)
	}

	var visibleA, hiddenB int
	if err := InTenant(ctx, rls, tenantA, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM principals WHERE id = $1::uuid`, idA).Scan(&visibleA); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM principals WHERE id = $1::uuid`, idB).Scan(&hiddenB)
	}); err != nil {
		t.Fatal(err)
	}
	if visibleA != 1 || hiddenB != 0 {
		t.Fatalf("tenant A sees own=%d other=%d", visibleA, hiddenB)
	}

	var visibleB int
	if err := InTenant(ctx, rls, tenantB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM principals WHERE id = $1::uuid`, idA).Scan(&visibleB)
	}); err != nil {
		t.Fatal(err)
	}
	if visibleB != 0 {
		t.Fatalf("tenant B sees tenant A principal")
	}

	err := InTenant(ctx, rls, tenantA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO principals (tenant_id, kind, name) VALUES ($1::uuid, 'person', 'sneak')`, tenantB)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "42501") {
		t.Fatalf("expected RLS denial, got %v", err)
	}

	var leaked int
	if err := rls.QueryRow(ctx, `SELECT count(*) FROM principals WHERE id = $1::uuid`, idA).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatal("principal visible without a tenant setting; set_config was not transaction-local")
	}

	var adminSees int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM principals WHERE id = ANY($1::uuid[])`, []string{idA, idB}).Scan(&adminSees); err != nil {
		t.Fatal(err)
	}
	if adminSees != 2 {
		t.Fatalf("superuser sees %d principals, want 2", adminSees)
	}
}

func ensureRLSRole(ctx context.Context, admin *pgxpool.Pool) error {
	var exists bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, rlsRole).Scan(&exists); err != nil {
		return err
	}
	stmt := `CREATE ROLE ` + rlsRole + ` WITH LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE PASSWORD '` + rlsPassword + `'`
	if exists {
		stmt = `ALTER ROLE ` + rlsRole + ` WITH LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE PASSWORD '` + rlsPassword + `'`
	}
	if _, err := admin.Exec(ctx, stmt); err != nil {
		return err
	}
	var super, bypass bool
	if err := admin.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = $1`, rlsRole).Scan(&super, &bypass); err != nil {
		return err
	}
	if super || bypass {
		return fmt.Errorf("role %s still bypasses RLS", rlsRole)
	}
	var dbname string
	if err := admin.QueryRow(ctx, `SELECT current_database()`).Scan(&dbname); err != nil {
		return err
	}
	dbIdent := pgx.Identifier{dbname}.Sanitize()
	if _, err := admin.Exec(ctx, fmt.Sprintf(`GRANT CONNECT ON DATABASE %s TO %s`, dbIdent, rlsRole)); err != nil {
		return err
	}
	if _, err := admin.Exec(ctx, `GRANT USAGE ON SCHEMA public TO `+rlsRole); err != nil {
		return err
	}
	if _, err := admin.Exec(ctx, `GRANT SELECT, INSERT, UPDATE, DELETE ON tenants, identities, principals TO `+rlsRole); err != nil {
		return err
	}
	_, err := admin.Exec(ctx, `GRANT REFERENCES ON tenants, identities TO `+rlsRole)
	return err
}

func rlsPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	raw := testDatabaseURL(t)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(rlsRole, rlsPassword)
	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	return pool
}

func randSuffix(t *testing.T) string {
	t.Helper()
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[:])
}
