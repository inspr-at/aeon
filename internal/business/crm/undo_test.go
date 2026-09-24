// SPDX-License-Identifier: AGPL-3.0-only
package crm

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/jackc/pgx/v5"
)

func withUndo(t *testing.T, f *fixture) {
	t.Helper()
	plug, e := Plugin()
	if e != nil {
		t.Fatal(e)
	}
	reg := plugins.NewRegistry()
	if e = reg.Register(plug); e != nil {
		t.Fatal(e)
	}
	reg.Seal()
	f.handler = (&httpapi.Server{Pool: f.db.App, Modules: []httpapi.Module{New(f.db.App, reg), events.New(f.db.App, events.WithUndoHandlers(UndoHandlers(reg)))}}).Handler()
}
func eventOf(t *testing.T, f fixture, typ string) events.Event {
	t.Helper()
	all := logEvents(t, f)
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Type == typ {
			return all[i]
		}
	}
	t.Fatalf("missing %s event", typ)
	return events.Event{}
}
func undoEvent(t *testing.T, f fixture, event events.Event, status int) {
	t.Helper()
	expect(t, request(f.handler, f.admin, http.MethodPost, "/api/events/"+strconv.FormatInt(event.ID, 10)+"/undo", ""), status)
}
func TestCRMUndoStaleAndPrimary(t *testing.T) {
	f := setup(t)
	withUndo(t, &f)
	w := jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations", map[string]any{"name": "Undo Inc"})
	expect(t, w, 201)
	var c Customer
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	w = jsonRequest(t, f, f.admin, "PATCH", "/api/crm/organisations/"+c.ID, map[string]any{"name": "Changed Inc", "expected_revision": c.Revision})
	expect(t, w, 200)
	updated := eventOf(t, f, "crm.customer_updated")
	undoEvent(t, f, updated, 201)
	undoEvent(t, f, updated, 409)
	w = request(f.handler, f.admin, "GET", "/api/crm/organisations/"+c.ID, "")
	expect(t, w, 200)
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	if c.Name != "Undo Inc" {
		t.Fatal("customer undo did not restore name")
	}
	w = jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations/"+c.ID+"/contacts", map[string]any{"name": "First"})
	expect(t, w, 201)
	var first ContactRecord
	_ = json.Unmarshal(w.Body.Bytes(), &first)
	firstPrimary := eventOf(t, f, "crm.primary_contact_changed")
	w = jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations/"+c.ID+"/contacts", map[string]any{"name": "Second"})
	expect(t, w, 201)
	var second ContactRecord
	_ = json.Unmarshal(w.Body.Bytes(), &second)
	w = request(f.handler, f.admin, "GET", "/api/crm/organisations/"+c.ID, "")
	expect(t, w, 200)
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	w = jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations/"+c.ID+"/primary-contact", map[string]any{"contact_node_id": second.ID, "expected_revision": c.Revision})
	expect(t, w, 200)
	promote := eventOf(t, f, "crm.primary_contact_changed")
	undoEvent(t, f, promote, 201)
	w = request(f.handler, f.admin, "GET", "/api/crm/organisations/"+c.ID, "")
	expect(t, w, 200)
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	if c.PrimaryContactNodeID == nil || *c.PrimaryContactNodeID != first.ID {
		t.Fatal("primary undo failed")
	}
	// The first promotion is stale after further CRM changes.
	undoEvent(t, f, firstPrimary, 409)
	contactCreated := eventOf(t, f, "crm.contact_created")
	undoEvent(t, f, contactCreated, 201)
}
func TestCRMDeletionAndProjectUndo(t *testing.T) {
	f := setup(t)
	withUndo(t, &f)
	w := jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations", map[string]any{"name": "Delete Me"})
	expect(t, w, 201)
	var c Customer
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	w = jsonRequest(t, f, f.admin, "POST", "/api/crm/organisations/"+c.ID+"/contacts", map[string]any{"name": "Contact"})
	expect(t, w, 201)
	var contact ContactRecord
	_ = json.Unmarshal(w.Body.Bytes(), &contact)
	var project string
	err := db.InTenant(t.Context(), f.db.App, f.admin.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `INSERT INTO nodes(tenant_id,kind_id,key,title) SELECT $1,id,'PRJ-1','Project' FROM node_kinds WHERE slug='project' RETURNING id::text`, f.admin.TenantID).Scan(&project)
	})
	if err != nil {
		t.Fatal(err)
	}
	expect(t, jsonRequest(t, f, f.admin, "PUT", "/api/crm/projects/"+project+"/customer", map[string]any{"organisation_node_id": c.ID}), 200)
	link := eventOf(t, f, "crm.project_customer_changed")
	undoEvent(t, f, link, 201)
	expect(t, jsonRequest(t, f, f.admin, "PUT", "/api/crm/projects/"+project+"/cooperation", map[string]any{"engagement": "retainer"}), 200)
	cooperation := eventOf(t, f, "crm.project_cooperation_changed")
	undoEvent(t, f, cooperation, 201)
	expect(t, request(f.handler, f.admin, "DELETE", "/api/crm/organisations/"+c.ID, ""), 204)
	undoEvent(t, f, eventOf(t, f, "crm.customer_deleted"), 201)
	undoEvent(t, f, eventOf(t, f, "crm.contact_deleted"), 201)
	undoEvent(t, f, eventOf(t, f, "crm.primary_contact_changed"), 201)
	w = request(f.handler, f.admin, "GET", "/api/crm/organisations/"+c.ID, "")
	expect(t, w, 200)
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	if c.PrimaryContactNodeID == nil || *c.PrimaryContactNodeID != contact.ID {
		t.Fatalf("primary not restored %+v", c)
	}
}
