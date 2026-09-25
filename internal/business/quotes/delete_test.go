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
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

type quoteFixture struct {
	t        *testing.T
	database *dbtest.DB
	tenantID string
	ids      map[string]string
	mux      *http.ServeMux
}

func newQuoteFixture(t *testing.T, tenantID, slug string) *quoteFixture {
	t.Helper()
	database := dbtest.Open(t)
	ctx := context.Background()
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
		if _, err := tx.Exec(ctx, `INSERT INTO tenants(id,slug,name) VALUES($1::uuid,$2,'Quote delete test')`, tenantID, slug); err != nil {
			return err
		}
		for _, p := range []struct{ key, role string }{{"admin", "admin"}, {"member", "member"}} {
			var id string
			if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,'person',$2,ARRAY[$3]::text[]) RETURNING id::text`, tenantID, p.key, p.role).Scan(&id); err != nil {
				return err
			}
			ids[p.key] = id
		}
		for slug, prefix := range map[string]string{"cost_unit": "CU", "organisation": "ORG", "contact": "CON", "quote": "QUO"} {
			var id string
			if err := tx.QueryRow(ctx, `INSERT INTO node_kinds(tenant_id,slug,label,short_prefix,icon) VALUES($1::uuid,$2,$2,$3,$2) RETURNING id::text`, tenantID, slug, prefix).Scan(&id); err != nil {
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
			if err := tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title) VALUES($1::uuid,$2,$3::uuid,$4) RETURNING id::text`, tenantID, n.nodeKey, ids[n.kind], n.key).Scan(&id); err != nil {
				return err
			}
			ids[n.key] = id
		}
		for _, rel := range []struct{ from, to, kind string }{{"org", "project", "customer_of"}, {"contact", "org", "contact_for"}} {
			if _, err := tx.Exec(ctx, `INSERT INTO node_relations(tenant_id,source_node_id,target_node_id,type) VALUES($1::uuid,$2::uuid,$3::uuid,$4)`, tenantID, ids[rel.from], ids[rel.to], rel.kind); err != nil {
				return err
			}
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
	events.New(database.App, events.WithUndoHandlers(UndoHandlers(reg))).Mount(mux)
	return &quoteFixture{t: t, database: database, tenantID: tenantID, ids: ids, mux: mux}
}

