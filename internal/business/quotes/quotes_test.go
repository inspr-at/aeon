// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestQuoteSectionFormattingContract(t *testing.T) {
	pageBreak := true
	keepTogether := false
	base := quoteDocument{
		SchemaVersion: 1, MinimumWriterVersion: 1, Title: "Example", OfferDate: "2026-09-24", ValidUntil: "2026-10-24", Currency: "EUR",
		Sender: json.RawMessage(`{}`), Recipient: json.RawMessage(`{}`), Legal: json.RawMessage(`{}`), Layout: json.RawMessage(`{}`),
		Sections: []documentSection{{ID: "11111111-1111-4111-8111-111111111111", Heading: "Scope", Body: "Text", Nodes: []textNode{},
			NumberingStyle: "upper-roman", PageBreakBefore: &pageBreak, KeepTogether: &keepTogether, SpacingBeforeMM: "3.5", SpacingAfterMM: "2.0"}},
		Positions: []documentPosition{},
	}
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDocument(&decoded, false); err != nil {
		t.Fatal(err)
	}
	section := decoded.Sections[0]
	if section.NumberingStyle != "upper-roman" || section.PageBreakBefore == nil || !*section.PageBreakBefore || section.KeepTogether == nil || *section.KeepTogether || section.SpacingBeforeMM != "3.5" || section.SpacingAfterMM != "2.0" {
		t.Fatalf("section formatting lost on document round trip: %+v", section)
	}
	for _, change := range []struct {
		name string
		edit func(*documentSection)
	}{
		{"numbering", func(s *documentSection) { s.NumberingStyle = "octal" }},
		{"spacing precision", func(s *documentSection) { s.SpacingBeforeMM = "1.25" }},
		{"spacing range", func(s *documentSection) { s.SpacingAfterMM = "40.1" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			candidate := decoded
			candidate.Sections = append([]documentSection(nil), decoded.Sections...)
			change.edit(&candidate.Sections[0])
			if err := validateDocument(&candidate, false); err == nil {
				t.Fatal("invalid section formatting accepted")
			}
		})
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
	status, _ = call("customer", "GET", "/api/quotes", "")
	if status != 403 {
		t.Fatalf("customer listed tenant quotes: %d", status)
	}
	status, _ = call("customer", "GET", "/api/quotes/"+quoteID, "")
	if status != 200 {
		t.Fatalf("bound customer cannot read quote: %d", status)
	}
	status, _ = call("agent", "GET", "/api/quotes/"+quoteID, "")
	if status != 403 {
		t.Fatalf("agent read quote: %d", status)
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
	// P1 settings, customer-only numbered drafts and exact cent arithmetic.
	settingsBody := `{"expected_revision":0,"numbering_time_zone":"Europe/Vienna","default_currency":"EUR","sender":{"company":"Example Sender","street":"Example Street 1","postal_code":"0000","city":"Example City","country":"AT","email":"sender@example.test"},"defaults":{"intro":"","blocks":[],"accept_text":"","vat_note":""},"layout":{},"smtp_confirmation_enabled":false}`
	status, _ = call("agent", "PATCH", "/api/quotes/settings", settingsBody)
	if status != 403 {
		t.Fatalf("agent changed settings: %d", status)
	}
	status, settings := call("admin", "PATCH", "/api/quotes/settings", settingsBody)
	if status != 200 || settings["revision"] != json.Number("1") {
		t.Fatalf("settings %d %v", status, settings)
	}
	status, _ = call("admin", "PATCH", "/api/quotes/settings", settingsBody)
	if status != 409 {
		t.Fatalf("stale settings revision %d", status)
	}
	const writers = 8
	var wg sync.WaitGroup
	created := make([]map[string]any, writers)
	createStatuses := make([]int, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			createStatuses[i], created[i] = call("admin", "POST", "/api/quotes", fmt.Sprintf(`{"title":"Example %d","customer_org_node_id":%q}`, i, ids["org"]))
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for i, q := range created {
		if createStatuses[i] != 201 {
			t.Fatalf("concurrent create %d: %d %v", i, createStatuses[i], q)
		}
		number := q["offer_no"].(string)
		if seen[number] {
			t.Fatalf("duplicate offer number %s", number)
		}
		seen[number] = true
		if q["project_node_id"] != "" {
			t.Fatalf("customer-only quote has project %v", q)
		}
	}
	if len(seen) != writers {
		t.Fatal("missing commercial numbers")
	}
	pageReq := httptest.NewRequest("GET", "/api/quotes?limit=3", nil)
	pageReq = pageReq.WithContext(tenant.WithPrincipal(pageReq.Context(), as("admin")))
	pageRec := httptest.NewRecorder()
	mux.ServeHTTP(pageRec, pageReq)
	cursor := pageRec.Header().Get("X-Next-Cursor")
	var pageOne []map[string]any
	_ = json.Unmarshal(pageRec.Body.Bytes(), &pageOne)
	if pageRec.Code != 200 || len(pageOne) != 3 || cursor == "" {
		t.Fatalf("quote first page %d %s", pageRec.Code, pageRec.Body.String())
	}
	pageReq = httptest.NewRequest("GET", "/api/quotes?limit=3&cursor="+cursor, nil)
	pageReq = pageReq.WithContext(tenant.WithPrincipal(pageReq.Context(), as("admin")))
	pageRec = httptest.NewRecorder()
	mux.ServeHTTP(pageRec, pageReq)
	var pageTwo []map[string]any
	_ = json.Unmarshal(pageRec.Body.Bytes(), &pageTwo)
	if pageRec.Code != 200 || len(pageTwo) != 3 || pageOne[2]["quote_node_id"] == pageTwo[0]["quote_node_id"] {
		t.Fatalf("quote second page %d %s", pageRec.Code, pageRec.Body.String())
	}
	saveRaceID := created[2]["quote_node_id"].(string)
	_, raceDraft := call("admin", "GET", "/api/quotes/"+saveRaceID+"/draft", "")
	raceStatuses := make([]int, 2)
	raceResponses := make([]map[string]any, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			docCopy := map[string]any{}
			raw, _ := json.Marshal(raceDraft["document"])
			_ = json.Unmarshal(raw, &docCopy)
			docCopy["title"] = fmt.Sprintf("Concurrent %d", i)
			body, _ := json.Marshal(map[string]any{"client_session_id": "88888888-8888-4888-8888-888888888888", "mutation_id": fmt.Sprintf("99999999-9999-4999-8999-99999999999%d", i), "writer_version": 1, "document": docCopy})
			req := httptest.NewRequest("PATCH", "/api/quotes/"+saveRaceID+"/draft", strings.NewReader(string(body)))
			req.Header.Set("If-Match", `"qd-1"`)
			req = req.WithContext(tenant.WithPrincipal(req.Context(), as("admin")))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			raceStatuses[i] = rec.Code
			_ = json.Unmarshal(rec.Body.Bytes(), &raceResponses[i])
		}(i)
	}
	wg.Wait()
	if !((raceStatuses[0] == 200 && raceStatuses[1] == 412) || (raceStatuses[1] == 200 && raceStatuses[0] == 412)) {
		t.Fatalf("concurrent draft saves %v %v", raceStatuses, raceResponses)
	}
	var customerCount int
	e = db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM crm_customer_numbers WHERE organisation_node_id=$1::uuid`, ids["org"]).Scan(&customerCount)
	})
	if e != nil || customerCount != 1 {
		t.Fatalf("customer number race: %d %v", customerCount, e)
	}
	newID := created[0]["quote_node_id"].(string)
	status, draft := call("admin", "GET", "/api/quotes/"+newID+"/draft", "")
	if status != 200 {
		t.Fatalf("draft get %d %v", status, draft)
	}
	doc := draft["document"].(map[string]any)
	recipient := doc["recipient"].(map[string]any)
	recipient["address"] = "Example Address"
	recipient["email"] = "customer@example.test"
	doc["positions"] = []any{map[string]any{"id": "11111111-1111-4111-8111-111111111111", "pricing_source": "manual", "short_text": "Work", "long_text": "Description", "quantity": "1.5", "unit_label": "item", "unit_price_cents": 9999, "total_cents": 0, "currency": "EUR"}}
	patchBody, _ := json.Marshal(map[string]any{"client_session_id": "22222222-2222-4222-8222-222222222222", "mutation_id": "33333333-3333-4333-8333-333333333333", "writer_version": 1, "document": doc})
	patch := func(body []byte, tag string) (int, map[string]any) {
		req := httptest.NewRequest("PATCH", "/api/quotes/"+newID+"/draft", strings.NewReader(string(body)))
		if tag != "" {
			req.Header.Set("If-Match", tag)
		}
		req = req.WithContext(tenant.WithPrincipal(req.Context(), as("admin")))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var value map[string]any
		dec := json.NewDecoder(strings.NewReader(rec.Body.String()))
		dec.UseNumber()
		_ = dec.Decode(&value)
		return rec.Code, value
	}
	status, _ = patch(patchBody, "")
	if status != 428 {
		t.Fatalf("missing precondition %d", status)
	}
	status, receipt := patch(patchBody, `"qd-1"`)
	if status != 200 || receipt["acknowledged_revision"] != json.Number("2") {
		t.Fatalf("draft save %d %v", status, receipt)
	}
	position := receipt["document"].(map[string]any)["positions"].([]any)[0].(map[string]any)
	if position["total_cents"] != json.Number("14999") {
		t.Fatalf("cent rounding %v", position)
	}
	status, replay := patch(patchBody, `"qd-1"`)
	if status != 200 || replay["replayed"] != true {
		t.Fatalf("receipt replay %d %v", status, replay)
	}
	status, _ = patch(patchBody, `"qd-1"`)
	if status != 200 {
		t.Fatalf("receipt before stale check %d", status)
	}
	other := strings.Replace(string(patchBody), `"Work"`, `"Different"`, 1)
	status, _ = patch([]byte(other), `"qd-1"`)
	if status != 409 {
		t.Fatalf("mutation reuse %d", status)
	}
	other = strings.Replace(other, "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", 1)
	status, _ = patch([]byte(other), `"qd-1"`)
	if status != 412 {
		t.Fatalf("stale draft %d", status)
	}
	status, quoteNow := call("admin", "GET", "/api/quotes/"+newID, "")
	if status != 200 {
		t.Fatalf("quote get %d", status)
	}
	finalBody := fmt.Sprintf(`{"expected_quote_revision":%s,"expected_draft_revision":2,"expected_document_sha256":%q}`, quoteNow["revision"], receipt["document_sha256"])
	finalStatuses := make([]int, 2)
	finalResponses := make([]map[string]any, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			finalStatuses[i], finalResponses[i] = call("admin", "POST", "/api/quotes/"+newID+"/finalize", finalBody)
		}(i)
	}
	wg.Wait()
	if !((finalStatuses[0] == 200 && finalStatuses[1] == 409) || (finalStatuses[1] == 200 && finalStatuses[0] == 409)) {
		t.Fatalf("concurrent finalize %v %v", finalStatuses, finalResponses)
	}
	status, vDoc := call("admin", "GET", "/api/quotes/"+newID+"/versions/1", "")
	if status != 200 || vDoc["pricing_mode"] != "cent-half-up-v1" || vDoc["total"] != json.Number("149.9900") {
		t.Fatalf("frozen document %d %v", status, vDoc)
	}
	rateQuoteID := created[1]["quote_node_id"].(string)
	status, rateDraft := call("admin", "GET", "/api/quotes/"+rateQuoteID+"/draft", "")
	if status != 200 {
		t.Fatalf("rate draft %d %v", status, rateDraft)
	}
	rateDoc := rateDraft["document"].(map[string]any)
	rateRecipient := rateDoc["recipient"].(map[string]any)
	rateRecipient["address"] = "Example Address"
	rateRecipient["email"] = "customer@example.test"
	rateDoc["positions"] = []any{map[string]any{"id": "55555555-5555-4555-8555-555555555555", "pricing_source": "cost_unit", "short_text": "Rate work", "long_text": "", "quantity": "1.5", "unit_label": "hour", "unit_price_cents": 0, "total_cents": 0, "cost_unit_node_id": ids["cost"], "rate_unit": "hour", "currency": "EUR"}}
	rateBody, _ := json.Marshal(map[string]any{"client_session_id": "66666666-6666-4666-8666-666666666666", "mutation_id": "77777777-7777-4777-8777-777777777777", "writer_version": 1, "document": rateDoc})
	rateReq := httptest.NewRequest("PATCH", "/api/quotes/"+rateQuoteID+"/draft", strings.NewReader(string(rateBody)))
	rateReq.Header.Set("If-Match", `"qd-1"`)
	rateReq = rateReq.WithContext(tenant.WithPrincipal(rateReq.Context(), as("admin")))
	rateRec := httptest.NewRecorder()
	mux.ServeHTTP(rateRec, rateReq)
	var rateReceipt map[string]any
	rateDec := json.NewDecoder(strings.NewReader(rateRec.Body.String()))
	rateDec.UseNumber()
	_ = rateDec.Decode(&rateReceipt)
	if rateRec.Code != 200 {
		t.Fatalf("rate draft save %d %v", rateRec.Code, rateReceipt)
	}
	_, rateQuote := call("admin", "GET", "/api/quotes/"+rateQuoteID, "")
	rateFinal := fmt.Sprintf(`{"expected_quote_revision":%s,"expected_draft_revision":2,"expected_document_sha256":%q}`, rateQuote["revision"], rateReceipt["document_sha256"])
	status, _ = call("admin", "POST", "/api/quotes/"+rateQuoteID+"/finalize", rateFinal)
	if status != 200 {
		t.Fatalf("rate finalize %d", status)
	}
	status, rateVersion := call("admin", "GET", "/api/quotes/"+rateQuoteID+"/versions/1", "")
	if status != 200 || rateVersion["total"] != json.Number("29.9900") {
		t.Fatalf("rate version %d %v", status, rateVersion)
	}
	_, rateIssued := call("admin", "GET", "/api/quotes/"+rateQuoteID, "")
	branchBody := fmt.Sprintf(`{"expected_quote_revision":%s,"expected_version":1,"expected_content_sha256":%q}`, rateIssued["revision"], rateVersion["content_sha256"])
	status, branch := call("admin", "POST", "/api/quotes/"+rateQuoteID+"/draft/branch", branchBody)
	if status != 200 || branch["base_version"] != json.Number("1") {
		t.Fatalf("document branch %d %v", status, branch)
	}
	_, branchedQuote := call("admin", "GET", "/api/quotes/"+rateQuoteID, "")
	branchFinal := fmt.Sprintf(`{"expected_quote_revision":%s,"expected_draft_revision":%s,"expected_document_sha256":%q}`, branchedQuote["revision"], branch["draft_revision"], branch["document_sha256"])
	status, _ = call("admin", "POST", "/api/quotes/"+rateQuoteID+"/finalize", branchFinal)
	if status != 200 {
		t.Fatalf("branch finalization %d", status)
	}
	status, rateVersion2 := call("admin", "GET", "/api/quotes/"+rateQuoteID+"/versions/2", "")
	if status != 200 || rateVersion2["total"] != rateVersion["total"] || rateVersion2["content_sha256"] == rateVersion["content_sha256"] {
		t.Fatalf("branch version %d %v", status, rateVersion2)
	}
	var rateAmount string
	var rateCentsStored int64
	e = db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT rate_amount::text,total_cents FROM quote_document_lines WHERE quote_node_id=$1::uuid AND version=1`, rateQuoteID).Scan(&rateAmount, &rateCentsStored)
	})
	if e != nil || rateAmount != "19.9900" || rateCentsStored != 2999 {
		t.Fatalf("rate snapshot %s %d %v", rateAmount, rateCentsStored, e)
	}
	settingsNext := strings.Replace(settingsBody, `"expected_revision":0`, `"expected_revision":1`, 1)
	settingsNext = strings.Replace(settingsNext, "Example Sender", "Changed Sender", 1)
	settingsStatuses := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			settingsStatuses[i], _ = call("admin", "PATCH", "/api/quotes/settings", settingsNext)
		}(i)
	}
	wg.Wait()
	if !((settingsStatuses[0] == 200 && settingsStatuses[1] == 409) || (settingsStatuses[1] == 200 && settingsStatuses[0] == 409)) {
		t.Fatalf("settings CAS %v", settingsStatuses)
	}
	status, vAfterSettings := call("admin", "GET", "/api/quotes/"+newID+"/versions/1", "")
	if status != 200 || vAfterSettings["document"].(map[string]any)["sender"].(map[string]any)["company"] != "Example Sender" {
		t.Fatalf("historical sender changed: %d %v", status, vAfterSettings)
	}
	status, _ = patch([]byte(other), `"qd-2"`)
	if status != 409 {
		t.Fatalf("save after issue %d", status)
	}
	status, _ = call("admin", "POST", "/api/quotes/"+newID+"/duplicate", fmt.Sprintf(`{"expected_revision":%s}`, quoteNow["revision"]))
	if status != 409 {
		t.Fatalf("stale duplicate revision %d", status)
	}
	status, issuedNow := call("admin", "GET", "/api/quotes/"+newID, "")
	status, duplicated := call("admin", "POST", "/api/quotes/"+newID+"/duplicate", fmt.Sprintf(`{"expected_revision":%s}`, issuedNow["revision"]))
	if status != 201 || duplicated["offer_no"] == issuedNow["offer_no"] || duplicated["state"] != "draft" {
		t.Fatalf("duplicate %d %v", status, duplicated)
	}
	status, archived := call("admin", "PATCH", "/api/quotes/"+newID+"/visibility", fmt.Sprintf(`{"expected_revision":%s,"archived":true}`, issuedNow["revision"]))
	if status != 200 || archived["archived"] != true {
		t.Fatalf("archive %d %v", status, archived)
	}
	var legacyOrg string
	e = db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title) VALUES($1::uuid,'ORG-999',$2::uuid,'Legacy Example') RETURNING id::text`, tenantID, ids["organisation_kind"]).Scan(&legacyOrg); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO crm_customer_numbers(tenant_id,organisation_node_id,customer_no,provenance) VALUES($1::uuid,$2::uuid,'K25-001','imported')`, tenantID, legacyOrg)
		return err
	})
	if e != nil {
		t.Fatal(e)
	}
	status, legacyQuote := call("admin", "POST", "/api/quotes", fmt.Sprintf(`{"title":"Legacy Draft","customer_org_node_id":%q}`, legacyOrg))
	if status != 201 {
		t.Fatalf("legacy draft %d %v", status, legacyQuote)
	}
	converted, e := ReformatLegacyCustomerNumber(ctx, database.App, reg, as("admin"), legacyOrg, "K25-001")
	if e != nil || !strings.HasPrefix(converted, "K") {
		t.Fatalf("legacy conversion %q %v", converted, e)
	}
	_, legacyDraft := call("admin", "GET", "/api/quotes/"+legacyQuote["quote_node_id"].(string)+"/draft", "")
	if legacyDraft["draft_revision"] != json.Number("2") || legacyDraft["document"].(map[string]any)["recipient"].(map[string]any)["customer_no"] != converted {
		t.Fatalf("conversion did not update draft %v", legacyDraft)
	}
	_, e = ReformatLegacyCustomerNumber(ctx, database.App, reg, as("admin"), legacyOrg, "K25-001")
	if e == nil {
		t.Fatal("legacy conversion replay changed number")
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
