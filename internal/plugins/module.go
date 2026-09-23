// SPDX-License-Identifier: AGPL-3.0-only
package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Installation struct {
	PluginID             string    `json:"plugin_id"`
	Version              string    `json:"version"`
	ManifestDigestSHA256 string    `json:"manifest_digest_sha256"`
	Enabled              bool      `json:"enabled"`
	Permissions          []string  `json:"permissions"`
	UpdatedAt            time.Time `json:"updated_at"`
}
type installationWrite struct {
	ManifestDigestSHA256 string   `json:"manifest_digest_sha256"`
	Enabled              bool     `json:"enabled"`
	Permissions          []string `json:"permissions"`
}
type listedManifest struct {
	Manifest
	Installation *Installation `json:"installation"`
}
type Module struct {
	pool     *pgxpool.Pool
	registry *Registry
}

var _ httpapi.Module = (*Module)(nil)

// New returns an unmounted httpapi.Module. The coordinator mounts it after
// registering compiled manifests and steps with pharos.Register and
// janus.Register.
func New(pool *pgxpool.Pool, registry *Registry) httpapi.Module {
	return &Module{pool: pool, registry: registry}
}
func (m *Module) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/plugins", m.list)
	mux.HandleFunc("PUT /api/plugins/{pluginId}/installation", m.configure)
}
func principal(w http.ResponseWriter, r *http.Request) (tenant.Principal, bool) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok || p.ID == "" || p.TenantID == "" {
		httpapi.WriteError(w, 401, "authentication required")
		return tenant.Principal{}, false
	}
	return p, true
}
func respond(w http.ResponseWriter, v any, err error, status int) {
	w.Header().Set("Cache-Control", "no-store")
	if err == nil {
		httpapi.WriteJSON(w, status, v)
		return
	}
	var e *apiError
	if errors.As(err, &e) {
		httpapi.WriteError(w, e.status, e.message)
		return
	}
	slog.Error("plugins", "err", err)
	httpapi.WriteError(w, 500, "internal")
}

type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string             { return e.message }
func reject(status int, message string) error { return &apiError{status, message} }
func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r)
	if !ok {
		return
	}
	out := []listedManifest{}
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		for _, man := range m.registry.List() {
			item := listedManifest{Manifest: man}
			var ins Installation
			err := tx.QueryRow(r.Context(), `SELECT plugin_id,version,manifest_digest_sha256,enabled,permissions,updated_at FROM plugin_installations WHERE plugin_id=$1`, man.ID).Scan(&ins.PluginID, &ins.Version, &ins.ManifestDigestSHA256, &ins.Enabled, &ins.Permissions, &ins.UpdatedAt)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err == nil {
				item.Installation = &ins
			}
			out = append(out, item)
		}
		return nil
	})
	respond(w, out, err, 200)
}
func (m *Module) configure(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r)
	if !ok {
		return
	}
	if p.Kind != tenant.Person || !slices.Contains(p.Roles, "admin") {
		httpapi.WriteError(w, 403, "tenant admin required")
		return
	}
	id := r.PathValue("pluginId")
	man, ok := m.registry.Get(id)
	if !ok {
		httpapi.WriteError(w, 404, "plugin not found")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var in installationWrite
	if err := dec.Decode(&in); err != nil {
		httpapi.WriteError(w, 400, "invalid installation")
		return
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		httpapi.WriteError(w, 400, "invalid installation")
		return
	}
	if in.ManifestDigestSHA256 != man.DigestSHA256 {
		httpapi.WriteError(w, 409, "manifest digest mismatch")
		return
	}
	perms := map[string]bool{}
	for _, permission := range in.Permissions {
		if perms[permission] || !slices.Contains(man.Permissions, permission) {
			httpapi.WriteError(w, 400, "invalid permission subset")
			return
		}
		perms[permission] = true
	}
	var out Installation
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		var admin bool
		if err := tx.QueryRow(r.Context(), `SELECT kind='person' AND 'admin'=ANY(roles) FROM principals WHERE id=$1::uuid`, p.ID).Scan(&admin); err != nil {
			return err
		}
		if !admin {
			return reject(403, "tenant admin required")
		}
		var before *Installation
		var old Installation
		err := tx.QueryRow(r.Context(), `SELECT plugin_id,version,manifest_digest_sha256,enabled,permissions,updated_at FROM plugin_installations WHERE plugin_id=$1 FOR UPDATE`, id).Scan(&old.PluginID, &old.Version, &old.ManifestDigestSHA256, &old.Enabled, &old.Permissions, &old.UpdatedAt)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			before = &old
		}
		err = tx.QueryRow(r.Context(), `INSERT INTO plugin_installations(tenant_id,plugin_id,version,manifest_digest_sha256,owner,enabled,permissions,updated_by_principal_id) VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8::uuid) ON CONFLICT(tenant_id,plugin_id) DO UPDATE SET version=EXCLUDED.version,manifest_digest_sha256=EXCLUDED.manifest_digest_sha256,owner=EXCLUDED.owner,enabled=EXCLUDED.enabled,permissions=EXCLUDED.permissions,updated_by_principal_id=EXCLUDED.updated_by_principal_id,updated_at=now() RETURNING plugin_id,version,manifest_digest_sha256,enabled,permissions,updated_at`, p.TenantID, id, man.Version, man.DigestSHA256, man.Owner, in.Enabled, in.Permissions, p.ID).Scan(&out.PluginID, &out.Version, &out.ManifestDigestSHA256, &out.Enabled, &out.Permissions, &out.UpdatedAt)
		if err != nil {
			return err
		}
		ev, err := events.Append(r.Context(), tx, p, events.Change{Type: "plugin.installation_changed", Before: before, After: out})
		if err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO plugin_installation_events(tenant_id,plugin_id,event_id,manifest_digest_sha256,enabled,permissions) VALUES($1::uuid,$2,$3,$4,$5,$6)`, p.TenantID, id, ev.ID, man.DigestSHA256, in.Enabled, in.Permissions)
		return err
	})
	respond(w, out, err, 200)
}

// Enabled verifies the exact compiled digest and requested permission inside a
// caller's tenant transaction. Disabled or mismatched installations fail closed.
func Enabled(ctx context.Context, tx pgx.Tx, registry *Registry, id, permission string) (bool, error) {
	m, ok := registry.Get(id)
	if !ok || !slices.Contains(m.Permissions, permission) {
		return false, nil
	}
	var digest string
	var enabled bool
	var perms []string
	err := tx.QueryRow(ctx, `SELECT manifest_digest_sha256,enabled,permissions FROM plugin_installations WHERE plugin_id=$1`, id).Scan(&digest, &enabled, &perms)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return enabled && strings.EqualFold(digest, m.DigestSHA256) && slices.Contains(perms, permission), nil
}
