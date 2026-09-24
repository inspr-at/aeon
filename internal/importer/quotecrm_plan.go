// SPDX-License-Identifier: AGPL-3.0-only

package importer

import (
	"errors"
	"fmt"
	"strings"
)

// QuoteCRMSnapshot is a synthetic, read-only mapping contract. It is not an
// importer.Source snapshot and cannot trigger an import. A future source API
// must supply original evidence; a target renderer cannot recreate it.
type QuoteCRMSnapshot struct {
	SourceInstance string             `json:"source_instance"`
	Customers      []QuoteCRMCustomer `json:"customers"`
	Contacts       []QuoteCRMContact  `json:"contacts"`
	Quotes         []QuoteCRMQuote    `json:"quotes"`
}

type QuoteCRMCustomer struct {
	SourceID  string `json:"source_id"`
	Number    string `json:"customer_no"`
	Name      string `json:"name"`
	PrimaryID string `json:"primary_contact_source_id"`
}

type QuoteCRMContact struct {
	SourceID   string `json:"source_id"`
	CustomerID string `json:"customer_source_id"`
	Name       string `json:"name"`
}

type QuoteCRMQuote struct {
	SourceID        string `json:"source_id"`
	CustomerID      string `json:"customer_source_id"`
	Number          string `json:"offer_no"`
	State           string `json:"state"`
	Currency        string `json:"currency"`
	NetTotalMinor   int64  `json:"net_total_minor"`
	OriginalSHA256  string `json:"original_sha256"`
	OriginalPDFHash string `json:"original_pdf_sha256"`
}

type QuoteCRMMapping struct {
	SourceKey string `json:"source_key"`
	Target    string `json:"target"`
	Review    string `json:"review,omitempty"`
}

// PlanQuoteCRMMapping gives a stable namespace and rejects ambiguous links or
// numbering before any target write. Original PDF/digest absence is reported
// for human reconciliation. It never invents acceptance evidence or tokens.
func PlanQuoteCRMMapping(s QuoteCRMSnapshot) ([]QuoteCRMMapping, error) {
	if strings.TrimSpace(s.SourceInstance) == "" || strings.Contains(s.SourceInstance, ":") {
		return nil, errors.New("explicit source instance required")
	}
	key := func(kind, id string) string { return "pma:" + s.SourceInstance + ":" + kind + ":" + id }
	validDigest := func(value string) bool {
		if len(value) != 64 {
			return false
		}
		for _, c := range value {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return false
			}
		}
		return true
	}
	appendReview := func(mapping *QuoteCRMMapping, why string) {
		if mapping.Review != "" {
			mapping.Review += "; "
		}
		mapping.Review += why
	}
	seen := map[string]bool{}
	numbers := map[string]bool{}
	customers := map[string]QuoteCRMCustomer{}
	contacts := map[string]QuoteCRMContact{}
	result := make([]QuoteCRMMapping, 0, len(s.Customers)+len(s.Contacts)+len(s.Quotes))
	add := func(kind, id, target, number string) error {
		if strings.TrimSpace(id) == "" || strings.Contains(id, ":") || seen[kind+":"+id] {
			return fmt.Errorf("missing or duplicate %s source ID", kind)
		}
		seen[kind+":"+id] = true
		if number != "" {
			if numbers[kind+":"+number] {
				return fmt.Errorf("duplicate %s number", kind)
			}
			numbers[kind+":"+number] = true
		}
		result = append(result, QuoteCRMMapping{SourceKey: key(kind, id), Target: target})
		return nil
	}
	for _, c := range s.Customers {
		if strings.TrimSpace(c.Name) == "" {
			return nil, errors.New("customer name required")
		}
		if err := add("customer", c.SourceID, "organisation", c.Number); err != nil {
			return nil, err
		}
		customers[c.SourceID] = c
	}
	for _, c := range s.Contacts {
		if _, ok := customers[c.CustomerID]; !ok {
			return nil, errors.New("contact customer missing")
		}
		if err := add("contact", c.SourceID, "contact_for", ""); err != nil {
			return nil, err
		}
		contacts[c.SourceID] = c
	}
	for _, c := range s.Customers {
		if c.PrimaryID != "" && contacts[c.PrimaryID].CustomerID != c.SourceID {
			return nil, errors.New("primary contact is not linked to customer")
		}
	}
	for _, q := range s.Quotes {
		if _, ok := customers[q.CustomerID]; !ok {
			return nil, errors.New("quote customer missing")
		}
		if q.NetTotalMinor < 0 || len(q.Currency) != 3 || q.Currency != strings.ToUpper(q.Currency) {
			return nil, errors.New("invalid quote money")
		}
		if err := add("quote", q.SourceID, "business_quote", q.Number); err != nil {
			return nil, err
		}
		last := &result[len(result)-1]
		switch q.State {
		case "draft":
		case "issued", "accepted", "declined", "expired":
			if q.OriginalSHA256 != "" && !validDigest(q.OriginalSHA256) {
				return nil, errors.New("invalid original document digest")
			}
			if q.OriginalSHA256 == "" {
				appendReview(last, "original immutable document digest unavailable")
			}
			if q.OriginalPDFHash != "" && !validDigest(q.OriginalPDFHash) {
				return nil, errors.New("invalid original PDF digest")
			}
			if q.State == "accepted" && q.OriginalPDFHash == "" {
				appendReview(last, "original accepted PDF unavailable")
			}
			if q.State == "declined" || q.State == "expired" {
				appendReview(last, "historical outcome requires original evidence; no native decision is synthesized")
			}
		default:
			return nil, errors.New("unsupported source quote state")
		}
	}
	return result, nil
}