func (f *quoteFixture) call(actor, method, path, body string) (int, map[string]any) {
	f.t.Helper()
	roles := []string{actor}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req = req.WithContext(tenant.WithPrincipal(req.Context(), tenant.Principal{ID: f.ids[actor], TenantID: f.tenantID, Kind: tenant.Person, Roles: roles}))
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	var out map[string]any
	decoder := json.NewDecoder(strings.NewReader(rec.Body.String()))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	return rec.Code, out
}
func (f *quoteFixture) lastEvent(node, typ string) int64 {
	f.t.Helper()
	var id int64
	err := db.InTenant(context.Background(), f.database.App, f.tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id FROM events WHERE node_id=$1::uuid AND type=$2 AND undo_of IS NULL ORDER BY id DESC LIMIT 1`, node, typ).Scan(&id)
	})
	if err != nil {
		f.t.Fatalf("event %s on %s: %v", typ, node, err)
	}
	return id
}
func revisionOf(q map[string]any) string { return string(q["revision"].(json.Number)) }

func TestDeleteNeverIssuedDraftUndoArchiveAndDuplicate(t *testing.T) {
	f := newQuoteFixture(t, "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "quotes-delete-test")
	settings := `{"expected_revision":0,"numbering_time_zone":"Europe/Vienna","default_currency":"EUR","sender":{"company":"Example Sender","street":"Example Street 1","postal_code":"0000","city":"Example City","country":"AT","email":"sender@example.test"},"defaults":{"intro":"","blocks":[],"accept_text":"","vat_note":""},"layout":{},"smtp_confirmation_enabled":false}`
	if status, body := f.call("admin", "PATCH", "/api/quotes/settings", settings); status != 200 {
		t.Fatalf("settings %d %v", status, body)
	}
	create := func(title string) map[string]any {
		status, q := f.call("admin", "POST", "/api/quotes", fmt.Sprintf(`{"title":%q,"customer_org_node_id":%q}`, title, f.ids["org"]))
		if status != 201 {
			t.Fatalf("create %d %v", status, q)
		}
		return q
	}

	// Deleting a draft that was never issued: guarded, then hidden everywhere.
	a := create("Delete me")
	aID := a["quote_node_id"].(string)
	path := "/api/quotes/" + aID
	if status, _ := f.call("admin", "DELETE", path, ""); status != 400 {
		t.Fatalf("delete without a precondition %d", status)
	}
	if status, _ := f.call("admin", "DELETE", path+"?expected_revision=99", ""); status != 409 {
		t.Fatalf("stale delete %d", status)
	}
	if status, _ := f.call("member", "DELETE", path+"?expected_revision="+revisionOf(a), ""); status != 403 {
		t.Fatalf("member delete %d", status)
	}
	if status, body := f.call("admin", "DELETE", path+"?expected_revision="+revisionOf(a), ""); status != 204 {
		t.Fatalf("delete %d %v", status, body)
	}
	if status, _ := f.call("admin", "GET", path, ""); status != 404 {
		t.Fatalf("deleted quote still readable %d", status)
	}
	if status, _ := f.call("admin", "GET", path+"/draft", ""); status != 404 {
		t.Fatalf("deleted draft still readable %d", status)
	}
	listReq := httptest.NewRequest("GET", "/api/quotes?archived=all", nil)
	listReq = listReq.WithContext(tenant.WithPrincipal(listReq.Context(), tenant.Principal{ID: f.ids["admin"], TenantID: f.tenantID, Kind: tenant.Person, Roles: []string{"admin"}}))
	listRec := httptest.NewRecorder()
	f.mux.ServeHTTP(listRec, listReq)
	if strings.Contains(listRec.Body.String(), aID) {
		t.Fatal("deleted quote is listed")
	}
	// Its number stays allocated: the next quote gets a new one.
	b := create("Next")
	if b["offer_no"] == a["offer_no"] {
		t.Fatalf("offer number reused: %v", b["offer_no"])
	}

	// Undo brings it back once; a second undo is refused.
	deleted := f.lastEvent(aID, "quote.deleted")
	if status, body := f.call("admin", "POST", fmt.Sprintf("/api/events/%d/undo", deleted), ""); status != 201 {
		t.Fatalf("undo delete %d %v", status, body)
	}
	status, restored := f.call("admin", "GET", path, "")
	if status != 200 || restored["state"] != "draft" || restored["offer_no"] != a["offer_no"] {
		t.Fatalf("restored %d %v", status, restored)
	}
	if status, _ := f.call("admin", "POST", fmt.Sprintf("/api/events/%d/undo", deleted), ""); status != 409 {
		t.Fatalf("second undo %d", status)
	}

	// Archive and its undo.
	if status, q := f.call("admin", "PATCH", path+"/visibility", fmt.Sprintf(`{"expected_revision":%s,"archived":true}`, revisionOf(restored))); status != 200 || q["archived"] != true {
		t.Fatalf("archive %d %v", status, q)
	}
	archived := f.lastEvent(aID, "quote.visibility_changed")
	if status, body := f.call("admin", "POST", fmt.Sprintf("/api/events/%d/undo", archived), ""); status != 201 {
		t.Fatalf("undo archive %d %v", status, body)
	}
	if _, q := f.call("admin", "GET", path, ""); q["archived"] != false {
		t.Fatalf("archive not undone %v", q)
	}

	// Undoing a duplicate removes the untouched copy; a copy someone saved stays.
	_, current := f.call("admin", "GET", path, "")
	status, copyQ := f.call("admin", "POST", path+"/duplicate", fmt.Sprintf(`{"expected_revision":%s}`, revisionOf(current)))
	if status != 201 {
		t.Fatalf("duplicate %d %v", status, copyQ)
	}
	copyID := copyQ["quote_node_id"].(string)
	if status, body := f.call("admin", "POST", fmt.Sprintf("/api/events/%d/undo", f.lastEvent(copyID, "quote.duplicated")), ""); status != 201 {
		t.Fatalf("undo duplicate %d %v", status, body)
	}
	if status, _ := f.call("admin", "GET", "/api/quotes/"+copyID, ""); status != 404 {
		t.Fatalf("undone duplicate still readable %d", status)
	}
	status, second := f.call("admin", "POST", path+"/duplicate", fmt.Sprintf(`{"expected_revision":%s}`, revisionOf(current)))
	if status != 201 {
		t.Fatalf("second duplicate %d %v", status, second)
	}
	secondID := second["quote_node_id"].(string)
	_, draft := f.call("admin", "GET", "/api/quotes/"+secondID+"/draft", "")
	doc := draft["document"].(map[string]any)
	doc["title"] = "Edited copy"
	body, _ := json.Marshal(map[string]any{"client_session_id": "22222222-2222-4222-8222-222222222222", "mutation_id": "33333333-3333-4333-8333-333333333333", "writer_version": 2, "document": doc})
	req := httptest.NewRequest("PATCH", "/api/quotes/"+secondID+"/draft", strings.NewReader(string(body)))
	req.Header.Set("If-Match", `"qd-1"`)
	req = req.WithContext(tenant.WithPrincipal(req.Context(), tenant.Principal{ID: f.ids["admin"], TenantID: f.tenantID, Kind: tenant.Person, Roles: []string{"admin"}}))
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("save copy %d %s", rec.Code, rec.Body.String())
	}
	if status, _ := f.call("admin", "POST", fmt.Sprintf("/api/events/%d/undo", f.lastEvent(secondID, "quote.duplicated")), ""); status != 409 {
		t.Fatalf("undo of an edited duplicate %d", status)
	}

	// Anything once issued is evidence: it cannot be deleted, only archived.
	status, issued := f.call("admin", "POST", "/api/quotes", fmt.Sprintf(`{"title":"Issued","project_node_id":%q,"customer_org_node_id":%q}`, f.ids["project"], f.ids["org"]))
	if status != 201 {
		t.Fatalf("create issued %d %v", status, issued)
	}
	issuedID := issued["quote_node_id"].(string)
	version := fmt.Sprintf(`{"expected_revision":%s,"recipient_contact_node_id":%q,"currency":"EUR","title":"Issued","terms_markdown":"","lines":[{"description":"Work","cost_unit_node_id":%q,"unit":"hour","quantity":1,"tax_rate":0.2}]}`, revisionOf(issued), f.ids["contact"], f.ids["cost"])
	if status, v := f.call("admin", "POST", "/api/quotes/"+issuedID+"/versions", version); status != 201 {
		t.Fatalf("freeze %d %v", status, v)
	}
	status, issued = f.call("admin", "POST", "/api/quotes/"+issuedID+"/versions/1/issue", "")
	if status != 200 || issued["state"] != "issued" {
		t.Fatalf("issue %d %v", status, issued)
	}
	if status, body := f.call("admin", "DELETE", "/api/quotes/"+issuedID+"?expected_revision="+revisionOf(issued), ""); status != 409 || !strings.Contains(fmt.Sprint(body), "archive it instead") {
		t.Fatalf("issued delete %d %v", status, body)
	}
	// The database refuses it too, whatever the caller.
	err := db.InTenant(context.Background(), f.database.App, f.tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE business_quotes SET deleted_at=now(),deleted_by_principal_id=$2::uuid WHERE quote_node_id=$1::uuid`, issuedID, f.ids["admin"])
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "never issued") {
		t.Fatalf("trigger guard %v", err)
	}
}
