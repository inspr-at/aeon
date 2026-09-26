// SPDX-License-Identifier: AGPL-3.0-only
package crm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inspr-at/paimos/internal/events"
	"github.com/inspr-at/paimos/internal/httpapi"
	"github.com/inspr-at/paimos/internal/plugins"
	"github.com/inspr-at/paimos/internal/plugins/fence"
)

func TestHTTPProviderContractAndImport(t *testing.T) {
	f := setup(t)
	hits := 0
	failFetch := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fake" {
			t.Error("authorization header missing")
		}
		switch r.URL.Path {
		case "/customers/search":
			if r.URL.Query().Get("q") != "Acme" {
				t.Error("search query")
			}
			_ = json.NewEncoder(w).Encode([]RemoteCustomer{{ExternalID: "ext-1", Name: "Acme", Fields: CustomerFields{Domain: "example.test"}}})
		case "/customers/ext-1":
			hits++
			if failFetch {
				http.Error(w, "private provider detail", http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(RemoteCustomer{ExternalID: "ext-1", Name: "Acme", Fields: CustomerFields{Domain: "example.test"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	provider, e := NewHTTPProvider(server.URL, server.Client(), func(context.Context, string) (string, error) { return "fake", nil })
	if e != nil {
		t.Fatal(e)
	}
	providers := map[string]Provider{"http": provider}
	plug, e := PluginWithProviders(providers)
	if e != nil {
		t.Fatal(e)
	}
	reg := plugins.NewRegistry()
	if e = reg.Register(plug); e != nil {
		t.Fatal(e)
	}
	reg.Seal()
	f.handler = (&httpapi.Server{Pool: f.db.App, Modules: []httpapi.Module{NewWithProviders(f.db.App, reg, providers), events.New(f.db.App, events.WithUndoHandlers(UndoHandlers(reg)))}}).Handler()
	// Integration permission is separately granted; manual CRM did not need it.
	f.setInstall(t, true, f.digest, []string{fence.PermNodesContribute, fence.PermViewsProvide, fence.PermStepsApply, fence.PermIntegrationsCall})
	w := jsonRequest(t, f, f.admin, "PUT", "/api/crm/providers/http/config", map[string]any{"enabled": true, "secret_ref": "secret://test/crm"})
	expect(t, w, 200)
	if strings.Contains(w.Body.String(), "secret://") {
		t.Fatal("provider reference exposed")
	}
	w = request(f.handler, f.admin, "GET", "/api/crm/providers/search?q=Acme", "")
	expect(t, w, 200)
	if !strings.Contains(w.Body.String(), "ext-1") {
		t.Fatal(w.Body.String())
	}
	w = jsonRequest(t, f, f.admin, "POST", "/api/crm/providers/http/import", map[string]any{"external_id": "ext-1"})
	expect(t, w, 201)
	var imported Customer
	if e = json.Unmarshal(w.Body.Bytes(), &imported); e != nil {
		t.Fatal(e)
	}
	if imported.ExternalProvider != "http" || imported.Domain != "example.test" {
		t.Fatalf("imported %+v", imported)
	}
	w = request(f.handler, f.admin, "GET", "/api/crm/providers/search?q=Acme", "")
	expect(t, w, 200)
	if strings.Contains(w.Body.String(), "ext-1") {
		t.Fatal("imported result not deduplicated")
	}
	w = request(f.handler, f.admin, "POST", "/api/crm/organisations/"+imported.ID+"/sync", "")
	expect(t, w, 200)
	if hits != 2 {
		t.Fatalf("fetch calls %d", hits)
	}
	w = request(f.handler, f.admin, "GET", "/api/crm/organisations/"+imported.ID+"/sync-status", "")
	expect(t, w, 200)
	if !strings.Contains(w.Body.String(), `"state":"ok"`) || !strings.Contains(w.Body.String(), `"synced_at":`) {
		t.Fatalf("missing sync success status: %s", w.Body.String())
	}
	failFetch = true
	expect(t, request(f.handler, f.admin, "POST", "/api/crm/organisations/"+imported.ID+"/sync", ""), 409)
	w = request(f.handler, f.admin, "GET", "/api/crm/organisations/"+imported.ID+"/sync-status", "")
	expect(t, w, 200)
	if !strings.Contains(w.Body.String(), `"state":"error"`) || !strings.Contains(w.Body.String(), `"error":"provider_unavailable"`) || strings.Contains(w.Body.String(), "private provider detail") || !strings.Contains(w.Body.String(), `"synced_at":`) {
		t.Fatalf("unsafe or missing sync failure status: %s", w.Body.String())
	}
	failFetch = false
	expect(t, jsonRequest(t, f, f.admin, "POST", "/api/crm/providers/http/import", map[string]any{"external_id": "ext-1"}), 409)
	w = jsonRequest(t, f, f.admin, "PUT", "/api/crm/providers/http/config", map[string]any{"enabled": false, "secret_ref": "", "expected_revision": 1})
	expect(t, w, 200)
	expect(t, request(f.handler, f.admin, "GET", "/api/crm/organisations?q=Acme", ""), 200)
	expect(t, request(f.handler, f.admin, "POST", "/api/crm/organisations/"+imported.ID+"/sync", ""), 409)
	undoEvent(t, f, eventOf(t, f, "crm.provider_config_changed"), 201)
	expect(t, request(f.handler, f.admin, "POST", "/api/crm/organisations/"+imported.ID+"/sync", ""), 200)
}

func TestProviderResolverErrorDoesNotExposeKeyMaterial(t *testing.T) {
	marker := "synthetic-credential-must-stay-private"
	provider, err := NewHTTPProvider("https://example.invalid", nil, func(context.Context, string) (string, error) {
		return "", errors.New(marker)
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Search(t.Context(), "secret://test/crm", "Customer")
	if err == nil || strings.Contains(err.Error(), marker) {
		t.Fatalf("resolver error was not sanitized: %v", err)
	}
}
func TestProviderRejectsTenantEndpointAndRedirect(t *testing.T) {
	if _, e := NewHTTPProvider("http://127.0.0.1", nil, func(context.Context, string) (string, error) { return "", nil }); e == nil {
		t.Fatal("HTTP origin accepted")
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.test/", http.StatusFound)
	}))
	defer server.Close()
	p, e := NewHTTPProvider(server.URL, server.Client(), func(context.Context, string) (string, error) { return "fake", nil })
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.Search(context.Background(), "secret://test/crm", "x"); e == nil {
		t.Fatal("redirect followed")
	}
}
func TestHubSpotFakeServerMapping(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fake" {
			t.Error("missing fake token")
		}
		switch r.URL.Path {
		case "/crm/v3/objects/companies/search":
			if r.Method != "POST" {
				t.Error("search must post")
			}
			var body struct {
				Query string `json:"query"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Query != "Acme" {
				t.Error("search query")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"id": "123", "properties": map[string]any{"name": "Acme"}}}})
		case "/crm/v3/objects/companies/123":
			if r.URL.Query().Get("properties") == "" {
				t.Error("missing properties")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "123", "properties": map[string]any{"name": "Acme", "numberofemployees": "17", "annualrevenue": "1234.56", "city": "Graz", "country": "AT"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p, e := NewHubSpotProvider(server.URL, server.Client(), func(context.Context, string) (string, error) { return "fake", nil })
	if e != nil {
		t.Fatal(e)
	}
	hits, e := p.Search(context.Background(), "secret://test/crm", "Acme")
	if e != nil || len(hits) != 1 || hits[0].ExternalID != "123" {
		t.Fatalf("search %+v %v", hits, e)
	}
	company, e := p.Fetch(context.Background(), "secret://test/crm", "123")
	if e != nil || company.Fields.EmployeeCount == nil || *company.Fields.EmployeeCount != 17 || company.Fields.AnnualRevenueMinor == nil || *company.Fields.AnnualRevenueMinor != 123456 || company.Fields.BillingAddress == nil || company.Fields.BillingAddress.City != "Graz" {
		t.Fatalf("company %+v %v", company, e)
	}
}
