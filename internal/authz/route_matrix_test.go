// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestEffectiveRouteMatrix(t *testing.T) {
	d := dbtest.Open(t)
	ctx := t.Context()
	var tid string
	if err := db.InTenant(ctx, d.App, "00000000-0000-0000-0000-000000000000", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO tenants(slug,name) VALUES('az1-matrix','AZ1 matrix') RETURNING id::text`).Scan(&tid)
	}); err != nil {
		t.Fatal(err)
	}
	people := map[string]tenant.Principal{}
	for role, classic := range map[string]string{"owner": "super_admin", "admin": "admin", "member": "member", "viewer": "viewer", "guest": "external", "customer": "customer"} {
		p := tenant.Principal{TenantID: tid, Kind: tenant.Person, Roles: []string{classic}}
		if err := d.Admin.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,'person',$2,ARRAY[$3]) RETURNING id::text`, tid, role, classic).Scan(&p.ID); err != nil {
			t.Fatal(err)
		}
		dbtest.BindLegacy(t, d, tid, p.ID)
		if role == "viewer" {
			if _, err := d.Admin.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) SELECT $1::uuid,$2::uuid,id,'workspace' FROM roles WHERE tenant_id=$1::uuid AND key='viewer'`, tid, p.ID); err != nil {
				t.Fatal(err)
			}
		}
		people[role] = p
	}
	check := func(name string, p tenant.Principal, pattern string, want bool) {
		t.Helper()
		err := RequirePattern(BindPool(tenant.WithPrincipal(ctx, p), d.App), pattern, Scope{})
		if (err == nil) != want {
			t.Errorf("%s %s: got %v, want allowed=%v", name, pattern, err, want)
		}
	}
	for _, tc := range []struct {
		role, route string
		allow       bool
	}{
		{"owner", "POST /api/nodes", true}, {"owner", "POST /api/roles", true},
		{"admin", "POST /api/roles", true}, {"admin", "POST /api/nodes", true},
		{"member", "POST /api/nodes", true}, {"member", "POST /api/kinds", false},
		{"member", "POST /api/roles", false}, {"member", "POST /api/approvals/{approvalId}/decision", true},
		{"viewer", "GET /api/nodes", true}, {"viewer", "POST /api/nodes", false},
		{"guest", "GET /api/nodes", true}, {"guest", "POST /api/nodes/{nodeId}/comments", true},
		{"guest", "GET /api/agent-keys", false},
		{"customer", "GET /api/quotes/{quoteId}/versions/{version}", true},
		{"customer", "POST /api/quotes/{quoteId}/versions/{version}/accept", true},
		{"customer", "GET /api/quotes", false},
		{"customer", "GET /api/me/profile", true},
	} {
		check(tc.role, people[tc.role], tc.route, tc.allow)
	}
	var agent tenant.Principal
	agent.TenantID, agent.Kind = tid, tenant.Agent
	if err := db.InTenant(ctx, d.App, tid, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'agent','Matrix agent') RETURNING id::text`, tid).Scan(&agent.ID); err != nil {
			return err
		}
		var roleID string
		if err := tx.QueryRow(ctx, `INSERT INTO roles(tenant_id,key,name) VALUES($1::uuid,'matrix_agent','Matrix agent') RETURNING id::text`, tid).Scan(&roleID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions(tenant_id,role_id,permission) VALUES($1::uuid,$2::uuid,'nodes.read'),($1::uuid,$2::uuid,'nodes.write')`, tid, roleID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) VALUES($1::uuid,$2::uuid,$3::uuid,'workspace')`, tid, agent.ID, roleID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	agent.KeyCreatorID = people["admin"].ID
	check("agent without scopes", agent, "GET /api/nodes", false)
	agent.Scopes = []string{"nodes.read"}
	check("agent read scope", agent, "GET /api/nodes", true)
	check("agent read scope", agent, "POST /api/nodes", false)
	agent.Scopes = []string{"nodes.read", "nodes.write"}
	check("agent write scope", agent, "POST /api/nodes", true)
	if err := db.InTenant(ctx, d.App, tid, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE role_bindings SET role_id=(SELECT id FROM roles WHERE tenant_id=$1::uuid AND key='viewer') WHERE principal_id=$2::uuid AND scope_type='workspace'`, tid, people["admin"].ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	check("creator demoted", agent, "POST /api/nodes", false)
	check("creator demoted", agent, "GET /api/nodes", true)
	if _, err := d.Admin.Exec(ctx, `UPDATE principals SET status='deactivated' WHERE id=$1::uuid`, people["viewer"].ID); err != nil {
		t.Fatal(err)
	}
	check("deactivated viewer", people["viewer"], "GET /api/nodes", false)
}
