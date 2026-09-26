// SPDX-License-Identifier: AGPL-3.0-only

package db_test

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/dbtest"
)

// Every migration from 0500 onward runs against existing tenant data under
// the same non-bypass owner used in production. A later tenant insert also
// exercises the SECURITY DEFINER role seed installed by 0811.
func TestDataMigrationsAsAppOwner(t *testing.T) {
	ctx := t.Context()
	d, err := dbtest.NewUnmigrated(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Errorf("dbtest cleanup: %v", err)
		}
	})

	var tenants []string
	err = db.MigrateWithHook(ctx, d.App, func(name string) error {
		if name != "0500_harness.sql" {
			return nil
		}
		for i := 1; i <= 2; i++ {
			var tenantID string
			if err := d.App.QueryRow(ctx, `INSERT INTO tenants(slug,name) VALUES($1,$2) RETURNING id::text`,
				fmt.Sprintf("dbtest-upgrade-%d", i), fmt.Sprintf("Upgrade %d", i)).Scan(&tenantID); err != nil {
				return fmt.Errorf("seed tenant %d: %w", i, err)
			}
			tenants = append(tenants, tenantID)
			if err := db.InTenant(dbtest.Seed(ctx), d.App, tenantID, func(tx pgx.Tx) error {
				var actor, project, ticket string
				if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles)
					VALUES($1,'person','Owner',ARRAY['super_admin']) RETURNING id::text`, tenantID).Scan(&actor); err != nil {
					return err
				}
				if err := tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title)
					SELECT $1,'PRJ-1',id,'Project' FROM node_kinds WHERE tenant_id=$1 AND slug='project'
					RETURNING id::text`, tenantID).Scan(&project); err != nil {
					return err
				}
				if err := tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title,parent_id,state)
					SELECT $1,'TKT-1',id,'Ticket',$2::uuid,'in-progress' FROM node_kinds
					WHERE tenant_id=$1 AND slug='ticket' RETURNING id::text`, tenantID, project).Scan(&ticket); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, `INSERT INTO events(tenant_id,actor_principal_id,node_id,type,after)
					VALUES($1,$2::uuid,$3::uuid,'node.created',jsonb_build_object('id',$3::text))`, tenantID, actor, ticket)
				return err
			}); err != nil {
				return fmt.Errorf("seed tenant %d data: %w", i, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tenants) != 2 {
		t.Fatalf("seeded %d tenants, want 2", len(tenants))
	}
	for _, tenantID := range tenants {
		var roles, owners, nodes, events, normalized, refs int
		if err := d.Admin.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM roles WHERE tenant_id=$1 AND builtin),
			(SELECT count(*) FROM role_bindings b JOIN roles r ON (r.tenant_id,r.id)=(b.tenant_id,b.role_id)
			 WHERE b.tenant_id=$1 AND r.key='owner'),
			(SELECT count(*) FROM nodes WHERE tenant_id=$1),
			(SELECT count(*) FROM events WHERE tenant_id=$1),
			(SELECT count(*) FROM nodes n WHERE n.tenant_id=$1 AND n.key='TKT-1'
			 AND n.state='in_progress' AND n.project_id=(SELECT id FROM nodes WHERE tenant_id=$1 AND key='PRJ-1')),
			(SELECT count(*) FROM events e JOIN nodes n ON n.tenant_id=e.tenant_id AND n.id=e.node_id
			 WHERE e.tenant_id=$1 AND e.type='node.created' AND e.node_refs @> ARRAY[n.id])`, tenantID).Scan(&roles, &owners, &nodes, &events, &normalized, &refs); err != nil {
			t.Fatal(err)
		}
		if roles != 6 || owners != 1 || nodes != 2 || events < 2 || normalized != 1 || refs != 1 {
			t.Fatalf("tenant %s after migrations: roles=%d owners=%d nodes=%d events=%d normalized=%d refs=%d",
				tenantID, roles, owners, nodes, events, normalized, refs)
		}
	}

	var newTenant string
	if err := d.App.QueryRow(ctx, `INSERT INTO tenants(slug,name) VALUES('dbtest-after-upgrade','After upgrade') RETURNING id::text`).Scan(&newTenant); err != nil {
		t.Fatalf("new tenant under app role: %v", err)
	}
	var builtins int
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM roles WHERE tenant_id=$1 AND builtin`, newTenant).Scan(&builtins); err != nil {
		t.Fatal(err)
	}
	if builtins != 6 {
		t.Fatalf("new tenant built-in roles=%d, want 6", builtins)
	}
}
