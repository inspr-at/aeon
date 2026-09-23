// SPDX-License-Identifier: AGPL-3.0-only

// Package httpapi wires the HTTP server. Shared contract between P0.2 (owns the
// server) and P0.3 (mounts auth routes and middleware); extend, do not rename.
package httpapi

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is anything that adds routes under /api (auth, nodes, ...).
type Module interface {
	Mount(mux *http.ServeMux)
}

// Server holds what every module needs.
type Server struct {
	Mux        *http.ServeMux
	Pool       *pgxpool.Pool
	Middleware []func(http.Handler) http.Handler // applied outermost-first to /api
	Modules    []Module
}
