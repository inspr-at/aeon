// SPDX-License-Identifier: AGPL-3.0-only

// Package directory serves the tenant's people and agents by name for the
// business screens: whose week of hours is shown, who logged time, whose period
// waits for approval and who approved it. It is a read model of principals
// (id, kind, name, roles) and grants nothing.
//
// Route, mounted by the coordinator next to the business plugin modules:
//
//	GET /api/business/principals
//
// Staff persons (admin or member) read it while at least one business plugin
// is enabled at its compiled digest. Every query runs through db.InTenant.
package directory

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/tenant"
)

// BusinessPlugins are the plugin IDs that open the directory.
var BusinessPlugins = []string{"business_costs", "business_crm", "business_quotes", "business_hours"}

// Principal is one tenant principal as business screens name it.
type Principal struct {
	ID    string   `json:"id"`
	Kind  string   `json:"kind"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

type module struct {
	pool *pgxpool.Pool
	reg  *plugins.Registry
}

// New returns the directory module. reg is the sealed registry holding the
// business plugins; a nil registry fails closed.
func New(pool *pgxpool.Pool, reg *plugins.Registry) httpapi.Module {
	return &module{pool: pool, reg: reg}
}

func (m *module) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/business/principals", m.list)
}

var errClosed = errors.New("no business plugin is enabled")

func (m *module) list(w http.ResponseWriter, r *http.Request) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok || p.ID == "" || p.TenantID == "" {
		httpapi.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if p.Kind != tenant.Person || (!slices.Contains(p.Roles, "admin") && !slices.Contains(p.Roles, "member")) {
		httpapi.WriteError(w, http.StatusForbidden, "admin or member person required")
		return
	}
	out := []Principal{}
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		if err := m.open(r.Context(), tx, p.TenantID); err != nil {
			return err
		}
		rows, err := tx.Query(r.Context(), `SELECT id::text, kind, name, roles FROM principals
			WHERE tenant_id = $1::uuid ORDER BY kind DESC, lower(name), id`, p.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item Principal
			if err := rows.Scan(&item.ID, &item.Kind, &item.Name, &item.Roles); err != nil {
				return err
			}
			if item.Roles == nil {
				item.Roles = []string{}
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	switch {
	case errors.Is(err, errClosed):
		httpapi.WriteError(w, http.StatusForbidden, errClosed.Error())
	case err != nil:
		httpapi.WriteError(w, http.StatusInternalServerError, "directory unavailable")
	default:
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteJSON(w, http.StatusOK, out)
	}
}

// open passes when any business plugin is enabled at its compiled digest.
func (m *module) open(ctx context.Context, tx pgx.Tx, tenantID string) error {
	if m.reg == nil {
		return errClosed
	}
	for _, id := range BusinessPlugins {
		plug, found := m.reg.Lookup(id)
		if !found {
			continue
		}
		var enabled bool
		var digest string
		err := tx.QueryRow(ctx, `SELECT enabled, manifest_digest_sha256 FROM plugin_installations
			WHERE tenant_id = $1::uuid AND plugin_id = $2`, tenantID, id).Scan(&enabled, &digest)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if enabled && digest == plug.Manifest.DigestSHA256 {
			return nil
		}
	}
	return errClosed
}
