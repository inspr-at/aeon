// SPDX-License-Identifier: AGPL-3.0-only
package offers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var offerNumber = regexp.MustCompile(`^A([0-9]{6})-([0-9]{2,})$`)

type legacyDocument struct {
	Title      string                 `json:"title"`
	Subtitle   string                 `json:"subtitle"`
	ProjectRef string                 `json:"project_ref"`
	OfferDate  string                 `json:"offer_date"`
	ValidUntil string                 `json:"valid_until"`
	Sender     json.RawMessage        `json:"sender"`
	Customer   json.RawMessage        `json:"customer"`
	Intro      string                 `json:"intro"`
	AcceptText string                 `json:"accept_text"`
	VATNote    string                 `json:"vat_note"`
	Footer     map[string]json.Number `json:"footer"`
	Blocks     []struct {
		Heading string           `json:"heading"`
		Body    string           `json:"body"`
		Nodes   []map[string]any `json:"nodes"`
	} `json:"blocks"`
	Positions []struct {
		ShortText      string      `json:"short_text"`
		LongText       string      `json:"long_text"`
		Quantity       json.Number `json:"quantity"`
		Unit           string      `json:"unit"`
		UnitPriceCents int64       `json:"unit_price_cents"`
		TotalCents     int64       `json:"total_cents"`
	} `json:"positions"`
	NetTotalCents int64 `json:"net_total_cents"`
}

type Document struct {
	SchemaVersion        int               `json:"schema_version"`
	MinimumWriterVersion int               `json:"minimum_writer_version"`
	Title                string            `json:"title"`
	Subtitle             string            `json:"subtitle"`
	ProjectRef           string            `json:"project_ref"`
	OfferDate            string            `json:"offer_date"`
	ValidUntil           string            `json:"valid_until"`
	Currency             string            `json:"currency"`
	Sender               json.RawMessage   `json:"sender"`
	Recipient            json.RawMessage   `json:"recipient"`
	Legal                map[string]string `json:"legal"`
	Layout               map[string]string `json:"layout"`
	Sections             []Section         `json:"sections"`
	Positions            []Position        `json:"positions"`
	NetTotalCents        int64             `json:"net_total_cents"`
	Profile              *ProfileSnapshot  `json:"profile,omitempty"`
}

