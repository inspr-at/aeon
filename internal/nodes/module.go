// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/httpapi"
)

// Module serves /api/kinds and /api/nodes.
type Module struct {
	pool   *pgxpool.Pool
	events Writer
}

var _ httpapi.Module = (*Module)(nil)

// New returns the httpapi.Module for /api/kinds and /api/nodes.
// The coordinator mounts it on the server; this package does not edit cmd/aeon.
// events records each mutation inside the tenant transaction. A nil events
// value selects SQLWriter until internal/events exposes its writer.
func New(pool *pgxpool.Pool, events Writer) httpapi.Module {
	if events == nil {
		events = SQLWriter{}
	}
	return &Module{pool: pool, events: events}
}

// Mount registers kind and node routes. Paths are the full /api paths the
// server mux expects.
func (m *Module) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/kinds", m.handleListKinds)
	mux.HandleFunc("POST /api/kinds", m.handleCreateKind)
	mux.HandleFunc("GET /api/kinds/{kindId}", m.handleGetKind)
	mux.HandleFunc("PATCH /api/kinds/{kindId}", m.handleUpdateKind)
	mux.HandleFunc("DELETE /api/kinds/{kindId}", m.handleDeleteKind)

	mux.HandleFunc("GET /api/nodes", m.handleListNodes)
	mux.HandleFunc("POST /api/nodes", m.handleCreateNode)
	mux.HandleFunc("GET /api/nodes/tree", m.handleTree)
	mux.HandleFunc("GET /api/nodes/{nodeId}", m.handleGetNode)
	mux.HandleFunc("PATCH /api/nodes/{nodeId}", m.handleUpdateNode)
	mux.HandleFunc("DELETE /api/nodes/{nodeId}", m.handleDeleteNode)
	mux.HandleFunc("POST /api/nodes/{nodeId}/move", m.handleMoveNode)
}

func (m *Module) tx(ctx context.Context, tenantID string, fn func(context.Context, pgx.Tx) error) error {
	return db.InTenant(ctx, m.pool, tenantID, func(tx pgx.Tx) error {
		return fn(ctx, tx)
	})
}

// lockTree serializes tree edits for this tenant on the same advisory key the
// node trigger uses, so position assignment and cycle checks cannot race.
func lockTree(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_setting('aeon.tenant_id', true), 0))`)
	return err
}
