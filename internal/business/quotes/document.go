// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type textMark struct {
	Start  int  `json:"start"`
	End    int  `json:"end"`
	Bold   bool `json:"bold,omitempty"`
	Italic bool `json:"italic,omitempty"`
}
type textNode struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	Text         string     `json:"text"`
	Depth        int        `json:"depth,omitempty"`
	Marker       string     `json:"marker,omitempty"`
	Numbering    string     `json:"numbering,omitempty"`
	ListStart    int        `json:"list_start,omitempty"`
	ListContinue bool       `json:"list_continue,omitempty"`
	SectionBound bool       `json:"section_bound,omitempty"`
	Glyph        string     `json:"glyph,omitempty"`
	MarkerXMM    string     `json:"marker_x_mm,omitempty"`
	MarkerYMM    string     `json:"marker_y_mm,omitempty"`
	TextStartMM  string     `json:"text_start_mm,omitempty"`
	Marks        []textMark `json:"marks,omitempty"`
}
type documentSection struct {
	ID              string     `json:"id"`
	Heading         string     `json:"heading"`
	Body            string     `json:"body"`
	Nodes           []textNode `json:"nodes"`
	NumberingStyle  string     `json:"numbering_style,omitempty"`
	PageBreakBefore *bool      `json:"page_break_before,omitempty"`
	KeepTogether    *bool      `json:"keep_together,omitempty"`
	SpacingBeforeMM string     `json:"spacing_before_mm,omitempty"`
	SpacingAfterMM  string     `json:"spacing_after_mm,omitempty"`
}
type documentPosition struct {
	ID             string `json:"id"`
	PricingSource  string `json:"pricing_source"`
	ShortText      string `json:"short_text"`
	LongText       string `json:"long_text"`
	Quantity       string `json:"quantity"`
	UnitLabel      string `json:"unit_label"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	TotalCents     int64  `json:"total_cents"`
	CostUnitNodeID string `json:"cost_unit_node_id,omitempty"`
	RateUnit       string `json:"rate_unit,omitempty"`
	Currency       string `json:"currency"`
}
type quoteDocument struct {
	SchemaVersion        int                      `json:"schema_version"`
	MinimumWriterVersion int                      `json:"minimum_writer_version"`
	Title                string                   `json:"title"`
	Subtitle             string                   `json:"subtitle"`
	ProjectRef           string                   `json:"project_ref"`
	OfferDate            string                   `json:"offer_date"`
	ValidUntil           string                   `json:"valid_until"`
	Currency             string                   `json:"currency"`
	Sender               json.RawMessage          `json:"sender"`
	Recipient            json.RawMessage          `json:"recipient"`
	Legal                json.RawMessage          `json:"legal"`
	Layout               json.RawMessage          `json:"layout"`
	Profile              *documentProfileSnapshot `json:"profile,omitempty"`
	Sections             []documentSection        `json:"sections"`
	Positions            []documentPosition       `json:"positions"`
	NetTotalCents        int64                    `json:"net_total_cents"`
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func decodeDocument(raw []byte) (quoteDocument, error) {
	var d quoteDocument
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&d); err != nil {
		return d, bad("invalid document schema")
	}
	if d.SchemaVersion != 1 || d.MinimumWriterVersion < 1 || d.MinimumWriterVersion > 2 {
		return d, conflict("unsupported document writer")
	}
	if !jsonObject(d.Sender) || !jsonObject(d.Recipient) || !jsonObject(d.Legal) || !jsonObject(d.Layout) {
		return d, bad("invalid document snapshots")
	}
	if err := validateSnapshotFields(d.Sender, senderFields); err != nil {
		return d, err
	}
	if err := validateSnapshotFields(d.Recipient, recipientFields); err != nil {
		return d, err
	}
	if err := validateSnapshotFields(d.Legal, legalFields); err != nil {
		return d, err
	}
	if err := validateSnapshotFields(d.Layout, layoutFields); err != nil {
		return d, err
	}
	if d.Profile != nil && (!uuidRe.MatchString(d.Profile.ID) || d.Profile.Revision < 1 || d.Profile.Definition.Schema != "inspr.document-profile.v1") {
		return d, bad("invalid document profile snapshot")
	}
	return d, nil
}

// Inline marks are part of the full-document write contract introduced by
// writer 2. Keep the requirement after all marks are removed so a stale older
// editor cannot later replace a marked document with an unformatted copy.
func documentMinimumWriterVersion(d quoteDocument) int {
	if d.MinimumWriterVersion > 1 {
		return d.MinimumWriterVersion
	}
	for _, section := range d.Sections {
		for _, node := range section.Nodes {
			if len(node.Marks) > 0 {
				return 2
			}
		}
	}
	return 1
}

var senderFields = map[string]bool{"company": true, "street": true, "postal_code": true, "city": true, "country": true, "register_no": true, "register_court": true, "email": true, "phone": true, "website": true, "uid": true, "bank_name": true, "iban": true, "bic": true, "contact_person": true, "logo_file_id": true, "logo_sha256": true}
var recipientFields = map[string]bool{"name": true, "address": true, "contact": true, "country": true, "customer_no": true, "email": true, "contact_node_id": true}
var legalFields = map[string]bool{"intro": true, "accept_text": true, "vat_note": true, "discount_note": true, "payment_terms": true}
var layoutFields = map[string]bool{"logo_width_mm": true, "logo_offset_mm": true, "logo_file_id": true, "logo_sha256": true, "page_style": true}

func validateSnapshotFields(raw json.RawMessage, allowed map[string]bool) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return bad("invalid document snapshot")
	}
	for name, value := range fields {
		if !allowed[name] {
			return bad("unsupported document snapshot field")
		}
		var s string
		if err := json.Unmarshal(value, &s); err != nil || len(s) > 100000 {
			return bad("invalid document snapshot field")
		}
		if allowed["logo_width_mm"] {
			switch name {
			case "logo_width_mm":
				if _, err := parseMM(s, 180, 960); err != nil {
					return err
				}
			case "logo_offset_mm":
				if _, err := parseMM(s, -60, 100); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func parseMM(s string, min, max int64) (int64, error) {
	if s == "" {
		return 0, nil
	}
	sign := int64(1)
	if strings.HasPrefix(s, "-") {
		sign = -1
		s = s[1:]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts[0]) > 3 {
		return 0, bad("invalid millimetres")
	}
	for _, part := range parts {
		if part == "" {
			return 0, bad("invalid millimetres")
		}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return 0, bad("invalid millimetres")
			}
		}
	}
	if len(parts) == 2 && len(parts[1]) != 1 {
		return 0, bad("millimetres require one decimal place")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, bad("invalid millimetres")
	}
	value := whole * 10
	if len(parts) == 2 {
		frac, _ := strconv.ParseInt(parts[1], 10, 64)
		value += frac
	}
	value *= sign
	if value < min || value > max {
		return 0, bad("millimetres out of range")
	}
	return value, nil
}
func validateDocument(d *quoteDocument, final bool) error {
	if len(d.Title) > 512 || len(d.Subtitle) > 512 || len(d.ProjectRef) > 512 || !currencyRe.MatchString(d.Currency) || len(d.Sections) > 20 || len(d.Positions) > 100 {
		return bad("invalid document bounds")
	}
	day, e := time.Parse("2006-01-02", d.OfferDate)
	if e != nil {
		return bad("invalid offer date")
	}
	until, e := time.Parse("2006-01-02", d.ValidUntil)
	if e != nil || until.Before(day) {
		return bad("invalid validity date")
	}
	if final && (strings.TrimSpace(d.Title) == "" || len(d.Positions) == 0) {
		return bad("document needs a title and position")
	}
	ids := map[string]bool{}
	for _, s := range d.Sections {
		if !uuidRe.MatchString(s.ID) || ids[s.ID] || len(s.Heading) > 500 || len(s.Body) > 100000 || len(s.Nodes) > 100 {
			return bad("invalid section")
		}
		if s.NumberingStyle != "" && s.NumberingStyle != "decimal" && s.NumberingStyle != "upper-roman" && s.NumberingStyle != "lower-roman" && s.NumberingStyle != "upper-alpha" && s.NumberingStyle != "lower-alpha" && s.NumberingStyle != "none" {
			return bad("invalid section numbering style")
		}
		if _, err := parseMM(s.SpacingBeforeMM, 0, 400); err != nil {
			return err
		}
		if _, err := parseMM(s.SpacingAfterMM, 0, 400); err != nil {
			return err
		}
		ids[s.ID] = true
		for _, n := range s.Nodes {
			if !uuidRe.MatchString(n.ID) || ids[n.ID] || utf8.RuneCountInString(n.Text) > 2000 || n.Depth < 0 || n.Depth > 5 || utf8.RuneCountInString(n.Glyph) > 4 {
				return bad("invalid prose node")
			}
			ids[n.ID] = true
			if n.Kind != "paragraph" && n.Kind != "item" {
				return bad("invalid prose kind")
			}
			if n.Kind == "paragraph" && (n.Marker != "" || n.Numbering != "" || n.ListStart != 0 || n.ListContinue || n.SectionBound) {
				return bad("invalid paragraph markers")
			}
			if (n.Kind != "item" || n.Marker != "decimal") && (n.Numbering != "" || n.ListStart != 0 || n.ListContinue || n.SectionBound) {
				return bad("numbering requires a decimal item")
			}
			if n.Kind == "paragraph" && (n.Depth != 0 || n.Glyph != "" || n.MarkerXMM != "" || n.MarkerYMM != "" || n.TextStartMM != "") {
				return bad("invalid paragraph layout")
			}
			if n.Glyph != "" && (n.Marker == "decimal" || strings.TrimSpace(n.Glyph) != n.Glyph || strings.ContainsAny(n.Glyph, "<>&")) {
				return bad("invalid bullet glyph")
			}
			if _, err := parseMM(n.MarkerXMM, -300, 300); err != nil {
				return err
			}
			if _, err := parseMM(n.MarkerYMM, -200, 200); err != nil {
				return err
			}
			if _, err := parseMM(n.TextStartMM, -200, 400); err != nil {
				return err
			}
			if n.Marker != "" && n.Marker != "disc" && n.Marker != "circle" && n.Marker != "square" && n.Marker != "dash" && n.Marker != "decimal" {
				return bad("invalid list marker")
			}
			if n.Numbering != "" && n.Numbering != "outline" {
				return bad("invalid numbering")
			}
			if n.ListStart < 0 || n.ListStart > 9999 || n.ListStart > 0 && n.ListContinue || n.SectionBound && n.Numbering != "outline" {
				return bad("invalid numbering start")
			}
			units := utf16.Encode([]rune(n.Text))
			end := 0
			for _, m := range n.Marks {
				if m.Start < end || m.Start < 0 || m.End <= m.Start || m.End > len(units) || (!m.Bold && !m.Italic) || !utf16Boundary(units, m.Start) || !utf16Boundary(units, m.End) {
					return bad("invalid inline marks")
				}
				end = m.End
			}
		}
	}
	var total int64
	for i := range d.Positions {
		p := &d.Positions[i]
		if !uuidRe.MatchString(p.ID) || ids[p.ID] || len(p.ShortText) > 2000 || len(p.LongText) > 10000 || len(p.UnitLabel) > 80 || strings.TrimSpace(p.UnitLabel) == "" || p.Currency != d.Currency || p.UnitPriceCents < 0 || p.UnitPriceCents > 1_000_000_000 {
			return bad("invalid position")
		}
		ids[p.ID] = true
		if p.PricingSource != "manual" && p.PricingSource != "cost_unit" {
			return bad("invalid pricing source")
		}
		if p.PricingSource == "manual" && p.CostUnitNodeID != "" || p.PricingSource == "cost_unit" && !uuidRe.MatchString(p.CostUnitNodeID) {
			return bad("invalid cost unit")
		}
		if (p.PricingSource == "manual" && p.RateUnit != "") || (p.PricingSource == "cost_unit" && p.RateUnit != "hour" && p.RateUnit != "day" && p.RateUnit != "item") {
			return bad("invalid rate unit")
		}
		qty, e := parseQuantity(p.Quantity)
		if e != nil {
			return bad("invalid quantity")
		}
		if final && (strings.TrimSpace(p.ShortText) == "" || qty == 0) {
			return bad("incomplete position")
		}
		amount := new(big.Int).Mul(big.NewInt(qty), big.NewInt(p.UnitPriceCents))
		amount.Add(amount, big.NewInt(50))
		amount.Div(amount, big.NewInt(100))
		if !amount.IsInt64() || amount.Int64() > 1_000_000_000_000-total {
			return bad("quote total too large")
		}
		p.TotalCents = amount.Int64()
		total += p.TotalCents
	}
	d.NetTotalCents = total
	if final {
		var sender struct {
			Company    string `json:"company"`
			Street     string `json:"street"`
			PostalCode string `json:"postal_code"`
			City       string `json:"city"`
			Country    string `json:"country"`
			Email      string `json:"email"`
		}
		var recipient struct {
			Name    string `json:"name"`
			Address string `json:"address"`
			Email   string `json:"email"`
		}
		if json.Unmarshal(d.Sender, &sender) != nil || json.Unmarshal(d.Recipient, &recipient) != nil || strings.TrimSpace(sender.Company) == "" || strings.TrimSpace(sender.Street) == "" || strings.TrimSpace(sender.PostalCode) == "" || strings.TrimSpace(sender.City) == "" || strings.TrimSpace(sender.Country) == "" || strings.TrimSpace(recipient.Name) == "" || strings.TrimSpace(recipient.Address) == "" || !validEmail(sender.Email) || !validEmail(recipient.Email) {
			return bad("sender or recipient is incomplete")
		}
	}
	return nil
}
func validEmail(s string) bool { a, e := mail.ParseAddress(s); return e == nil && a.Address == s }
func utf16Boundary(units []uint16, at int) bool {
	return at <= 0 || at >= len(units) || !(units[at-1] >= 0xD800 && units[at-1] <= 0xDBFF && units[at] >= 0xDC00 && units[at] <= 0xDFFF)
}
func parseQuantity(s string) (int64, error) {
	parts := strings.Split(s, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || len(parts[0]) > 7 {
		return 0, bad("invalid quantity")
	}
	for _, part := range parts {
		if part == "" {
			return 0, bad("invalid quantity")
		}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return 0, bad("invalid quantity")
			}
		}
	}
	if len(parts[0]) > 1 && parts[0][0] == '0' {
		return 0, bad("invalid quantity")
	}
	whole, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || whole > 1_000_000 {
		return 0, bad("invalid quantity")
	}
	var frac int64
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, bad("invalid quantity")
		}
		frac, e = strconv.ParseInt(parts[1], 10, 64)
		if e != nil {
			return 0, bad("invalid quantity")
		}
		if len(parts[1]) == 1 {
			frac *= 10
		}
	}
	return whole*100 + frac, nil
}
func makeDocument(ctx context.Context, tx pgx.Tx, settings quoteSettings, org, title, customerNo string, day time.Time) (quoteDocument, error) {
	var d quoteDocument
	recipient, err := customerRecipient(ctx, tx, org, customerNo)
	if err != nil {
		return d, err
	}
	var defaults struct {
		Intro  string `json:"intro"`
		Blocks []struct {
			Heading string     `json:"heading"`
			Body    string     `json:"body"`
			Nodes   []textNode `json:"nodes"`
		} `json:"blocks"`
		AcceptText string `json:"accept_text"`
		VATNote    string `json:"vat_note"`
	}
	if err := json.Unmarshal(settings.Defaults, &defaults); err != nil {
		return d, err
	}
	recipientRaw, _ := json.Marshal(recipient)
	legal, _ := json.Marshal(map[string]string{"intro": defaults.Intro, "accept_text": defaults.AcceptText, "vat_note": defaults.VATNote})
	positionID, err := newID()
	if err != nil {
		return d, err
	}
	d = quoteDocument{SchemaVersion: 1, MinimumWriterVersion: 1, Title: title, OfferDate: day.Format("2006-01-02"), ValidUntil: day.AddDate(0, 0, 30).Format("2006-01-02"), Currency: settings.DefaultCurrency, Sender: settings.Sender, Recipient: recipientRaw, Legal: legal, Layout: settings.Layout, Sections: []documentSection{}, Positions: []documentPosition{{ID: positionID, PricingSource: "manual", Quantity: "1", UnitLabel: "item", Currency: settings.DefaultCurrency}}}
	for _, b := range defaults.Blocks {
		id, e := newID()
		if e != nil {
			return d, e
		}
		section := documentSection{ID: id, Heading: b.Heading, Body: b.Body, Nodes: b.Nodes}
		if section.Nodes == nil {
			section.Nodes = []textNode{}
		}
		for i := range section.Nodes {
			nodeID, e := newID()
			if e != nil {
				return d, e
			}
			section.Nodes[i].ID = nodeID
		}
		d.Sections = append(d.Sections, section)
	}
	d.MinimumWriterVersion = documentMinimumWriterVersion(d)
	return d, nil
}

func customerRecipient(ctx context.Context, tx pgx.Tx, org, customerNo string) (map[string]string, error) {
	var name string
	var orgFields, contactFields []byte
	var contactName, contactID string
	err := tx.QueryRow(ctx, `SELECT o.title,o.fields,coalesce(c.title,''),coalesce(c.fields,'{}'::jsonb),coalesce(c.id::text,'')
		FROM nodes o LEFT JOIN crm_organisation_profiles p ON p.tenant_id=o.tenant_id AND p.organisation_node_id=o.id
		LEFT JOIN nodes c ON c.tenant_id=p.tenant_id AND c.id=p.primary_contact_node_id AND c.deleted_at IS NULL
		WHERE o.id=$1::uuid AND o.deleted_at IS NULL`, org).Scan(&name, &orgFields, &contactName, &contactFields, &contactID)
	if err != nil {
		return nil, err
	}
	var fields struct {
		BillingAddress  quoteAddress `json:"billing_address"`
		VisitingAddress quoteAddress `json:"visiting_address"`
	}
	if err := json.Unmarshal(orgFields, &fields); err != nil {
		return nil, err
	}
	var contact struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(contactFields, &contact); err != nil {
		return nil, err
	}
	address := fields.VisitingAddress
	if fields.BillingAddress.hasValue() {
		address = fields.BillingAddress
	}
	return map[string]string{"name": name, "address": address.display(), "contact": contactName, "country": address.Country, "customer_no": customerNo, "email": contact.Email, "contact_node_id": contactID}, nil
}

type quoteAddress struct {
	Street     string `json:"street"`
	PostalCode string `json:"postal_code"`
	City       string `json:"city"`
	Country    string `json:"country"`
	Freeform   string `json:"freeform"`
}

func (a quoteAddress) hasValue() bool {
	return strings.TrimSpace(a.Street+a.PostalCode+a.City+a.Freeform) != ""
}

func (a quoteAddress) display() string {
	if strings.TrimSpace(a.Freeform) != "" {
		return strings.TrimSpace(a.Freeform)
	}
	return strings.TrimSpace(strings.Join([]string{strings.TrimSpace(a.Street), strings.TrimSpace(a.PostalCode + " " + a.City)}, "\n"))
}
