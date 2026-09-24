// SPDX-License-Identifier: AGPL-3.0-only

package crm

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/inspr-at/aeon/internal/tenant"
)

// BoundPrincipal is one binding as the CRM screen shows it: the binding plus
// the bound principal's display name and kind. It grants nothing; acceptance
// rechecks the binding inside its own transaction.
type BoundPrincipal struct {
	Binding
	PrincipalName string `json:"principal_name"`
	PrincipalKind string `json:"principal_kind"`
}

// bindings lists who is bound to a contact. Staff persons (admin or member)
// read it while business_crm is enabled at its compiled digest with
// views.provide; the list is scoped to the caller's tenant by db.InTenant.
func (m *module) bindings(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r)
	if !ok {
		return
	}
	if p.Kind != tenant.Person || (!slices.Contains(p.Roles, "admin") && !slices.Contains(p.Roles, "member")) {
		writeErr(w, errForbidden)
		return
	}
	contactID, ok := parseUUID(r.PathValue("contactId"))
	if !ok {
		writeErr(w, errInvalid("invalid contact id"))
		return
	}
	out := []BoundPrincipal{}
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		if err := m.viewOpen(r.Context(), tx, p.TenantID); err != nil {
			return err
		}
		var slug string
		err := tx.QueryRow(r.Context(), `SELECT k.slug FROM nodes n JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id
			WHERE n.tenant_id=$1::uuid AND n.id=$2::uuid AND n.deleted_at IS NULL`, p.TenantID, contactID).Scan(&slug)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && slug != Contact) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(r.Context(), `SELECT cp.principal_id::text, cp.contact_node_id::text, cp.bound_by_principal_id::text, cp.bound_at, pr.name, pr.kind
			FROM crm_contact_principals cp
			JOIN principals pr ON pr.tenant_id = cp.tenant_id AND pr.id = cp.principal_id
			WHERE cp.tenant_id = $1::uuid AND cp.contact_node_id = $2::uuid
			ORDER BY cp.bound_at, cp.principal_id`, p.TenantID, contactID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b BoundPrincipal
			if err := rows.Scan(&b.PrincipalID, &b.ContactNodeID, &b.BoundByPrincipalID, &b.BoundAt, &b.PrincipalName, &b.PrincipalKind); err != nil {
				return err
			}
			out = append(out, b)
		}
		return rows.Err()
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpapi.WriteJSON(w, http.StatusOK, out)
}

// viewOpen requires the installation at the compiled digest with views.provide.
func (m *module) viewOpen(ctx context.Context, tx pgx.Tx, tenantID string) error {
	if m.reg == nil {
		return errClosed
	}
	plug, found := m.reg.Lookup(ID)
	if !found {
		return errClosed
	}
	var enabled bool
	var digest string
	var perms []string
	err := tx.QueryRow(ctx, `SELECT enabled, manifest_digest_sha256, permissions FROM plugin_installations
		WHERE tenant_id = $1::uuid AND plugin_id = $2`, tenantID, ID).Scan(&enabled, &digest, &perms)
	if errors.Is(err, pgx.ErrNoRows) {
		return errClosed
	}
	if err != nil {
		return err
	}
	if !enabled || digest != plug.Manifest.DigestSHA256 || !slices.Contains(perms, fence.PermViewsProvide) {
		return errClosed
	}
	return nil
}
