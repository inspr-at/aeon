// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/aeon/internal/embedding"
	"github.com/inspr-at/aeon/internal/httpapi"
)

// Module serves GET /api/search. New returns it as an httpapi.Module.
type Module struct {
	pool     *pgxpool.Pool
	provider embedding.Provider
}

// New returns the search module. A nil provider serves lexical search only.
func New(pool *pgxpool.Pool, provider embedding.Provider) httpapi.Module {
	return &Module{pool: pool, provider: provider}
}

// Mount registers GET /api/search.
func (m *Module) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/search", m.handleSearch)
}
