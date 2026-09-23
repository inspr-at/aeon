// SPDX-License-Identifier: AGPL-3.0-only

package relations

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/business/crm"
	"github.com/inspr-at/aeon/internal/events"
)

var errGraph = errors.New("relation kind or direction is not allowed")

// enforceGraph checks CRM link kinds after both nodes are locked. Classic
// relation types are unchanged.
func enforceGraph(ctx context.Context, tx pgx.Tx, tenantID, source, target, relType string) error {
	if !crm.GraphType(relType) {
		return nil
	}
	sourceKind, err := nodeKindSlug(ctx, tx, tenantID, source)
	if err != nil {
		return err
	}
	targetKind, err := nodeKindSlug(ctx, tx, tenantID, target)
	if err != nil {
		return err
	}
	if !crm.GraphAllowed(sourceKind, targetKind, relType) {
		return errGraph
	}
	return nil
}

func nodeKindSlug(ctx context.Context, tx pgx.Tx, tenantID, nodeID string) (string, error) {
	var slug string
	err := tx.QueryRow(ctx, `SELECT k.slug
		FROM nodes n
		JOIN node_kinds k ON k.tenant_id = n.tenant_id AND k.id = n.kind_id
		WHERE n.tenant_id = $1::uuid AND n.id = $2::uuid AND n.deleted_at IS NULL`,
		tenantID, nodeID).Scan(&slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", events.ErrNotFound
	}
	return slug, err
}
