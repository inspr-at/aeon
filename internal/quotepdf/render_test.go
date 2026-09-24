// SPDX-License-Identifier: AGPL-3.0-only

package quotepdf

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
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
