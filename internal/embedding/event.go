// SPDX-License-Identifier: AGPL-3.0-only

package embedding

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/tenant"
)

// eventEmbedded is the stable type for a stored node embedding.
const eventEmbedded = "node.embedded"

const actorName = "aeon-embedding"

// Change is one append-only event. The shape matches internal/events.Change:
// nil Before or After is SQL NULL, and at least one snapshot is required.
type Change struct {
	NodeID *string
	Type   string
	Before any
	After  any
}

// AppendFunc writes one event inside the caller's tenant transaction.
// events.Writer.Append is the implementation to inject when package events
// is available. It must not commit tx.
type AppendFunc func(ctx context.Context, tx pgx.Tx, p tenant.Principal, c Change) error

func sqlAppend(ctx context.Context, tx pgx.Tx, p tenant.Principal, c Change) error {
	before, err := snapshot(c.Before)
	if err != nil {
		return err
	}
	after, err := snapshot(c.After)
	if err != nil {
		return err
	}
	if before == nil && after == nil {
		return errors.New("event requires a snapshot")
	}
	_, err = tx.Exec(ctx, `INSERT INTO events
		(tenant_id, actor_principal_id, node_id, type, before, after)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		p.TenantID, p.ID, c.NodeID, c.Type, before, after)
	return err
}

func snapshot(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return nil, nil
	}
	return b, nil
}

// embeddingSnapshot is the before/after body of node.embedded. The halfvec
// is stored on node_embeddings; the hash identifies the document it came from.
type embeddingSnapshot struct {
	NodeID      string `json:"node_id"`
	Model       string `json:"model"`
	ContentHash string `json:"content_hash"`
}

func ensureActor(ctx context.Context, tx pgx.Tx, tenantID string) (tenant.Principal, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "embedding:"+tenantID); err != nil {
		return tenant.Principal{}, err
	}
	var id string
	err := tx.QueryRow(ctx, `
		SELECT id::text FROM principals
		WHERE tenant_id = $1 AND kind = 'agent' AND 'embedding' = ANY(roles)
		ORDER BY created_at, id
		LIMIT 1`, tenantID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			INSERT INTO principals (tenant_id, kind, name, roles)
			VALUES ($1, 'agent', $2, ARRAY['embedding'])
			RETURNING id::text`, tenantID, actorName).Scan(&id)
	}
	if err != nil {
		return tenant.Principal{}, err
	}
	return tenant.Principal{
		ID:       id,
		TenantID: tenantID,
		Kind:     tenant.Agent,
		Name:     actorName,
		Roles:    []string{"embedding"},
	}, nil
}
