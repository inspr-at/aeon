// SPDX-License-Identifier: AGPL-3.0-only

package imports

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/httpapi"
)

type Module struct {
	pool     *pgxpool.Pool
	inTenant func(context.Context, *pgxpool.Pool, string, func(pgx.Tx) error) error
}

// New returns an httpapi.Module serving read-only import status routes.
func New(pool *pgxpool.Pool) httpapi.Module {
	return &Module{pool: pool, inTenant: db.InTenant}
}

func (m *Module) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/imports", m.list)
	mux.HandleFunc("GET /api/imports/{importId}", m.get)
}
