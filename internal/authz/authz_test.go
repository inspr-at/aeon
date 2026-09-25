// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"errors"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestRegistryAndBuiltins(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Registry {
		if seen[p.Key] || p.Key == "" || p.Group == "" || p.Description == "" || len(p.GrantableAt) == 0 {
			t.Fatalf("bad permission %+v", p)
		}
		if p.Risk != "low" && p.Risk != "medium" && p.Risk != "high" {
			t.Fatalf("risk %+v", p)
		}
		seen[p.Key] = true
	}
	cases := []struct {
		role, permission string
		allow            bool
	}{
		{"owner", "ownership.transfer", true}, {"admin", "ownership.transfer", false},
		{"admin", "roles.manage", true}, {"member", "nodes.write", true},
		{"member", "roles.manage", false}, {"viewer", "nodes.read", true},
		{"member", "plugins.read", true}, {"member", "plugins.manage", false},
		{"member", "quotes.portal_accept", false},
		{"viewer", "nodes.write", false}, {"guest", "comments.write", true},
		{"guest", "nodes.write", false}, {"guest", "keys.read", false},
		{"customer", "quotes.portal_read", true},
		{"customer", "quotes.read", false},
		{"customer", "quotes.issue", false},
	}
	for _, tc := range cases {
		got, _ := BuiltinPermissions(tc.role)
		if contains(got, tc.permission) != tc.allow {
			t.Errorf("%s %s: want %v", tc.role, tc.permission, tc.allow)
		}
	}
}

func TestLegacyMappingAndOwnerProtection(t *testing.T) {
	d := dbtest.Open(t)
	ctx := t.Context()
	var tid string
	err := db.InTenant(ctx, d.App, "00000000-0000-0000-0000-000000000000", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO tenants(slug,name) VALUES('az1-fixture','AZ1 fixture') RETURNING id::text`).Scan(&tid)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.InTenant(ctx, d.App, tid, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE roles SET name='Changed' WHERE tenant_id=$1::uuid AND key='owner'`, tid)
		return err
	})
	if err == nil {
		t.Fatal("built-in role changed in database")
	}
	ids := map[string]string{}
	for _, role := range []string{"super_admin", "admin", "member", "reviewer", "external", "customer", "system"} {
		err = db.InTenant(ctx, d.App, tid, func(tx pgx.Tx) error {
			kind := "person"
			if role == "system" {
				kind = "agent"
			}
			var id string
			if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,$2,$3,ARRAY[$3]) RETURNING id::text`, tid, kind, role).Scan(&id); err != nil {
				return err
			}
			ids[role] = id
			_, err := tx.Exec(ctx, `SELECT aeon_bind_legacy_principal($1::uuid,$2::uuid)`, tid, id)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	expected := map[string]string{"super_admin": "owner", "admin": "admin", "member": "member", "reviewer": "member", "external": "guest", "customer": "customer"}
	err = db.InTenant(ctx, d.App, tid, func(tx pgx.Tx) error {
		for old, want := range expected {
			var got string
			if err := tx.QueryRow(ctx, `SELECT r.key FROM role_bindings b JOIN roles r ON r.tenant_id=b.tenant_id AND r.id=b.role_id WHERE b.principal_id=$1::uuid`, ids[old]).Scan(&got); err != nil {
				return err
			}
			if got != want {
				t.Errorf("%s mapped to %s, want %s", old, got, want)
			}
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM role_bindings WHERE principal_id=$1::uuid`, ids["system"]).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Error("service principal received a binding")
		}
		// An existing binding survives a changed legacy label and a rerun.
		if _, err := tx.Exec(ctx, `UPDATE principals SET roles=ARRAY['customer'] WHERE id=$1::uuid`, ids["admin"]); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT aeon_bind_legacy_principal($1::uuid,$2::uuid)`, tid, ids["admin"]); err != nil {
			return err
		}
		var got string
		if err := tx.QueryRow(ctx, `SELECT r.key FROM role_bindings b JOIN roles r ON r.tenant_id=b.tenant_id AND r.id=b.role_id WHERE b.principal_id=$1::uuid`, ids["admin"]).Scan(&got); err != nil {
			return err
		}
		if got != "admin" {
			t.Errorf("rerun replaced binding with %s", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p := tenant.Principal{ID: ids["super_admin"], TenantID: tid, Kind: tenant.Person}
	check := func(permission string, scopes []string, want bool) {
		t.Helper()
		p.Scopes = scopes
		err := Require(BindPool(tenant.WithPrincipal(ctx, p), d.App), permission, Scope{})
		if (err == nil) != want {
			t.Errorf("%s scopes %v: %v", permission, scopes, err)
		}
	}
	check("ownership.transfer", nil, true)
	p.Kind = tenant.Agent
	check("nodes.read", nil, false)
	check("nodes.read", []string{"nodes.read"}, true)
	err = db.InTenant(ctx, d.App, tid, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE principals SET status='deactivated' WHERE id=$1::uuid`, ids["super_admin"])
		return err
	})
	if err == nil {
		t.Fatal("last active owner deactivated")
	}
	if !errors.Is(err, ErrForbidden) { // PostgreSQL constraint error is expected.
		t.Logf("last owner guard: %v", err)
	}
	err = db.InTenant(ctx, d.App, tid, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE role_bindings SET role_id=(SELECT id FROM roles WHERE tenant_id=$1::uuid AND key='owner')
		  WHERE tenant_id=$1::uuid AND principal_id=$2::uuid`, tid, ids["admin"])
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE principals SET status='deactivated' WHERE id=$1::uuid`, ids["super_admin"])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	p.Kind = tenant.Person
	check("nodes.read", nil, false)
}
