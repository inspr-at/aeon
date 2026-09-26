// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/jackc/pgx/v5"
)

func TestLookupNodesIsBoundedAndTenantScoped(t *testing.T) {
	p := newPrincipal(t, "lookup")
	kind := kindBySlug(t, p, "ticket")
	first := mustNode(t, p, `{"kind_id":"`+kind.ID+`","title":"First"}`)
	second := mustNode(t, p, `{"kind_id":"`+kind.ID+`","title":"Second"}`)
	other := addPrincipal(t, "lookup-other")
	outsider := mustNode(t, other, `{"kind_id":"`+kindBySlug(t, other, "ticket").ID+`","title":"Outside"}`)
	missing := "00000000-0000-4000-8000-000000000099"
	ids := second.ID + "," + outsider.ID + "," + first.ID + "," + second.ID + "," + missing
	status, body := call(t, &p, http.MethodGet, "/api/nodes/lookup?ids="+url.QueryEscape(ids), "")
	page := decode[struct {
		Items []nodePreview `json:"items"`
	}](t, status, body, http.StatusOK)
	if len(page.Items) != 2 || page.Items[0].ID != second.ID || page.Items[1].ID != first.ID || page.Items[0].Title != "Second" {
		t.Fatalf("lookup order, dedupe or tenant scope: %+v", page.Items)
	}
	for _, suffix := range []string{"", "?ids=bad", "?ids=" + first.ID + ",", "?ids=" + first.ID + ",bad"} {
		status, _ := call(t, &p, http.MethodGet, "/api/nodes/lookup"+suffix, "")
		if status != http.StatusBadRequest {
			t.Fatalf("%q returned %d", suffix, status)
		}
	}
	many := make([]string, 101)
	for i := range many {
		many[i] = fmt.Sprintf("00000000-0000-4000-8000-%012x", i)
	}
	status, _ = call(t, &p, http.MethodGet, "/api/nodes/lookup?ids="+url.QueryEscape(strings.Join(many, ",")), "")
	if status != http.StatusBadRequest {
		t.Fatalf("101 IDs returned %d", status)
	}
}

func TestLookupNodesByKeyResolvesProjectAndEarlierKeys(t *testing.T) {
	p := newPrincipal(t, "lookup-keys")
	projectKind := kindBySlug(t, p, "project")
	ticketKind := kindBySlug(t, p, "ticket")
	taskKind := kindBySlug(t, p, "task")
	project := mustNode(t, p, `{"kind_id":"`+projectKind.ID+`","title":"Release notes","fields":{"project_key":"REL"}}`)
	ticket := mustNode(t, p, `{"kind_id":"`+ticketKind.ID+`","title":"Link tickets","parent_id":"`+project.ID+`","key_prefix":"REL"}`)
	task := mustNode(t, p, `{"kind_id":"`+taskKind.ID+`","title":"Nested task","parent_id":"`+ticket.ID+`","key_prefix":"REL"}`)
	if err := db.InTenant(dbtest.Seed(t.Context()), appPool, p.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO node_key_aliases(tenant_id,key,node_id) VALUES($1::uuid,'OLD-7',$2::uuid)`, p.TenantID, ticket.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	other := addPrincipal(t, "lookup-keys-other")
	outsider := mustNode(t, other, `{"kind_id":"`+kindBySlug(t, other, "ticket").ID+`","title":"Outside","key_prefix":"ELSE"}`)
	keys := strings.ToLower(task.Key) + ",NOPE-9," + outsider.Key + ",OLD-7," + ticket.Key + "," + task.Key
	status, body := call(t, &p, http.MethodGet, "/api/nodes/lookup?keys="+url.QueryEscape(keys), "")
	page := decode[struct {
		Items []nodePreview `json:"items"`
	}](t, status, body, http.StatusOK)
	want := []nodePreview{
		{ID: task.ID, Key: task.Key, Title: "Nested task", State: task.State, RequestedKey: task.Key, ProjectID: project.ID},
		{ID: ticket.ID, Key: ticket.Key, Title: "Link tickets", State: ticket.State, RequestedKey: "OLD-7", ProjectID: project.ID},
		{ID: ticket.ID, Key: ticket.Key, Title: "Link tickets", State: ticket.State, RequestedKey: ticket.Key, ProjectID: project.ID},
	}
	if fmt.Sprint(page.Items) != fmt.Sprint(want) {
		t.Fatalf("key lookup (case, dedupe, tenant, alias, project):\n got %+v\nwant %+v", page.Items, want)
	}
	// IDs and keys together: ID previews first, and only key previews name the request.
	status, body = call(t, &p, http.MethodGet, "/api/nodes/lookup?ids="+project.ID+"&keys="+ticket.Key, "")
	mixed := decode[struct {
		Items []nodePreview `json:"items"`
	}](t, status, body, http.StatusOK)
	if len(mixed.Items) != 2 || mixed.Items[0].ID != project.ID || mixed.Items[0].RequestedKey != "" || mixed.Items[1].RequestedKey != ticket.Key {
		t.Fatalf("mixed lookup: %+v", mixed.Items)
	}
	for _, suffix := range []string{"?keys=", "?keys=REL", "?keys=REL-0", "?keys=" + url.QueryEscape("REL-1,x y")} {
		status, _ := call(t, &p, http.MethodGet, "/api/nodes/lookup"+suffix, "")
		if status != http.StatusBadRequest {
			t.Fatalf("%q returned %d", suffix, status)
		}
	}
	many := make([]string, 101)
	for i := range many {
		many[i] = fmt.Sprintf("REL-%d", i+1)
	}
	status, _ = call(t, &p, http.MethodGet, "/api/nodes/lookup?keys="+strings.Join(many, ","), "")
	if status != http.StatusBadRequest {
		t.Fatalf("101 keys returned %d", status)
	}
}
