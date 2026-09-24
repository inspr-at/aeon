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
