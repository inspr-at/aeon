// SPDX-License-Identifier: AGPL-3.0-only

package quotepdf

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBundledDocumentRendersA4(t *testing.T) {
	if !Available() {
		t.Skip("Chromium unavailable")
	}
	assets := os.DirFS("../../web/dist")
	if _, err := os.Stat("../../web/dist/quote-print.html"); err != nil {
		t.Skip("build web assets first")
	}
	document := sampleDocument()
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	pdf, err := Render(ctx, assets, Payload{Document: document, OfferNo: "A260924-1", PublicURL: "https://example.invalid/offers/selector/token"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || len(pdf) < 1000 {
		t.Fatalf("invalid PDF (%d bytes)", len(pdf))
	}
	if !bytes.Contains(pdf, []byte("/MediaBox [0 0 594")) {
		t.Fatal("PDF is not A4-sized")
	}
}

func TestBundledDocumentRefusesWholeBlockOverflow(t *testing.T) {
	if !Available() {
		t.Skip("Chromium unavailable")
	}
	assets := os.DirFS("../../web/dist")
	if _, err := os.Stat("../../web/dist/quote-print.html"); err != nil {
		t.Skip("build web assets first")
	}
	tooLong := strings.Replace(string(sampleDocument()), `"text":"Work"`, `"text":"`+strings.Repeat("Oversized whole block. ", 1200)+`"`, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	_, err := Render(ctx, assets, Payload{Document: json.RawMessage(tooLong), OfferNo: "A260924-1"})
	if err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("expected explicit overflow, got %v", err)
	}
}

func TestSyntheticPDFGoldenTextAndGeometry(t *testing.T) {
	if !Available() {
		t.Skip("Chromium unavailable")
	}
	assets := os.DirFS("../../web/dist")
	if _, err := os.Stat("../../web/dist/quote-print.html"); err != nil {
		t.Skip("build web assets first")
	}
	document, err := os.ReadFile("../../web/tests/quotes/fixtures/synthetic-document.json")
	if err != nil {
		t.Fatal(err)
	}
	goldenBytes, err := os.ReadFile("../../web/tests/quotes/fixtures/pdf-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Width     float64  `json:"page_width_points"`
		Height    float64  `json:"page_height_points"`
		Tolerance float64  `json:"geometry_tolerance_points"`
		Required  []string `json:"required_text"`
		Forbidden []string `json:"forbidden_text"`
	}
	if err := json.Unmarshal(goldenBytes, &golden); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	pdf, err := Render(ctx, assets, Payload{Document: document, OfferNo: "A260924-01", PublicURL: "https://example.invalid/offers/synthetic/token"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || len(pdf) > 20<<20 {
		t.Fatalf("invalid PDF size %d", len(pdf))
	}
	box := regexp.MustCompile(`/MediaBox\s*\[\s*0\s+0\s+([0-9.]+)\s+([0-9.]+)\s*\]`).FindSubmatch(pdf)
	if len(box) != 3 {
		t.Fatal("missing PDF MediaBox")
	}
	width, _ := strconv.ParseFloat(string(box[1]), 64)
	height, _ := strconv.ParseFloat(string(box[2]), 64)
	if width < golden.Width-golden.Tolerance || width > golden.Width+golden.Tolerance || height < golden.Height-golden.Tolerance || height > golden.Height+golden.Tolerance {
		t.Fatalf("PDF geometry %.2f x %.2f, want %.2f x %.2f", width, height, golden.Width, golden.Height)
	}
	// Poppler checks the emitted PDF bytes when installed; source/DOM text is
	// insufficient evidence because print CSS can omit text.
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Log("pdftotext unavailable; PDF text check requires the release runner")
		return
	}
	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", "-", "-")
	cmd.Stdin = bytes.NewReader(pdf)
	textBytes, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	content := string(textBytes)
	for _, wanted := range golden.Required {
		if !strings.Contains(content, wanted) {
			t.Errorf("PDF text missing %q", wanted)
		}
	}
	for _, denied := range golden.Forbidden {
		if strings.Contains(content, denied) {
			t.Errorf("PDF text contains %q", denied)
		}
	}
}

func TestSyntheticClassicV1PDFGolden(t *testing.T) {
	if !Available() {
		t.Skip("Chromium unavailable")
	}
	if _, err := os.Stat("../../web/dist/quote-print.html"); err != nil {
		t.Skip("build web assets first")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext unavailable")
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(sampleDocument(), &document); err != nil {
		t.Fatal(err)
	}
	document["profile"] = json.RawMessage(`{
	  "id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","revision":1,"definition":{
	    "schema":"inspr.document-profile.v1","layout_variant":"classic-v1","locale":"de-AT","fonts":[],
	    "colors":{"ink":"#1e2727","muted":"#6f7d7b","soft":"#a3aeac","accent":"#55d0c0","rule":"#dfe6e5","paper":"#ffffff"},
	    "typography":{"body_pt":"10","title_pt":"17","section_pt":"18","table_pt":"9.6","footer_pt":"7.5"},
	    "page":{"width_mm":"210","height_mm":"297","top_mm":"18","right_mm":"20","bottom_mm":"16","left_mm":"22"},
	    "cover":{"top_mm":"11","title_gap_mm":"7","columns_gap_mm":"8","columns_padding_mm":"6"},
	    "sections":{"numbering":"upper-roman","heading_case":"upper"},
	    "positions_table":{"columns":[{"key":"position","width_mm":"9"},{"key":"description","width_mm":"71"},{"key":"quantity","width_mm":"15"},{"key":"unit","width_mm":"22"},{"key":"unit_price","width_mm":"24"},{"key":"total","width_mm":"27"}],"separator":"rule","repeat_header":true},
	    "totals":{"vat":"note","discount":"hidden","net_label":"Net total"},
	    "payment_terms":{"position":"sections","heading":"Payment"},
	    "acceptance":{"signature_columns":2,"gap_mm":"14","lead_mm":"28"},
	    "footer":{"width_mm":"33","offset_mm":"0","page_number_format":"PAGE {page} OF {total}"},
	    "labels":{"quote":"QUOTE","terms":"TERMS","positions":"ITEMS","signature_customer":"Customer signature","signature_sender":"Studio signature"}
	  }
	}`)
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	pdf, err := Render(ctx, os.DirFS("../../web/dist"), Payload{Document: raw, OfferNo: "S-001"})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", "-", "-")
	cmd.Stdin = bytes.NewReader(pdf)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	pages := strings.Split(strings.TrimSuffix(string(output), "\f"), "\f")
	if len(pages) != 2 {
		t.Fatalf("classic golden pages = %d, want 2", len(pages))
	}
	for _, wanted := range []string{"Synthetic quote", "I. TERMS", "Scope", "PAGE 1 OF 2"} {
		if !strings.Contains(pages[0], wanted) {
			t.Errorf("classic cover missing %q", wanted)
		}
	}
	for _, wanted := range []string{"II. ITEMS", "01", "Service", "Net total", "€ 1,00", "PAGE 2 OF 2"} {
		if !strings.Contains(pages[1], wanted) {
			t.Errorf("classic positions missing %q", wanted)
		}
	}
}

func sampleDocument() json.RawMessage {
	return json.RawMessage(`{
	  "schema_version":1,"minimum_writer_version":1,"title":"Synthetic quote","subtitle":"A neutral example","project_ref":"TEST-1",
	  "offer_date":"2026-09-24","valid_until":"2026-10-24","currency":"EUR",
	  "sender":{"company":"Example Studio","street":"Sample Lane 1","postal_code":"1000","city":"Test City","country":"AT"},
	  "recipient":{"name":"Sample Customer","address":"Demo Street 2"},
	  "legal":{"intro":"A test introduction.","accept_text":"I accept this offer.","vat_note":"Tax according to agreed terms."},
	  "layout":{},
	  "sections":[{"id":"11111111-1111-4111-8111-111111111111","heading":"Scope","body":"Work","nodes":[{"id":"22222222-2222-4222-8222-222222222222","kind":"paragraph","text":"Work"}]}],
	  "positions":[{"id":"33333333-3333-4333-8333-333333333333","pricing_source":"manual","short_text":"Service","long_text":"Synthetic service","quantity":"1.00","unit_label":"item","unit_price_cents":100,"total_cents":100,"currency":"EUR"}],
	  "net_total_cents":100
	}`)
}
