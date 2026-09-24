// SPDX-License-Identifier: AGPL-3.0-only

package approvals

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
)

const (
	eventProposed = "approval.proposed"
	eventApproved = "approval.approved"
	eventDenied   = "approval.denied"
	eventRevoked  = "approval.revoked"
)

const approvalFrom = `
	SELECT r.id::text, r.agent_principal_id::text, r.scope, r.resource_kind,
	       r.resource_id::text, r.run_id::text, r.rationale, r.expires_at, r.proposed_at,
	       d.decision, d.decided_by_principal_id::text
	FROM approval_requests r
	LEFT JOIN approval_decisions d
	  ON d.tenant_id = r.tenant_id AND d.request_id = r.id`

// approvalSnapshot is the event body. revoked_at is audit detail; the HTTP
// Approval object does not include it.
type approvalSnapshot struct {
	Approval
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

func (m *Module) list(ctx context.Context, p tenant.Principal, limit int) ([]Approval, error) {
	if p.Kind != tenant.Person && p.Kind != tenant.Agent {
		return nil, fail(http.StatusForbidden, "forbidden")
	}
	var agentID *string
	if p.Kind == tenant.Agent {
		id := p.ID
		agentID = &id
	}
	var items []Approval
	err := m.inTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, approvalFrom+`
			WHERE ($1::uuid IS NULL OR r.agent_principal_id = $1::uuid)
			ORDER BY r.proposed_at DESC, r.id DESC
			LIMIT $2`, agentID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		items = []Approval{}
		for rows.Next() {
			item, err := scanApproval(rows)
			if err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (m *Module) propose(ctx context.Context, p tenant.Principal, authorization string, in proposal) (Approval, error) {
	var out Approval
	err := m.inTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		scopes, err := lookupKeyScopes(ctx, tx, authorization, p.ID)
		if err != nil {
			return err
		}
		if !ScopeWithinKey(in.Scope, scopes) {
			return fail(http.StatusForbidden, "scope exceeds the API key")
		}
		if err := verifyResource(ctx, tx, p.ID, in); err != nil {
			return err
		}
		var id string
		err = tx.QueryRow(ctx, `
			INSERT INTO approval_requests (
				tenant_id, proposed_by_principal_id, agent_principal_id, run_id,
				scope, resource_kind, resource_id, rationale, expires_at)
			VALUES ($1::uuid, $2::uuid, $2::uuid, $3::uuid, $4, $5, $6::uuid, $7, $8)
			RETURNING id::text`,
			p.TenantID, p.ID, in.RunID, in.Scope, in.ResourceKind, in.ResourceID, in.Rationale, in.ExpiresAt,
		).Scan(&id)
		if err != nil {
			return err
		}
		out, err = loadApproval(ctx, tx, id)
		if err != nil {
			return err
		}
		_, err = events.Append(ctx, tx, p, events.Change{
			Type:   eventProposed,
			After:  out,
			NodeID: nodeRef(out),
		})
		return err
	})
	return out, err
}

func (m *Module) decide(ctx context.Context, p tenant.Principal, id, decision, reason string) (Approval, error) {
	var out Approval
	err := m.inTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		before, expired, err := lockRequest(ctx, tx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return fail(http.StatusNotFound, "approval not found")
		}
		if err != nil {
			return err
		}
		if err := requirePerson(ctx, tx, p, "only a person may decide a live approval"); err != nil {
			return err
		}
		var existing string
		err = tx.QueryRow(ctx, `
			SELECT decision FROM approval_decisions WHERE request_id = $1::uuid`, id).Scan(&existing)
		if err == nil {
			return fail(http.StatusConflict, "approval is already decided")
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if expired {
			return fail(http.StatusConflict, "approval has expired")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO approval_decisions (
				tenant_id, request_id, decided_by_principal_id, decision, reason)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5)`,
			p.TenantID, id, p.ID, decision, reason); err != nil {
			return err
		}
		if decision == "approved" {
			tag, err := tx.Exec(ctx, `
				INSERT INTO agent_permission_grants (
					tenant_id, approval_request_id, agent_principal_id,
					scope, resource_kind, resource_id, valid_until)
				SELECT r.tenant_id, r.id, r.agent_principal_id,
				       r.scope, r.resource_kind, r.resource_id, r.expires_at
				FROM approval_requests r
				JOIN approval_decisions d
				  ON d.tenant_id = r.tenant_id AND d.request_id = r.id
				WHERE r.id = $1::uuid AND d.decision = 'approved'`, id)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return errors.New("approved grant was not inserted")
			}
		}
		out, err = loadApproval(ctx, tx, id)
		if err != nil {
			return err
		}
		eventType := eventDenied
		if decision == "approved" {
			eventType = eventApproved
		}
		_, err = events.Append(ctx, tx, p, events.Change{
			Type:   eventType,
			Before: before,
			After:  out,
			NodeID: nodeRef(out),
		})
		return err
	})
	return out, err
}

func (m *Module) revoke(ctx context.Context, p tenant.Principal, id string) (Approval, error) {
	var out Approval
	err := m.inTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		if _, _, err = lockRequest(ctx, tx, id); errors.Is(err, pgx.ErrNoRows) {
			return fail(http.StatusNotFound, "approval not found")
		} else if err != nil {
			return err
		}
		if err := requirePerson(ctx, tx, p, "only a person may revoke a grant"); err != nil {
			return err
		}
		var revokedAt *time.Time
		err = tx.QueryRow(ctx, `
			SELECT revoked_at FROM agent_permission_grants
			WHERE approval_request_id = $1::uuid
			FOR UPDATE`, id).Scan(&revokedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return fail(http.StatusConflict, "only an approved grant can be revoked")
		}
		if err != nil {
			return err
		}
		out, err = loadApproval(ctx, tx, id)
		if err != nil {
			return err
		}
		if revokedAt != nil {
			return nil
		}
		var at time.Time
		err = tx.QueryRow(ctx, `
			UPDATE agent_permission_grants
			SET revoked_at = now()
			WHERE approval_request_id = $1::uuid AND revoked_at IS NULL
			RETURNING revoked_at`, id).Scan(&at)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = events.Append(ctx, tx, p, events.Change{
			Type:   eventRevoked,
			Before: out,
			After:  approvalSnapshot{Approval: out, RevokedAt: &at},
			NodeID: nodeRef(out),
		})
		return err
	})
	return out, err
}

func requirePerson(ctx context.Context, tx pgx.Tx, p tenant.Principal, msg string) error {
	if p.Kind != tenant.Person {
		return fail(http.StatusForbidden, msg)
	}
	var kind string
	err := tx.QueryRow(ctx, `SELECT kind FROM principals WHERE id = $1::uuid`, p.ID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && kind != string(tenant.Person)) {
		return fail(http.StatusForbidden, msg)
	}
	return err
}

func verifyResource(ctx context.Context, tx pgx.Tx, agentID string, in proposal) error {
	switch in.ResourceKind {
	case "node":
		if err := liveNode(ctx, tx, *in.ResourceID); err != nil {
			return err
		}
	case "run":
		if err := ownRun(ctx, tx, agentID, *in.ResourceID); err != nil {
			return err
		}
	}
	if in.RunID != nil && in.ResourceKind != "run" {
		return ownRun(ctx, tx, agentID, *in.RunID)
	}
	return nil
}

func liveNode(ctx context.Context, tx pgx.Tx, id string) error {
	var ok bool
	err := tx.QueryRow(ctx, `
		SELECT true FROM nodes WHERE id = $1::uuid AND deleted_at IS NULL`, id).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(http.StatusForbidden, "resource is not in this tenant")
	}
	return err
}

func ownRun(ctx context.Context, tx pgx.Tx, agentID, runID string) error {
	var owner string
	err := tx.QueryRow(ctx, `
		SELECT agent_principal_id::text FROM agent_runs WHERE id = $1::uuid`, runID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(http.StatusForbidden, "resource is not in this tenant")
	}
	if err != nil {
		return err
	}
	if owner != agentID {
		return fail(http.StatusForbidden, "run belongs to another agent")
	}
	return nil
}

func lockRequest(ctx context.Context, tx pgx.Tx, id string) (Approval, bool, error) {
	var a Approval
	var resourceID, runID *string
	var expired bool
	err := tx.QueryRow(ctx, `
		SELECT id::text, agent_principal_id::text, scope, resource_kind,
		       resource_id::text, run_id::text, rationale, expires_at, proposed_at,
		       expires_at <= now()
		FROM approval_requests
		WHERE id = $1::uuid
		FOR UPDATE`, id).Scan(
		&a.ID, &a.AgentPrincipalID, &a.Scope, &a.ResourceKind,
		&resourceID, &runID, &a.Rationale, &a.ExpiresAt, &a.ProposedAt, &expired)
	a.ResourceID = resourceID
	a.RunID = runID
	a.Risk = Risk(a.Scope, a.ResourceKind)
	return a, expired, err
}

func loadApproval(ctx context.Context, tx pgx.Tx, id string) (Approval, error) {
	return scanApproval(tx.QueryRow(ctx, approvalFrom+` WHERE r.id = $1::uuid`, id))
}

func scanApproval(row pgx.Row) (Approval, error) {
	var a Approval
	var resourceID, runID, decision, decidedBy *string
	err := row.Scan(
		&a.ID, &a.AgentPrincipalID, &a.Scope, &a.ResourceKind,
		&resourceID, &runID, &a.Rationale, &a.ExpiresAt, &a.ProposedAt,
		&decision, &decidedBy)
	a.ResourceID = resourceID
	a.RunID = runID
	a.Risk = Risk(a.Scope, a.ResourceKind)
	a.Decision = decision
	a.DecidedByPrincipalID = decidedBy
	return a, err
}

func nodeRef(a Approval) *string {
	if a.ResourceKind != "node" || a.ResourceID == nil {
		return nil
	}
	id := *a.ResourceID
	return &id
}
