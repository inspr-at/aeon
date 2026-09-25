// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
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
