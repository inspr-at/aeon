// SPDX-License-Identifier: AGPL-3.0-only

package dbtest

import (
	"context"
	"testing"
)

func TestDatabasesAreIsolated(t *testing.T) {
	ctx := context.Background()
	a := Open(t)
	b := Open(t)
	if a.Name == b.Name || a.Role == b.Role || a.URL == b.URL {
		t.Fatalf("handles overlap name=%s/%s role=%s/%s", a.Name, b.Name, a.Role, b.Role)
	}

	var got string
	if err := a.Admin.QueryRow(ctx, `SELECT current_database()`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != a.Name {
		t.Fatalf("admin database %s, want %s", got, a.Name)
	}

	var user string
	var super, bypass bool
	if err := a.App.QueryRow(ctx, `
		SELECT current_user, rolsuper, rolbypassrls
		FROM pg_roles WHERE rolname = current_user`).Scan(&user, &super, &bypass); err != nil {
		t.Fatal(err)
	}
	if user != a.Role || super || bypass {
		t.Fatalf("app role %s super=%v bypass=%v", user, super, bypass)
	}

	var owner string
	if err := a.Admin.QueryRow(ctx, `
		SELECT tableowner FROM pg_tables
		WHERE schemaname = 'public' AND tablename = 'principals'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != a.Role {
		t.Fatalf("principals owner %s, want app role %s", owner, a.Role)
	}
	var migrationsOwner string
	if err := a.Admin.QueryRow(ctx, `
		SELECT tableowner FROM pg_tables
		WHERE schemaname = 'public' AND tablename = 'schema_migrations'`).Scan(&migrationsOwner); err != nil {
		t.Fatal(err)
	}
	if migrationsOwner == a.Role {
		t.Fatal("schema_migrations must stay with the migrating user")
	}

	if _, err := a.Admin.Exec(ctx, `INSERT INTO tenants (slug, name) VALUES ('iso', 'Iso')`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := b.Admin.QueryRow(ctx, `SELECT count(*) FROM tenants WHERE slug = 'iso'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("tenant inserted in one database is visible in the other")
	}
}
