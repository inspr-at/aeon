// SPDX-License-Identifier: AGPL-3.0-only
package crm

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/dbtest"
	"github.com/inspr-at/paimos/internal/tenant"
)

func jsonRequest(t *testing.T, f fixture, p tenant.Principal, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, e := json.Marshal(body)
	if e != nil {
		t.Fatal(e)
	}
	return request(f.handler, p, method, path, string(raw))
}
func TestManualCustomerContactPrimaryAndTenantIsolation(t *testing.T) {
	f := setup(t)
	w := jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations", map[string]any{"name": "Example GmbH", "billing_address": map[string]any{"street": "Main 1", "postal_code": "8010", "city": "Graz", "country": "AT"}, "annual_revenue_minor": 12345})
	expect(t, w, 201)
	var c Customer
	if e := json.Unmarshal(w.Body.Bytes(), &c); e != nil {
		t.Fatal(e)
	}
	if c.ID == "" || c.CustomerNo != nil || c.BillingAddress == nil || c.BillingAddress.City != "Graz" || c.AnnualRevenueMinor == nil || *c.AnnualRevenueMinor != 12345 {
		t.Fatalf("customer %+v", c)
	}
	err := db.InTenant(dbtest.Seed(t.Context()), f.db.App, f.other.TenantID, func(tx pgx.Tx) error {
		_, e := tx.Exec(t.Context(), `UPDATE plugin_installations SET permissions=array_append(permissions,'views.provide') WHERE plugin_id=$1`, ID)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	expect(t, request(f.handler, f.other, "GET", "/api/crm/organisations/"+c.ID, ""), 404)
	expect(t, request(f.handler, f.admin, "GET", "/api/crm/organisations?q=Example", ""), 200)
	w = jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations/"+c.ID+"/contacts", map[string]any{"name": "Ada", "email": "ada@example.test", "role": "Buyer"})
	expect(t, w, 201)
	var first ContactRecord
	if e := json.Unmarshal(w.Body.Bytes(), &first); e != nil {
		t.Fatal(e)
	}
	if !first.Primary || first.OrganisationNodeID != c.ID {
		t.Fatalf("first %+v", first)
	}
	expect(t, request(f.handler, f.admin, "GET", "/api/crm/contacts/"+first.ID, ""), 200)
	w = jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations/"+c.ID+"/contacts", map[string]any{"name": "Bea", "email": "ada@example.test"})
	expect(t, w, 201)
	var second ContactRecord
	if e := json.Unmarshal(w.Body.Bytes(), &second); e != nil {
		t.Fatal(e)
	}
	if second.Primary {
		t.Fatal("second unexpectedly primary")
	} // Equal email never creates principal binding.
	if count(t, f, f.admin.TenantID, `SELECT count(*) FROM crm_contact_principals`) != 0 {
		t.Fatal("email bound identity")
	}
	w = request(f.handler, f.admin, "GET", "/api/crm/organisations/"+c.ID, "")
	expect(t, w, 200)
	if e := json.Unmarshal(w.Body.Bytes(), &c); e != nil {
		t.Fatal(e)
	}
	w = jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations/"+c.ID+"/primary-contact", map[string]any{"contact_node_id": second.ID, "expected_revision": c.Revision})
	expect(t, w, 200)
	expect(t, request(f.handler, f.admin, "DELETE", "/api/crm/contacts/"+second.ID, ""), 204)
	w = request(f.handler, f.admin, "GET", "/api/crm/organisations/"+c.ID, "")
	expect(t, w, 200)
	if e := json.Unmarshal(w.Body.Bytes(), &c); e != nil {
		t.Fatal(e)
	}
	if c.PrimaryContactNodeID == nil || *c.PrimaryContactNodeID != first.ID {
		t.Fatalf("successor %+v", c)
	}
	expect(t, request(f.handler, f.admin, "DELETE", "/api/crm/organisations/"+c.ID, ""), 204)
	expect(t, request(f.handler, f.admin, "GET", "/api/crm/organisations/"+c.ID, ""), 404)
	w = request(f.handler, f.other, "GET", "/api/crm/organisations", "")
	expect(t, w, 200)
	if strings.Contains(w.Body.String(), c.ID) {
		t.Fatal("customer leaked across tenants")
	}
}
func TestCustomerRevisionNoteDraftAndProjectLink(t *testing.T) {
	f := setup(t)
	w := jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations", map[string]any{"name": "Client"})
	expect(t, w, 201)
	var c Customer
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	bad := map[string]any{"name": "Changed", "expected_revision": c.Revision + 1}
	expect(t, jsonRequest(t, f, f.admin, "PATCH", "/api/crm/organisations/"+c.ID, bad), 409)
	w = jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations/"+c.ID+"/note-rewrite", map[string]any{"draft_text": "Human review pending", "expected_revision": c.Revision})
	expect(t, w, 201)
	var draft struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &draft)
	w = request(f.handler, f.admin, "GET", "/api/crm/organisations/"+c.ID, "")
	expect(t, w, 200)
	var unchanged Customer
	_ = json.Unmarshal(w.Body.Bytes(), &unchanged)
	if unchanged.CustomerNotes != "" {
		t.Fatal("draft auto-applied")
	}
	w = request(f.handler, f.admin, "POST", "/api/crm/organisations/"+c.ID+"/note-rewrite/"+draft.ID+"/apply", "")
	expect(t, w, 200)
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	if c.CustomerNotes != "Human review pending" {
		t.Fatalf("notes %+v", c)
	}
	expect(t, request(f.handler, f.admin, "POST", "/api/crm/organisations/"+c.ID+"/note-rewrite/"+draft.ID+"/apply", ""), 409)
	var project string
	e := db.InTenant(dbtest.Seed(t.Context()), f.db.App, f.admin.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `INSERT INTO nodes(tenant_id,kind_id,key,title) SELECT $1,id,'PRJ-1','Project' FROM node_kinds WHERE slug='project' RETURNING id::text`, f.admin.TenantID).Scan(&project)
	})
	if e != nil {
		t.Fatal(e)
	}
	w = jsonRequest(t, f, f.admin, "PUT", "/api/crm/projects/"+project+"/customer", map[string]any{"organisation_node_id": c.ID})
	expect(t, w, 200)
	var attachment string
	e = db.InTenant(dbtest.Seed(t.Context()), f.db.App, f.admin.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `INSERT INTO attachments(tenant_id,node_id,sha256,name,content_type,size,created_by) VALUES($1::uuid,$2::uuid,$3,'agreement.pdf','application/pdf',1,$4::uuid) RETURNING id::text`, f.admin.TenantID, project, strings.Repeat("a", 64), f.admin.ID).Scan(&attachment)
	})
	if e != nil {
		t.Fatal(e)
	}
	expect(t, jsonRequest(t, f, f.admin, "PUT", "/api/crm/documents/"+attachment+"/metadata", map[string]any{"title": "Agreement", "category": "contract", "status": "active", "valid_from": "2026-01-01", "valid_until": "2027-12-31", "expected_revision": 0}), 200)
	expect(t, jsonRequest(t, f, f.admin, "PUT", "/api/crm/projects/"+project+"/cooperation", map[string]any{"engagement": "retainer", "sla": "next business day", "expected_revision": 0}), 200)
	w = request(f.handler, f.admin, "GET", "/api/crm/organisations/"+c.ID+"/related", "")
	expect(t, w, 200)
	var related Related
	_ = json.Unmarshal(w.Body.Bytes(), &related)
	if len(related.Projects) != 1 || related.Projects[0].ID != project {
		t.Fatalf("related %+v", related)
	}
	if len(related.Documents) != 1 || related.Documents[0].AttachmentID != attachment || related.Documents[0].Revision != 1 || related.Documents[0].Status != "active" || related.Projects[0].CooperationRevision != 1 || !strings.Contains(string(related.Projects[0].Cooperation), "retainer") {
		t.Fatalf("related metadata %+v", related)
	}
	expect(t, jsonRequest(t, f, f.admin, "PUT", "/api/crm/documents/"+attachment+"/metadata", map[string]any{"title": "New", "category": "contract", "status": "expired", "expected_revision": 0}), 409)
	expect(t, request(f.handler, f.admin, "DELETE", "/api/crm/organisations/"+c.ID, ""), 409)
	if !strings.Contains(fmt.Sprint(logEvents(t, f)), "crm.note_rewrite_applied") {
		t.Fatal("missing event")
	}
}
