// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"fmt"
	"net/http"
	"time"

	"github.com/inspr-at/aeon/internal/agentruns"
	"github.com/inspr-at/aeon/internal/auth"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

const (
	daemonID   = "demo-lumen-daemon"
	generation = "demo-gen-1"
	sessionRef = "demo-lumen-scribe-session-v1"
	lease      = "demo-lumen-scribe-lease-000000000001"
)

func (s *seeder) agents() error {
	var err error
	s.scribe, s.scribeKey, err = s.agent("Lumen Scribe", []string{
		"intake.write", "approvals.request", "approvals.propose",
		"account.manage", "run.claim", "run.telemetry", "run.create",
		"harness.write", "work_orders.write", "work_orders.read",
		"nodes.read", "nodes.write",
	})
	if err != nil {
		return err
	}
	s.clerk, s.clerkKey, err = s.agent("Harbor Clerk", []string{
		"approvals.request", "nodes.read", "nodes.write",
	})
	return err
}

func (s *seeder) agent(name string, scopes []string) (tenant.Principal, string, error) {
	_, agentID, token, err := auth.OperatorCreateAgentKey(s.ctx, s.pool, s.tenantID, name, "", scopes, nil)
	if err != nil {
		return tenant.Principal{}, "", fmt.Errorf("agent %s: %w", name, err)
	}
	if name == "Lumen Scribe" {
		if err := s.allowJourneyGates(agentID); err != nil {
			return tenant.Principal{}, "", err
		}
	}
	return tenant.Principal{ID: agentID, TenantID: s.tenantID, Kind: tenant.Agent, Name: name}, token, nil
}

// allowJourneyGates adds the journey prefixes the journey module checks.
// aeon agent-key create rejects them because they are not permission-registry
// keys. Proposals still go through the approvals API, which reads this list.
func (s *seeder) allowJourneyGates(agentID string) error {
	added := []string{"journey.requirements", "journey.build"}
	return db.InTenant(tenant.WithPrincipal(s.ctx, s.admin), s.pool, s.tenantID, func(tx pgx.Tx) error {
		var keyID string
		tag, err := tx.Exec(s.ctx, `UPDATE agent_keys SET scopes = scopes || $2::text[]
			WHERE principal_id=$1::uuid AND NOT (scopes @> $2::text[])`, agentID, added)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		if err := tx.QueryRow(s.ctx, `SELECT id::text FROM agent_keys WHERE principal_id=$1::uuid ORDER BY created_at DESC LIMIT 1`, agentID).Scan(&keyID); err != nil {
			return err
		}
		_, err = events.Append(s.ctx, tx, s.admin, events.Change{
			Type:  "demo.key_scopes",
			After: map[string]any{"key_id": keyID, "principal_id": agentID, "added": added},
		})
		return err
	})
}

