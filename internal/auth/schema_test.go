// SPDX-License-Identifier: AGPL-3.0-only

package auth

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
)

var (
	adminPool *pgxpool.Pool
	appPool   *pgxpool.Pool
	appRole   string
	testDB    *dbtest.DB
	setupOnce sync.Once
	setupErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if testDB != nil {
		if err := testDB.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "dbtest cleanup: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	handle, err := dbtest.New(ctx)
	if err != nil {
		return err
	}
	testDB = handle
	adminPool = handle.Admin
	appPool = handle.App
	appRole = handle.Role
	return nil
}

func testInTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string, fn func(pgx.Tx) error) error {
	return db.InTenant(ctx, pool, tenantID, fn)
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
