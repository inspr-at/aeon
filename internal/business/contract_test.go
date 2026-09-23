// SPDX-License-Identifier: AGPL-3.0-only

package business_test

import (
	"context"
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/jackc/pgx/v5"
)

func TestR4QuoteAcceptanceIsVersionBoundAndTenantScoped(t *testing.T) {
	ctx := context.Background()
	fresh := dbtest.Open(t)
	var tenantA, tenantB string
	for _, item := range []struct {
		slug, name string
		id         *string
	}{
		{"r4-a", "A", &tenantA}, {"r4-b", "B", &tenantB},
	} {
		if err := fresh.Admin.QueryRow(ctx, `INSERT INTO tenants(slug,name) VALUES($1,$2) RETURNING id::text`, item.slug, item.name).Scan(item.id); err != nil {
			t.Fatal(err)
		}
	}
	ids := map[string]string{}
	digest := strings.Repeat("a", 64)
	err := db.InTenant(ctx, fresh.App, tenantA, func(tx pgx.Tx) error {
		var err error
		ids["person"], err = insertID(ctx, tx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1,'person','Customer',ARRAY['customer']) RETURNING id::text`, tenantA)
		if err != nil {
			return err
		}
		for slug, prefix := range map[string]string{"cost_unit": "CU", "organisation": "ORG", "contact": "CON", "quote": "QUO"} {
			ids[slug], err = insertID(ctx, tx, `INSERT INTO node_kinds(tenant_id,slug,label,short_prefix,icon) VALUES($1,$2,$2,$3,$2) RETURNING id::text`, tenantA, slug, prefix)
			if err != nil {
				return err
			}
		}
		ids["project_kind"], err = insertID(ctx, tx, `SELECT id::text FROM node_kinds WHERE tenant_id=$1 AND slug='project'`, tenantA)
		if err != nil {
			return err
		}
		for _, n := range []struct{ name, kind, key string }{
			{"project", "project_kind", "PRJ-1"}, {"cost", "cost_unit", "CU-1"},
			{"org", "organisation", "ORG-1"}, {"contact_node", "contact", "CON-1"},
			{"quote_node", "quote", "QUO-1"},
		} {
			ids[n.name], err = insertID(ctx, tx, `INSERT INTO nodes(tenant_id,kind_id,key,title) VALUES($1,$2,$3,$4) RETURNING id::text`, tenantA, ids[n.kind], n.key, n.name)
			if err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO crm_contact_principals(tenant_id,contact_node_id,principal_id,bound_by_principal_id) VALUES($1,$2,$3,$3)`, tenantA, ids["contact_node"], ids["person"]); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO node_relations(tenant_id,source_node_id,target_node_id,type) VALUES($1,$2,$3,'contact_for')`, tenantA, ids["contact_node"], ids["org"]); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO business_quotes(tenant_id,quote_node_id,project_node_id,customer_org_node_id,current_version) VALUES($1,$2,$3,$4,1)`, tenantA, ids["quote_node"], ids["project"], ids["org"]); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO quote_versions(tenant_id,quote_node_id,version,recipient_contact_node_id,currency,title,subtotal,tax_total,total,content_sha256,created_by_principal_id) VALUES($1,$2,1,$3,'EUR','Offer',10,0,10,$4,$5)`, tenantA, ids["quote_node"], ids["contact_node"], digest, ids["person"]); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO quote_line_items(tenant_id,quote_node_id,version,position,description,cost_unit_node_id,unit,quantity,rate_amount,net_amount) VALUES($1,$2,1,0,'Work',$3,'hour',1,10,10)`, tenantA, ids["quote_node"], ids["cost"]); err != nil {
			return err
		}
		ids["event"], err = insertID(ctx, tx, `INSERT INTO events(tenant_id,actor_principal_id,type,after) VALUES($1,$2,'quote.issued','{}') RETURNING id::text`, tenantA, ids["person"])
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO quote_issues(tenant_id,quote_node_id,version,issued_by_principal_id,event_id) VALUES($1,$2,1,$3,$4)`, tenantA, ids["quote_node"], ids["person"], ids["event"]); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE business_quotes SET state='issued' WHERE tenant_id=$1 AND quote_node_id=$2`, tenantA, ids["quote_node"])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// A different digest is rejected even for the bound customer principal.
	bad := db.InTenant(ctx, fresh.App, tenantA, func(tx pgx.Tx) error {
		acceptEvent, err := insertID(ctx, tx, `INSERT INTO events(tenant_id,actor_principal_id,type,after) VALUES($1,$2,'quote.accepted','{}') RETURNING id::text`, tenantA, ids["person"])
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO quote_acceptances(tenant_id,quote_node_id,version,customer_principal_id,recipient_contact_node_id,accepted_content_sha256,event_id) VALUES($1,$2,1,$3,$4,$5,$6)`, tenantA, ids["quote_node"], ids["person"], ids["contact_node"], strings.Repeat("b", 64), acceptEvent)
		return err
	})
	if bad == nil {
		t.Fatal("stale digest accepted")
	}
	err = db.InTenant(ctx, fresh.App, tenantA, func(tx pgx.Tx) error {
		acceptEvent, err := insertID(ctx, tx, `INSERT INTO events(tenant_id,actor_principal_id,type,after) VALUES($1,$2,'quote.accepted','{}') RETURNING id::text`, tenantA, ids["person"])
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO quote_acceptances(tenant_id,quote_node_id,version,customer_principal_id,recipient_contact_node_id,accepted_content_sha256,event_id) VALUES($1,$2,1,$3,$4,$5,$6)`, tenantA, ids["quote_node"], ids["person"], ids["contact_node"], digest, acceptEvent)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		tenant string
		want   int
	}{{tenantA, 1}, {tenantB, 0}} {
		err = db.InTenant(ctx, fresh.App, tc.tenant, func(tx pgx.Tx) error {
			var got int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM quote_acceptances`).Scan(&got); err != nil {
				return err
			}
			if got != tc.want {
				t.Fatalf("tenant %s sees %d acceptances, want %d", tc.tenant, got, tc.want)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func insertID(ctx context.Context, tx pgx.Tx, sql string, args ...any) (string, error) {
	var id string
	err := tx.QueryRow(ctx, sql, args...).Scan(&id)
	return id, err
}
