// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/inspr-at/aeon/internal/tenant"
)

func (s *seeder) hours() error {
	var catalog []struct {
		ID          string   `json:"id"`
		Digest      string   `json:"digest_sha256"`
		Permissions []string `json:"permissions"`
	}
	if err := s.api.do(s.admin, "", http.MethodGet, "/api/plugins", nil, http.StatusOK, &catalog, nil); err != nil {
		return fmt.Errorf("plugins: %w", err)
	}
	for _, item := range catalog {
		if item.ID != "business_costs" && item.ID != "business_hours" {
			continue
		}
		if err := s.api.do(s.admin, "", http.MethodPut, "/api/plugins/"+item.ID+"/installation", map[string]any{
			"manifest_digest_sha256": item.Digest, "enabled": true, "permissions": item.Permissions,
		}, http.StatusOK, nil, nil); err != nil {
			return fmt.Errorf("enable %s: %w", item.ID, err)
		}
	}
	kindID := s.kinds["cost_unit"]
	if kindID == "" {
		var created idBody
		if err := s.api.do(s.admin, "", http.MethodPost, "/api/kinds", map[string]any{
			"slug": "cost_unit", "label": "Cost unit", "short_prefix": "CU", "icon": "cost",
			"allowed_child_kinds": []string{}, "field_schema": map[string]any{},
		}, http.StatusCreated, &created, nil); err != nil {
			return fmt.Errorf("cost unit kind: %w", err)
		}
		kindID = created.ID
		s.kinds["cost_unit"] = kindID
	}
	unitID, err := s.node(kindID, "CU-1", "Fictional archive hour", "Screenshot cost unit. Not a payroll rate.", "open", s.lumenID, nil)
	if err != nil {
		return err
	}
	if err := s.api.do(s.admin, "", http.MethodPost, "/api/cost-units/"+unitID+"/rates", rateBody{
		Unit: "hour", Currency: "EUR", Internal: json.Number("80.00"), Bill: json.Number("140.00"), From: "2026-01-01",
	}, http.StatusCreated, nil, nil); err != nil {
		return fmt.Errorf("rate: %w", err)
	}
	adminPeriod, err := s.period(s.admin)
	if err != nil {
		return err
	}
	ivoPeriod, err := s.period(s.ivo)
	if err != nil {
		return err
	}
	if err := s.entry(adminPeriod, s.admin, unitID, s.ids["LT-4"],
		time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 10, 11, 0, 0, 0, time.UTC),
		"Fictional hours oiling the oak desks."); err != nil {
		return err
	}
	if err := s.entry(adminPeriod, s.admin, unitID, s.ids["LT-9"],
		time.Date(2026, 9, 11, 13, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 11, 14, 30, 0, 0, time.UTC),
		"Fictional hours posting the quiet-hour card."); err != nil {
		return err
	}
	return s.entry(ivoPeriod, s.ivo, unitID, s.ids["HT-4"],
		time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC),
		"Fictional hours coiling a spare line.")
}

func (s *seeder) period(who tenant.Principal) (string, error) {
	var period idBody
	if err := s.api.do(s.admin, "", http.MethodPost, "/api/time-periods", map[string]any{
		"principal_id": who.ID,
		"starts_at":    time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		"ends_at":      time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}, http.StatusCreated, &period, nil); err != nil {
		return "", fmt.Errorf("period for %s: %w", who.Name, err)
	}
	return period.ID, nil
}

type rateBody struct {
	Unit     string      `json:"unit"`
	Currency string      `json:"currency"`
	Internal json.Number `json:"internal_amount"`
	Bill     json.Number `json:"bill_amount"`
	From     string      `json:"effective_from"`
}

func (s *seeder) entry(periodID string, who tenant.Principal, unitID, nodeID string, start, end time.Time, note string) error {
	if err := s.api.do(s.admin, "", http.MethodPost, "/api/time-entries", map[string]any{
		"period_id": periodID, "cost_unit_node_id": unitID, "currency": "EUR",
		"source": "manual", "principal_id": who.ID, "node_id": nodeID,
		"started_at": start, "ended_at": end, "note": note,
	}, http.StatusCreated, nil, nil); err != nil {
		return fmt.Errorf("time entry for %s: %w", who.Name, err)
	}
	return nil
}