func (s *seeder) work() error {
	var order struct {
		NodeID   string `json:"node_id"`
		Revision int64  `json:"revision"`
	}
	if err := s.api.do(s.admin, "", http.MethodPost, "/api/work-orders", map[string]any{
		"title": "Fictional lantern pass", "body": "Screenshot work order. No harness is running.",
		"parent_id": s.lumenID, "assignee_principal_id": s.scribe.ID,
		"criteria": []string{"The prop lantern is labeled"},
	}, http.StatusCreated, &order, nil); err != nil {
		return fmt.Errorf("work order: %w", err)
	}
	if err := s.api.do(s.admin, "", http.MethodPatch, "/api/work-orders/"+order.NodeID, map[string]any{
		"expected_revision": order.Revision, "status": "ready",
	}, http.StatusOK, &order, nil); err != nil {
		return fmt.Errorf("ready work order: %w", err)
	}
	var profiles []struct {
		ID      string `json:"id"`
		Harness string `json:"harness"`
		Enabled bool   `json:"enabled"`
	}
	if err := s.api.do(s.admin, "", http.MethodGet, "/api/models", nil, http.StatusOK, &profiles, nil); err != nil {
		return fmt.Errorf("models: %w", err)
	}
	var profileID string
	for _, profile := range profiles {
		if profile.Enabled && profile.Harness == "codex" {
			profileID = profile.ID
			break
		}
	}
	if profileID == "" {
		return fmt.Errorf("no enabled codex model profile")
	}
	var run idBody
	if err := s.api.do(s.admin, "", http.MethodPost, "/api/work-orders/"+order.NodeID+"/runs", map[string]any{
		"agent_principal_id": s.scribe.ID, "model_profile_id": profileID,
	}, http.StatusCreated, &run, nil); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	var account idBody
	if err := s.api.do(s.scribe, s.scribeKey, http.MethodPost, "/api/agent-accounts", map[string]any{
		"account_key": "lumen-scribe-local", "harness": "codex", "daemon_id": daemonID,
		"label": "Fictional Lumen desk", "max_parallel_runs": 1,
	}, http.StatusCreated, &account, nil); err != nil {
		return fmt.Errorf("agent account: %w", err)
	}
	if err := s.api.do(s.scribe, s.scribeKey, http.MethodPost, "/api/agent-accounts/"+account.ID+"/probe", map[string]any{
		"daemon_id": daemonID, "daemon_generation": generation, "available": true,
	}, http.StatusOK, nil, nil); err != nil {
		return fmt.Errorf("probe: %w", err)
	}
	start := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	end := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	window := []byte(fmt.Sprintf(`{"starts_at":%q,"ends_at":%q,"unit":"requests","allowance":100000,"pace_model":"unrestricted","burst_ratio":0}`, start, end))
	if err := s.api.do(s.admin, "", http.MethodPost, "/api/agent-accounts/"+account.ID+"/windows", jsonRaw(window), http.StatusCreated, nil, nil); err != nil {
		return fmt.Errorf("allowance window: %w", err)
	}
	var routed struct {
		Reservations []struct {
			ID string `json:"reservation_id"`
		} `json:"reservations"`
	}
	if err := s.api.do(s.scribe, s.scribeKey, http.MethodPost, "/api/agent-accounts/route", map[string]any{
		"run_id": run.ID, "estimated_units": map[string]int64{"requests": 1},
	}, http.StatusOK, &routed, nil); err != nil {
		return fmt.Errorf("route: %w", err)
	}
	ids := make([]string, 0, len(routed.Reservations))
	for _, item := range routed.Reservations {
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 {
		return fmt.Errorf("route reserved nothing")
	}
	if err := s.api.do(s.scribe, s.scribeKey, http.MethodPost, "/api/runs/"+run.ID+"/claim", map[string]any{
		"daemon_id": daemonID, "daemon_generation": generation, "reservation_ids": ids,
	}, http.StatusOK, nil, nil); err != nil {
		return fmt.Errorf("claim: %w", err)
	}
	headers := map[string]string{agentruns.DaemonHeader: daemonID, agentruns.GenerationHeader: generation}
	if err := s.api.do(s.scribe, s.scribeKey, http.MethodPost, "/api/runs/"+run.ID+"/telemetry", map[string]any{
		"sequence": 1, "kind": "finished", "status": "completed",
		"input_tokens_delta": 0, "output_tokens_delta": 0, "cost_micros_delta": 0,
		"tool_count_delta": 0, "turn_count_delta": 0,
	}, http.StatusOK, nil, headers); err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	ticket := s.ids["LT-1"]
	if err := s.api.do(s.admin, "", http.MethodPost, "/api/projects/"+s.lumenID+"/harness-sessions", map[string]any{
		"agent_principal_id": s.scribe.ID, "run_id": run.ID, "ticket_node_id": ticket,
		"work_order_id": order.NodeID, "harness": "codex", "host": "demo-workstation",
		"management_mode": "managed", "role": "worker", "work_shape": "ship",
		"advertised_capabilities": []string{"inbox", "status"},
		"harness_session_ref":     sessionRef, "worker_lease": lease,
	}, http.StatusCreated, nil, nil); err != nil {
		return fmt.Errorf("harness session: %w", err)
	}
	return s.pendingApproval()
}

func (s *seeder) pendingApproval() error {
	code, raw, err := s.api.call(s.clerk, s.clerkKey, http.MethodPost, "/api/approvals", map[string]any{
		"scope": "nodes.write", "resource_kind": "node", "resource_id": s.ids["HT-1"],
		"rationale":  "Fictional demo: Harbor Clerk asks to edit a ticket and nobody has answered.",
		"expires_at": time.Now().Add(72 * time.Hour).UTC(),
	}, nil)
	if err != nil {
		return err
	}
	if code != http.StatusCreated {
		return fmt.Errorf("pending approval: status %d: %s", code, clip(raw))
	}
	return nil
}

// jsonRaw marshals as already-encoded JSON.
type jsonRaw []byte

func (r jsonRaw) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	return r, nil
}
