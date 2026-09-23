// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/tenant"
)

func TestNewIsModule(t *testing.T) {
	var module httpapi.Module = New(nil, nil)
	mux := http.NewServeMux()
	module.Mount(mux)
}

func TestParseAndRejectBeforeDatabase(t *testing.T) {
	mux := http.NewServeMux()
	New(nil, nil).Mount(mux)
	principal := tenant.Principal{ID: "cccccccc-cccc-4ccc-8ccc-ccccccccccc1", TenantID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1", Kind: tenant.Person, Name: "Ada"}

	status, body := call(mux, nil, "/api/search?q=Alpha")
	if status != http.StatusUnauthorized || !contains(body, `"unauthorized"`) {
		t.Fatalf("auth %d %s", status, body)
	}
	status, body = call(mux, &principal, "/api/search")
	if status != http.StatusBadRequest || !contains(body, `"q is required"`) {
		t.Fatalf("missing q %d %s", status, body)
	}
	status, body = call(mux, &principal, "/api/search?q=%20")
	if status != http.StatusBadRequest {
		t.Fatalf("blank q %d %s", status, body)
	}
	status, body = call(mux, &principal, "/api/search?q=Alpha&limit=0")
	if status != http.StatusBadRequest || !contains(body, "limit") {
		t.Fatalf("limit %d %s", status, body)
	}
	status, body = call(mux, &principal, "/api/search?q=Alpha&limit=201")
	if status != http.StatusBadRequest {
		t.Fatalf("limit high %d", status)
	}
	status, body = call(mux, &principal, "/api/search?q=Alpha&kind_id=not-a-uuid")
	if status != http.StatusBadRequest || !contains(body, "kind_id") {
		t.Fatalf("kind %d %s", status, body)
	}
	status, body = call(mux, &principal, "/api/search?q=Alpha&state=")
	if status != http.StatusBadRequest {
		t.Fatalf("state %d %s", status, body)
	}
	raw, err := encodeCursor(cursorBody{Tenant: principal.TenantID, Q: "other", Score: "0.1", ID: "00000000-0000-4000-8000-000000000001"})
	if err != nil {
		t.Fatal(err)
	}
	status, body = call(mux, &principal, "/api/search?q=Alpha&cursor="+raw)
	if status != http.StatusBadRequest || !contains(body, "cursor") {
		t.Fatalf("cursor %d %s", status, body)
	}
	status, _ = call(mux, &principal, "/api/search?q=Alpha&cursor=not-a-cursor")
	if status != http.StatusBadRequest {
		t.Fatalf("bad cursor %d", status)
	}
	if _, err := parseLimit(""); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/search?q=Alpha", nil)
	req = req.WithContext(tenant.WithPrincipal(req.Context(), principal))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post %d", rec.Code)
	}
}

func TestPageHits(t *testing.T) {
	hits := []scored{
		{ID: "00000000-0000-4000-8000-000000000001", Score: 1.0 / 61},
		{ID: "00000000-0000-4000-8000-000000000002", Score: 1.0 / 62},
		{ID: "00000000-0000-4000-8000-000000000003", Score: 1.0 / 63},
	}
	bind := cursorBody{Tenant: "t", Q: "Alpha"}
	var cur *cursorBody
	var seen []string
	for {
		page, next, err := pageHits(hits, cur, 1, bind)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		seen = append(seen, page[0].ID)
		if next == nil {
			break
		}
		decoded, err := decodeCursor(*next)
		if err != nil || !cursorMatches(decoded, "t", "Alpha", "", "") {
			t.Fatal(err)
		}
		cur = &decoded
	}
	if len(seen) != 3 || seen[0] == seen[1] || seen[2] != hits[2].ID {
		t.Fatalf("pages %v", seen)
	}
}

func call(mux *http.ServeMux, p *tenant.Principal, path string) (int, string) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if p != nil {
		req = req.WithContext(tenant.WithPrincipal(req.Context(), *p))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func contains(body, part string) bool {
	return strings.Contains(body, part)
}