// ProfileSnapshot freezes the tenant's selected document profile into each
// imported source revision, including issued versions.
type ProfileSnapshot struct {
	ID         string          `json:"id"`
	Revision   int             `json:"revision"`
	Definition json.RawMessage `json:"definition"`
}
type Section struct {
	ID      string           `json:"id"`
	Heading string           `json:"heading"`
	Body    string           `json:"body"`
	Nodes   []map[string]any `json:"nodes"`
}
type Position struct {
	ID             string `json:"id"`
	PricingSource  string `json:"pricing_source"`
	ShortText      string `json:"short_text"`
	LongText       string `json:"long_text"`
	Quantity       string `json:"quantity"`
	UnitLabel      string `json:"unit_label"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	TotalCents     int64  `json:"total_cents"`
	Currency       string `json:"currency"`
}

func convertDocument(instance string, o Offer, contactID string) (Document, error) {
	var src legacyDocument
	if err := decode(o.Document, &src); err != nil {
		return Document{}, fmt.Errorf("offer %d document: %w", o.ID, err)
	}
	if src.Title == "" || len(src.Title) > 512 || len(src.Blocks) > 20 || len(src.Positions) > 100 {
		return Document{}, errors.New("invalid offer document bounds")
	}
	day, err := time.Parse("2006-01-02", src.OfferDate)
	if err != nil {
		return Document{}, errors.New("invalid offer date")
	}
	valid, err := time.Parse("2006-01-02", src.ValidUntil)
	if err != nil || valid.Before(day) {
		return Document{}, errors.New("invalid validity date")
	}
	if !json.Valid(src.Sender) || !json.Valid(src.Customer) {
		return Document{}, errors.New("missing document party snapshot")
	}
	var recipient map[string]any
	if err := json.Unmarshal(src.Customer, &recipient); err != nil || recipient == nil {
		return Document{}, errors.New("invalid recipient")
	}
	if contactID != "" {
		recipient["contact_node_id"] = contactID
	}
	recipientJSON, _ := json.Marshal(recipient)
	d := Document{SchemaVersion: 1, MinimumWriterVersion: 1, Title: src.Title, Subtitle: src.Subtitle, ProjectRef: src.ProjectRef, OfferDate: src.OfferDate, ValidUntil: src.ValidUntil, Currency: "EUR", Sender: src.Sender, Recipient: recipientJSON, Legal: map[string]string{"intro": src.Intro, "accept_text": src.AcceptText, "vat_note": src.VATNote}, Layout: map[string]string{}, Sections: []Section{}, Positions: []Position{}}
	for _, key := range []string{"logo_width_mm", "logo_offset_mm"} {
		if v, ok := src.Footer[key]; ok {
			d.Layout[key] = v.String()
		}
	}
	for i, s := range src.Blocks {
		if len(s.Nodes) > 100 {
			return Document{}, errors.New("too many prose nodes")
		}
		section := Section{ID: stableID(instance, "offer", o.ID, "section", i), Heading: s.Heading, Body: s.Body, Nodes: []map[string]any{}}
		for j, n := range s.Nodes {
			copyNode := map[string]any{}
			for k, v := range n {
				copyNode[k] = v
			}
			copyNode["id"] = stableID(instance, "offer", o.ID, fmt.Sprintf("section-%d-node", i), j)
			section.Nodes = append(section.Nodes, copyNode)
		}
		d.Sections = append(d.Sections, section)
	}
	for i, p := range src.Positions {
		qty := p.Quantity.String()
		if qty == "" {
			return Document{}, errors.New("position quantity missing")
		}
		computed, err := lineTotal(p.UnitPriceCents, qty)
		if err != nil || computed != p.TotalCents {
			return Document{}, fmt.Errorf("offer %d position %d total mismatch", o.ID, i)
		}
		if strings.TrimSpace(p.ShortText) == "" || p.Unit == "" {
			return Document{}, errors.New("invalid position")
		}
		d.Positions = append(d.Positions, Position{ID: stableID(instance, "offer", o.ID, "position", i), PricingSource: "manual", ShortText: p.ShortText, LongText: p.LongText, Quantity: qty, UnitLabel: p.Unit, UnitPriceCents: p.UnitPriceCents, TotalCents: p.TotalCents, Currency: "EUR"})
		if d.NetTotalCents > 1_000_000_000_000-p.TotalCents {
			return Document{}, errors.New("offer total too large")
		}
		d.NetTotalCents += p.TotalCents
	}
	if d.NetTotalCents != src.NetTotalCents {
		return Document{}, errors.New("offer net total mismatch")
	}
	if o.Status != "draft" && len(d.Positions) == 0 {
		return Document{}, errors.New("issued offer needs a position")
	}
	return d, nil
}
func lineTotal(price int64, quantity string) (int64, error) {
	if price < 0 {
		return 0, errors.New("negative price")
	}
	parts := strings.Split(quantity, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || len(parts[0]) > 15 {
		return 0, errors.New("invalid quantity")
	}
	for _, p := range parts {
		if p == "" {
			return 0, errors.New("invalid quantity")
		}
		for _, ch := range p {
			if ch < '0' || ch > '9' {
				return 0, errors.New("invalid quantity")
			}
		}
	}
	if len(parts) == 2 && len(parts[1]) > 2 {
		return 0, errors.New("quantity has too many decimals")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	qty := whole * 100
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) == 1 {
			frac += "0"
		}
		n, _ := strconv.ParseInt(frac, 10, 64)
		qty += n
	}
	if qty <= 0 {
		return 0, errors.New("quantity must be positive")
	}
	n := new(big.Int).Mul(big.NewInt(price), big.NewInt(qty))
	n.Add(n, big.NewInt(50))
	n.Div(n, big.NewInt(100))
	if !n.IsInt64() || n.Int64() > 1_000_000_000_000 {
		return 0, errors.New("line total too large")
	}
	return n.Int64(), nil
}
func majorMoney(value json.Number) (int64, error) {
	s := value.String()
	parts := strings.Split(s, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || len(parts[0]) > 12 {
		return 0, errors.New("invalid source money")
	}
	for _, part := range parts {
		if part == "" {
			return 0, errors.New("invalid source money")
		}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return 0, errors.New("invalid source money")
			}
		}
	}
	if len(parts) == 2 && len(parts[1]) > 2 {
		return 0, errors.New("source money has sub-cent precision")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	minor := whole * 100
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) == 1 {
			frac += "0"
		}
		n, _ := strconv.ParseInt(frac, 10, 64)
		minor += n
	}
	return minor, nil
}
func stableID(instance, kind string, id int64, part string, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("paimos:%s:%s:%d:%s:%d", instance, kind, id, part, index)))
	sum[6] = (sum[6] & 15) | 80
	sum[8] = (sum[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}
func sha(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
