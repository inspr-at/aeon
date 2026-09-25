// SPDX-License-Identifier: AGPL-3.0-only

package db_test

import (
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/jackc/pgx/v5"
)

// Production runs as a table-owning NOSUPERUSER NOBYPASSRLS role, so FORCE ROW
// LEVEL SECURITY applies to it. Creating a tenant there must still seed the six
// built-in roles (the image smoke caught this failing after migration 0810).
func TestEnsureTenantSeedsBuiltinRolesUnderForcedRLS(t *testing.T) {
	d := dbtest.Open(t)
	ctx := t.Context()
	// dbtest applies migrations as a superuser, so SECURITY DEFINER functions
	// would bypass RLS; production owns them with the app role. Mirror that.
	for _, fn := range []string{"aeon_seed_builtin_roles(uuid)", "aeon_seed_builtin_roles_trigger()"} {
		if _, err := d.Admin.Exec(ctx, `ALTER FUNCTION `+fn+` OWNER TO `+pgx.Identifier{d.Role}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.EnsureTenant(ctx, d.App, "seed-rls", "Seed under RLS"); err != nil {
		t.Fatal(err)
	}
	var tenantID string
	if err := d.Admin.QueryRow(ctx, `SELECT id::text FROM tenants WHERE slug='seed-rls'`).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	var builtins int
	if err := db.InTenant(ctx, d.App, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM roles WHERE builtin`).Scan(&builtins)
	}); err != nil {
		t.Fatal(err)
	}
	if builtins != 6 {
		t.Fatalf("built-in roles seeded under forced RLS: %d, want 6", builtins)
	}
	// A second call is a no-op and must not fail either.
	if err := db.EnsureTenant(ctx, d.App, "seed-rls", "Seed under RLS"); err != nil {
		t.Fatal(err)
	}
}
