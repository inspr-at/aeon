// SPDX-License-Identifier: AGPL-3.0-only

package hours

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

type PeriodWrite struct {
	PrincipalID string    `json:"principal_id"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
}
type Period struct {
	PeriodWrite
	ID       string    `json:"id"`
	State    string    `json:"state"`
	Revision int64     `json:"revision"`
	Approval *Approval `json:"approval"`
	digest   string
}
type Approval struct {
	ApprovedBy    string `json:"approved_by_principal_id"`
	EntriesSHA256 string `json:"entries_sha256"`
	TotalSeconds  int64  `json:"total_seconds"`
	EventID       int64  `json:"event_id"`
}
type EntryWrite struct {
	PeriodID    string    `json:"period_id"`
	CostUnitID  string    `json:"cost_unit_node_id"`
	Currency    string    `json:"currency"`
	Source      string    `json:"source"`
	PrincipalID string    `json:"principal_id,omitempty"`
	NodeID      string    `json:"node_id,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	EndedAt     time.Time `json:"ended_at"`
	AgentRunID  *string   `json:"agent_run_id,omitempty"`
	Note        string    `json:"note"`
}
type Entry struct {
	EntryWrite
	ID              string      `json:"id"`
	DurationSeconds int64       `json:"duration_seconds"`
	RateAmount      json.Number `json:"rate_amount"`
	Amount          json.Number `json:"amount"`
}
type Amount struct {
	Currency string      `json:"currency"`
	Amount   json.Number `json:"amount"`
}
type Totals struct {
	NodeID          string   `json:"node_id"`
	DurationSeconds int64    `json:"duration_seconds"`
	Amounts         []Amount `json:"amounts"`
}

func entryDigest(entries []Entry) (string, int64, error) {
	raw, err := json.Marshal(entries)
	if err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(raw)
	var seconds int64
	for _, e := range entries {
		seconds += e.DurationSeconds
	}
	return hex.EncodeToString(sum[:]), seconds, nil
}
func utc(t time.Time) time.Time { return t.UTC() }
