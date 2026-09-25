// SPDX-License-Identifier: AGPL-3.0-only
package offers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/business/quotes"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestImportedDraftRoundTrip(t *testing.T) {
	// An operator may point this test at a private export. The committed test
	// always uses an invented bundle and never stores customer content.
	var bundle Bundle
	if path := os.Getenv("AEON_FX3_BUNDLE_DIR"); path != "" {
		var err error
		bundle, err = ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		bundle = Bundle{
			Customers: []Customer{{ID: 17, Name: "Synthetic Buyer", CustomerNo: "K26011", UpdatedAt: "2026-01-01 09:00:00"}},
			Contacts:  []Contact{{ID: 29, CustomerID: 17, Name: "Sample Person", Email: "sample@example.invalid", IsPrimary: true, UpdatedAt: "2026-01-01 09:00:00"}},
			Offers:    []Offer{{ID: 41, CustomerID: 17, OfferNo: "A260101-01", Revision: 3, Status: "draft", Document: json.RawMessage(`{"title":"Synthetic offer","subtitle":"","project_ref":"TEST-1","offer_date":"2026-01-01","valid_until":"2026-12-31","sender":{"company":"Example GmbH"},"customer":{"name":"Synthetic Buyer","address":"Testweg 1","contact":"Sample Person","country":"AT","customer_no":"K26011","email":"sample@example.invalid"},"intro":"Introduction","accept_text":"Acceptance","vat_note":"VAT additional","footer":{"logo_width_mm":33,"logo_offset_mm":0},"blocks":[{"heading":"Scope","body":"Service terms","nodes":[{"kind":"item","text":"First item","marker":"decimal","numbering":"outline","section_bound":true,"depth":1,"marker_x_mm":3,"text_start_mm":1.5}]}],"positions":[{"short_text":"Synthetic item","long_text":"Sample scope","quantity":1.25,"unit":"item","unit_price_cents":101,"total_cents":126}],"net_total_cents":126}`)}},
			Settings:  json.RawMessage(`{"sender":{},"defaults":{}}`), Branding: json.RawMessage(`{"name":"Synthetic"}`),
		}
	}
	database := dbtest.Open(t)
	ctx := context.Background()
	tenantID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	reg := plugins.NewRegistry()
	for _, id := range []string{"business_costs", "business_crm"} {
		plugin := plugins.Plugin{Manifest: plugins.Manifest{ID: id, Version: "1", Owner: "aeon", Permissions: []string{fence.PermNodesContribute}}}
		digest, err := plugins.Digest(plugin)
		if err != nil {
			t.Fatal(err)
		}
		plugin.Manifest.DigestSHA256 = digest
		if err := reg.Register(plugin); err != nil {
			t.Fatal(err)
		}
	}
	quotePlugin, err := quotes.ManifestPlugin()
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(quotePlugin); err != nil {
		t.Fatal(err)
	}
	reg.Seal()
	var adminID string
	if err := db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO tenants(id,slug,name) VALUES($1::uuid,'fx3-test','FX3 test')`, tenantID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,'person','Synthetic Admin',ARRAY['admin']) RETURNING id::text`, tenantID).Scan(&adminID); err != nil {
			return err
		}
		for slug, prefix := range map[string]string{"organisation": "ORG", "contact": "CON", "quote": "QUO"} {
			if _, err := tx.Exec(ctx, `INSERT INTO node_kinds(tenant_id,slug,label,short_prefix,icon) VALUES($1::uuid,$2,$2,$3,$2)`, tenantID, slug, prefix); err != nil {
				return err
			}
		}
		for _, id := range []string{"business_costs", "business_crm", quotes.PluginID} {
			plugin, _ := reg.Lookup(id)
			if _, err := tx.Exec(ctx, `INSERT INTO plugin_installations(tenant_id,plugin_id,version,manifest_digest_sha256,owner,enabled,permissions,updated_by_principal_id) VALUES($1::uuid,$2,$3,$4,$5,true,$6,$7::uuid)`, tenantID, id, plugin.Manifest.Version, plugin.Manifest.DigestSHA256, plugin.Manifest.Owner, plugin.Manifest.Permissions, adminID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	report, err := Import(ctx, database.App, tenantID, adminID, "synthetic", bundle, true)
	if err != nil {
		t.Fatal(err)
	}
	module, err := quotes.New(database.App, reg)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	module.Mount(mux)
	principal := tenant.Principal{TenantID: tenantID, ID: adminID, Kind: tenant.Person, Roles: []string{"admin"}}
	for _, mapping := range report.Mappings {
		if mapping.SourceKind != "offer" || mapping.Action != "create" {
			continue
		}
		editable := false
		for _, offer := range bundle.Offers {
			if mapping.SourceID == strconv.FormatInt(offer.ID, 10) && offer.Status == "draft" {
				editable = true
			}
		}
		if !editable {
			continue
		}
		path := "/api/quotes/" + mapping.NodeID + "/draft"
		request := httptest.NewRequest("GET", path, nil)
		request = request.WithContext(tenant.WithPrincipal(request.Context(), principal))
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("GET draft %d: %s", response.Code, response.Body.String())
		}
		var draft struct {
			Document      json.RawMessage `json:"document"`
			DraftRevision int64           `json:"draft_revision"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &draft); err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := json.Unmarshal(draft.Document, &document); err != nil {
			t.Fatal(err)
		}
		document["title"] = document["title"].(string) + " revised"
		body, err := json.Marshal(map[string]any{"client_session_id": "11111111-1111-4111-8111-111111111111", "mutation_id": "22222222-2222-4222-8222-222222222222", "writer_version": 2, "document": document})
		if err != nil {
			t.Fatal(err)
		}
		request = httptest.NewRequest("PATCH", path, strings.NewReader(string(body)))
		request.Header.Set("If-Match", fmt.Sprintf(`"qd-%d"`, draft.DraftRevision))
		request = request.WithContext(tenant.WithPrincipal(request.Context(), principal))
		response = httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("PATCH draft %d: %s", response.Code, response.Body.String())
		}
		if os.Getenv("AEON_FX3_BUNDLE_DIR") != "" {
			continue
		}
		var receipt struct {
			CurrentRevision int64 `json:"current_revision"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
			t.Fatal(err)
		}
		// A malformed editor request must identify the field in the 400 body.
		sections := document["sections"].([]any)
		node := sections[0].(map[string]any)["nodes"].([]any)[0].(map[string]any)
		node["marker_x_mm"] = 3
		invalid, err := json.Marshal(map[string]any{"client_session_id": "11111111-1111-4111-8111-111111111111", "mutation_id": "33333333-3333-4333-8333-333333333333", "writer_version": 2, "document": document})
		if err != nil {
			t.Fatal(err)
		}
		request = httptest.NewRequest("PATCH", path, strings.NewReader(string(invalid)))
		request.Header.Set("If-Match", fmt.Sprintf(`"qd-%d"`, receipt.CurrentRevision))
		request = request.WithContext(tenant.WithPrincipal(request.Context(), principal))
		response = httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		var refused struct {
			Errors map[string]string `json:"errors"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &refused); err != nil {
			t.Fatal(err)
		}
		if response.Code != 400 || refused.Errors["document.sections[0].nodes[0].marker_x_mm"] != "must be string" {
			t.Fatalf("field validation %d: %s", response.Code, response.Body.String())
		}
		// Simulate the pre-fix imported row, including its import event, then
		// exercise the operator repair twice and save the repaired draft.
		if err := db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE quote_drafts SET document=jsonb_set(document,'{sections,0,nodes,0,marker_x_mm}','3'::jsonb) WHERE quote_node_id=$1::uuid`, mapping.NodeID); err != nil {
				return err
			}
			_, err := events.Append(ctx, tx, principal, events.Change{NodeID: &mapping.NodeID, Type: "quote.draft_imported", After: map[string]any{"source_revision": 3}})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		fixed, err := RepairDraftDimensions(ctx, database.App, tenantID, adminID, "synthetic")
		if err != nil || fixed.Repaired != 1 {
			t.Fatalf("repair: %+v %v", fixed, err)
		}
		again, err := RepairDraftDimensions(ctx, database.App, tenantID, adminID, "synthetic")
		if err != nil || again.Repaired != 0 {
			t.Fatalf("repeat repair: %+v %v", again, err)
		}
		request = httptest.NewRequest("GET", path, nil)
		request = request.WithContext(tenant.WithPrincipal(request.Context(), principal))
		response = httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("GET repaired draft %d", response.Code)
		}
		if err := json.Unmarshal(response.Body.Bytes(), &draft); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(draft.Document), `"marker_x_mm": "3"`) && !strings.Contains(string(draft.Document), `"marker_x_mm":"3"`) {
			t.Fatal("repair did not write exact measurement string")
		}
		body, err = json.Marshal(map[string]any{"client_session_id": "11111111-1111-4111-8111-111111111111", "mutation_id": "44444444-4444-4444-8444-444444444444", "writer_version": 2, "document": json.RawMessage(draft.Document)})
		if err != nil {
			t.Fatal(err)
		}
		request = httptest.NewRequest("PATCH", path, strings.NewReader(string(body)))
		request.Header.Set("If-Match", fmt.Sprintf(`"qd-%d"`, draft.DraftRevision))
		request = request.WithContext(tenant.WithPrincipal(request.Context(), principal))
		response = httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != 200 {
			t.Fatalf("PATCH repaired draft %d: %s", response.Code, response.Body.String())
		}
	}
}
