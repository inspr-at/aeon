// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestExactMoneyAndManifest(t *testing.T) {
	a, e := parseDecimal("0.1000", false)
	if e != nil {
		t.Fatal(e)
	}
	b, e := parseDecimal("0.2000", false)
	if e != nil {
		t.Fatal(e)
	}
	c, e := add(a, b)
	if e != nil || c.String() != "0.3000" {
		t.Fatalf("decimal sum %s: %v", c, e)
	}
	v, e := multiply(decimal(19999), decimal(3333), 10000)
	if e != nil || v.String() != "0.6666" {
		t.Fatalf("rounded product %s: %v", v, e)
	}
	if _, e := parseDecimal("1.00001", false); e == nil {
		t.Fatal("excess precision accepted")
	}
	if got := pdfLiteral("Österreich €"); got != `\326sterreich \200` {
		t.Fatalf("PDF WinAnsi text %q", got)
	}
	p, e := ManifestPlugin()
	if e != nil {
		t.Fatal(e)
	}
	r := plugins.NewRegistry()
	if e := r.Register(p); e != nil {
		t.Fatal(e)
	}
}

func TestQuoteFlowAndGates(t *testing.T) {
	database := dbtest.Open(t)
	ctx := context.Background()
	tenantID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	reg := plugins.NewRegistry()
	for _, id := range []string{"business_costs", "business_crm"} {
		p := plugins.Plugin{Manifest: plugins.Manifest{ID: id, Version: "1", Owner: "aeon", Permissions: []string{fence.PermNodesContribute}}}
		sum, e := plugins.Digest(p)
		if e != nil {
			t.Fatal(e)
		}
		p.Manifest.DigestSHA256 = sum
		if e := reg.Register(p); e != nil {
			t.Fatal(e)
		}
	}
	quotePlugin, e := ManifestPlugin()
	if e != nil {
		t.Fatal(e)
	}
	if e := reg.Register(quotePlugin); e != nil {
		t.Fatal(e)
	}
	reg.Seal()
	ids := map[string]string{}
	e = db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO tenants(id,slug,name) VALUES($1::uuid,'quotes-test','Quotes test')`, tenantID); err != nil {
			return err
		}
		for _, p := range []struct{ key, kind, role string }{{"admin", "person", "admin"}, {"customer", "person", "customer"}, {"agent", "agent", ""}} {
			var id string
			err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,$2,$3,ARRAY[$4]::text[]) RETURNING id::text`, tenantID, p.kind, p.key, p.role).Scan(&id)
			if err != nil {
				return err
			}
			ids[p.key] = id
		}
		for slug, prefix := range map[string]string{"cost_unit": "CU", "organisation": "ORG", "contact": "CON", "quote": "QUO"} {
			var id string
			err := tx.QueryRow(ctx, `INSERT INTO node_kinds(tenant_id,slug,label,short_prefix,icon) VALUES($1::uuid,$2,$2,$3,$2) RETURNING id::text`, tenantID, slug, prefix).Scan(&id)
			if err != nil {
				return err
			}
			ids[slug+"_kind"] = id
		}
		var projectKind string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM node_kinds WHERE slug='project'`).Scan(&projectKind); err != nil {
			return err
		}
		ids["project_kind"] = projectKind
		for _, n := range []struct{ key, kind, nodeKey string }{{"project", "project_kind", "PRJ-1"}, {"org", "organisation_kind", "ORG-1"}, {"contact", "contact_kind", "CON-1"}, {"cost", "cost_unit_kind", "CU-1"}} {
			var id string
			err := tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title) VALUES($1::uuid,$2,$3::uuid,$4) RETURNING id::text`, tenantID, n.nodeKey, ids[n.kind], n.key).Scan(&id)
			if err != nil {
				return err
			}
			ids[n.key] = id
		}
		for _, rel := range []struct{ from, to, kind string }{{"org", "project", "customer_of"}, {"contact", "org", "contact_for"}} {
			if _, err := tx.Exec(ctx, `INSERT INTO node_relations(tenant_id,source_node_id,target_node_id,type) VALUES($1::uuid,$2::uuid,$3::uuid,$4)`, tenantID, ids[rel.from], ids[rel.to], rel.kind); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO crm_contact_principals(tenant_id,contact_node_id,principal_id,bound_by_principal_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`, tenantID, ids["contact"], ids["customer"], ids["admin"]); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO cost_unit_rates(tenant_id,cost_unit_node_id,unit,currency,internal_amount,bill_amount,effective_from,created_by_principal_id) VALUES($1::uuid,$2::uuid,'hour','EUR',0,19.99,'2020-01-01',$3::uuid)`, tenantID, ids["cost"], ids["admin"]); err != nil {
			return err
		}
		for _, id := range []string{"business_costs", "business_crm", PluginID} {
			p, _ := reg.Lookup(id)
			if _, err := tx.Exec(ctx, `INSERT INTO plugin_installations(tenant_id,plugin_id,version,manifest_digest_sha256,owner,enabled,permissions,updated_by_principal_id) VALUES($1::uuid,$2,$3,$4,$5,true,$6,$7::uuid)`, tenantID, id, p.Manifest.Version, p.Manifest.DigestSHA256, p.Manifest.Owner, p.Manifest.Permissions, ids["admin"]); err != nil {
				return err
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	module, e := New(database.App, reg)
	if e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	module.Mount(mux)
	as := func(actor string) tenant.Principal {
		kind := tenant.Person
		roles := []string{"admin"}
		if actor == "customer" {
			roles = []string{"customer"}
		}
		if actor == "agent" {
			kind = tenant.Agent
			roles = nil
		}
		return tenant.Principal{ID: ids[actor], TenantID: tenantID, Kind: kind, Roles: roles}
	}
	call := func(actor, method, path, body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req = req.WithContext(tenant.WithPrincipal(req.Context(), as(actor)))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var out map[string]any
		decoder := json.NewDecoder(strings.NewReader(rec.Body.String()))
		decoder.UseNumber()
		_ = decoder.Decode(&out)
		return rec.Code, out
	}
	status, q := call("admin", "POST", "/api/quotes", fmt.Sprintf(`{"title":"Offer","project_node_id":%q,"customer_org_node_id":%q}`, ids["project"], ids["org"]))
	if status != 201 {
		t.Fatalf("create %d %v", status, q)
	}
	quoteID := q["quote_node_id"].(string)
	listReq := httptest.NewRequest("GET", "/api/quotes", nil)
	listReq = listReq.WithContext(tenant.WithPrincipal(listReq.Context(), as("admin")))
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != 200 || !strings.Contains(listRec.Body.String(), quoteID) {
		t.Fatalf("list %d %s", listRec.Code, listRec.Body.String())
	}
	body := fmt.Sprintf(`{"expected_revision":1,"recipient_contact_node_id":%q,"currency":"EUR","title":"Offer","terms_markdown":"Payment in 30 days","lines":[{"description":"Work","cost_unit_node_id":%q,"unit":"hour","quantity":3,"tax_rate":0.2}]}`, ids["contact"], ids["cost"])
	status, v := call("admin", "POST", "/api/quotes/"+quoteID+"/versions", body)
	if status != 201 {
		t.Fatalf("freeze %d %v", status, v)
	}
	if v["total"] != json.Number("71.9640") {
		t.Fatalf("wrong total %v", v["total"])
	}
	digest := v["content_sha256"].(string)
	pdfReq := httptest.NewRequest("GET", "/api/quotes/"+quoteID+"/versions/1/export?format=pdf", nil)
	pdfReq = pdfReq.WithContext(tenant.WithPrincipal(pdfReq.Context(), as("admin")))
	pdfRec := httptest.NewRecorder()
	mux.ServeHTTP(pdfRec, pdfReq)
	if pdfRec.Code != 200 || !strings.HasPrefix(pdfRec.Body.String(), "%PDF-1.4") || !strings.Contains(pdfRec.Body.String(), "xref") {
		t.Fatalf("invalid PDF export: %d", pdfRec.Code)
	}
	status, _ = call("admin", "POST", "/api/quotes/"+quoteID+"/versions", body)
	if status != 409 {
		t.Fatalf("stale revision: %d", status)
	}
	status, q = call("admin", "POST", "/api/quotes/"+quoteID+"/versions/1/issue", "")
	if status != 200 || q["state"] != "issued" {
		t.Fatalf("issue %d %v", status, q)
	}
	status, _ = call("agent", "POST", "/api/quotes/"+quoteID+"/versions/1/accept", fmt.Sprintf(`{"expected_content_sha256":%q}`, digest))
	if status != 403 {
		t.Fatalf("agent acceptance %d", status)
	}
	status, _ = call("customer", "POST", "/api/quotes/"+quoteID+"/versions/1/accept", fmt.Sprintf(`{"expected_content_sha256":%q}`, strings.Repeat("b", 64)))
	if status != 409 {
		t.Fatalf("stale digest %d", status)
	}
	status, a := call("customer", "POST", "/api/quotes/"+quoteID+"/versions/1/accept", fmt.Sprintf(`{"expected_content_sha256":%q}`, digest))
	if status != 201 || a["accepted_content_sha256"] != digest {
		t.Fatalf("accept %d %v", status, a)
	}
	status, _ = call("customer", "POST", "/api/quotes/"+quoteID+"/versions/1/accept", fmt.Sprintf(`{"expected_content_sha256":%q}`, digest))
	if status != 201 {
		t.Fatalf("replay %d", status)
	}
	var count int
	e = db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM events WHERE type LIKE 'quote.%'`).Scan(&count)
	})
	if e != nil {
		t.Fatal(e)
	}
	if count != 4 {
		t.Fatalf("events %d, want 4", count)
	}
	e = db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE plugin_installations SET enabled=false WHERE plugin_id='business_crm'`)
		return err
	})
	if e != nil {
		t.Fatal(e)
	}
	status, _ = call("admin", "GET", "/api/quotes/"+quoteID, "")
	if status != 403 {
		t.Fatalf("disabled dependency permits read: %d", status)
	}
}
