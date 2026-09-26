// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/tenant"
)

func createTestTag(t *testing.T, p tenant.Principal, name string) nodeJSON {
	t.Helper()
	status, body := call(t, &p, http.MethodPost, "/api/kinds", `{"slug":"tag","label":"Tag","short_prefix":"TAG","icon":"tag","field_schema":{}}`)
	kind := decode[kindJSON](t, status, body, http.StatusCreated)
	status, body = call(t, &p, http.MethodPost, "/api/nodes", fmt.Sprintf(`{"kind_id":%q,"title":%q,"fields":{"color":"blue"}}`, kind.ID, name))
	return decode[nodeJSON](t, status, body, http.StatusCreated)
}

func createTaggedNode(t *testing.T, p tenant.Principal, tags []string) nodeJSON {
	t.Helper()
	kind := kindBySlug(t, p, "project")
	fields, _ := json.Marshal(map[string]any{"tags": tags, "unrelated": "preserved"})
	status, body := call(t, &p, http.MethodPost, "/api/nodes", fmt.Sprintf(`{"kind_id":%q,"title":"Project","fields":%s}`, kind.ID, fields))
	return decode[nodeJSON](t, status, body, http.StatusCreated)
}

func nodeTags(t *testing.T, p tenant.Principal, id string) []string {
	t.Helper()
	status, body := call(t, &p, http.MethodGet, "/api/nodes/"+id, "")
	node := decode[nodeJSON](t, status, body, http.StatusOK)
	var fields struct {
		Tags      []string `json:"tags"`
		Unrelated string   `json:"unrelated"`
	}
	if err := json.Unmarshal(node.Fields, &fields); err != nil || fields.Unrelated != "preserved" {
		t.Fatalf("fields %s: %v", node.Fields, err)
	}
	return fields.Tags
}

func TestTagAssignmentMutationIsAtomicAndTenantScoped(t *testing.T) {
	p := newPrincipal(t, "tags_a")
	tag := createTestTag(t, p, "Old")
	status, body := call(t, &p, http.MethodPatch, "/api/nodes/"+tag.ID, `{"fields":{"color":"blue","tags":["Old"]}}`)
	decode[nodeJSON](t, status, body, http.StatusOK)
	a := createTaggedNode(t, p, []string{"Old", "keep"})
	b := createTaggedNode(t, p, []string{"keep", "Old"})
	other := createTaggedNode(t, p, []string{"keep"})
	status, _ = call(t, &p, http.MethodPatch, "/api/nodes/"+tag.ID, `{"title":"bypass"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("generic tag rename status %d", status)
	}
	status, _ = call(t, &p, http.MethodDelete, "/api/nodes/"+tag.ID, "")
	if status != http.StatusBadRequest {
		t.Fatalf("generic tag delete status %d", status)
	}
	foreign := addPrincipal(t, "tags_b")
	_ = createTestTag(t, foreign, "Old")
	c := createTaggedNode(t, foreign, []string{"Old"})
	before := len(tenantEvents(t, p.TenantID))
	foreignBefore := len(tenantEvents(t, foreign.TenantID))

	status, body = call(t, &p, http.MethodPatch, "/api/tags/"+tag.ID, `{"name":"New","color":"green","description":"renamed"}`)
	updated := decode[nodeJSON](t, status, body, http.StatusOK)
	if updated.Title != "New" || updated.Body != "renamed" {
		t.Fatalf("tag: %+v", updated)
	}
	var ownFields struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(updated.Fields, &ownFields); err != nil || !reflect.DeepEqual(ownFields.Tags, []string{"New"}) {
		t.Fatalf("tag's own assignment: %s: %v", updated.Fields, err)
	}
	for _, id := range []string{a.ID, b.ID} {
		if got := nodeTags(t, p, id); !reflect.DeepEqual(got, []string{"New", "keep"}) && !reflect.DeepEqual(got, []string{"keep", "New"}) {
			t.Fatalf("renamed tags: %v", got)
		}
	}
	if got := nodeTags(t, p, other.ID); !reflect.DeepEqual(got, []string{"keep"}) {
		t.Fatalf("unassigned node: %v", got)
	}
	if got := nodeTags(t, foreign, c.ID); !reflect.DeepEqual(got, []string{"Old"}) {
		t.Fatalf("other tenant: %v", got)
	}
	events := tenantEvents(t, p.TenantID)
	if len(events) != before+3 || len(tenantEvents(t, foreign.TenantID)) != foreignBefore {
		t.Fatalf("event counts %d and %d", len(events)-before, len(tenantEvents(t, foreign.TenantID))-foreignBefore)
	}
	for _, event := range events[before:] {
		if event.Type != evNodeUpdated || event.Before == nil || event.After == nil || event.Actor != p.ID {
			t.Fatalf("rename event: %+v", event)
		}
	}

	before = len(events)
	status, body = call(t, &p, http.MethodDelete, "/api/tags/"+tag.ID, "")
	decode[any](t, status, body, http.StatusNoContent)
	if got := nodeTags(t, p, a.ID); !reflect.DeepEqual(got, []string{"keep"}) {
		t.Fatalf("deleted tag assignment: %v", got)
	}
	if got := nodeTags(t, foreign, c.ID); !reflect.DeepEqual(got, []string{"Old"}) {
		t.Fatalf("foreign assignment after delete: %v", got)
	}
	events = tenantEvents(t, p.TenantID)
	if len(events) != before+3 || events[len(events)-1].Type != evNodeDeleted {
		t.Fatalf("delete events: %+v", events[before:])
	}
	status, _ = call(t, &p, http.MethodGet, "/api/nodes/"+tag.ID, "")
	if status != http.StatusNotFound {
		t.Fatalf("deleted tag status %d", status)
	}
	status, _ = call(t, &foreign, http.MethodPatch, "/api/tags/"+tag.ID, `{"name":"stolen"}`)
	if status != http.StatusNotFound {
		t.Fatalf("cross-tenant status %d", status)
	}
}

type failSecondTagEvent struct{ writes int }

func (f *failSecondTagEvent) WriteEvent(ctx context.Context, tx pgx.Tx, e Event) error {
	f.writes++
	if f.writes == 2 {
		return errors.New("forced event failure")
	}
	return SQLWriter{}.WriteEvent(ctx, tx, e)
}

func TestTagMutationRollsBackAfterEventFailure(t *testing.T) {
	p := newPrincipal(t, "tags_rollback")
	tag := createTestTag(t, p, "Old")
	a := createTaggedNode(t, p, []string{"Old"})
	before := len(tenantEvents(t, p.TenantID))
	writer := &failSecondTagEvent{}
	status, _ := callAs(t, New(appPool, writer), &p, http.MethodPatch, "/api/tags/"+tag.ID, `{"name":"New"}`)
	if status != http.StatusInternalServerError || writer.writes != 2 {
		t.Fatalf("status %d; writes %d", status, writer.writes)
	}
	if got := nodeTags(t, p, a.ID); !reflect.DeepEqual(got, []string{"Old"}) {
		t.Fatalf("rolled-back assignment: %v", got)
	}
	status, body := call(t, &p, http.MethodGet, "/api/nodes/"+tag.ID, "")
	if current := decode[nodeJSON](t, status, body, http.StatusOK); current.Title != "Old" {
		t.Fatalf("rolled-back tag: %+v", current)
	}
	if got := len(tenantEvents(t, p.TenantID)); got != before {
		t.Fatalf("rolled-back events: %d, want %d", got, before)
	}
}
