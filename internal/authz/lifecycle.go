// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"errors"
	"net/http"
	"strings"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/jackc/pgx/v5"
)

func (m *Module) deactivate(w http.ResponseWriter, r *http.Request) {
	m.setStatus(w, r, "deactivated")
}

func (m *Module) reactivate(w http.ResponseWriter, r *http.Request) {
	m.setStatus(w, r, "active")
}

func (m *Module) setStatus(w http.ResponseWriter, r *http.Request, next string) {
	p := actor(r)
	id := r.PathValue("principal_id")
	if !uuidPattern.MatchString(id) {
		apiFail(w, 400, "invalid", "principal_id", "Principal ID must be a UUID")
		return
	}
	var member Member
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		if err := m.authorizeMutation(r.Context(), tx, p, "members.manage", nil); err != nil {
			return err
		}
		var kind, status string
		var legacy []string
		var linked *string
		if err := tx.QueryRow(r.Context(), `SELECT kind,status,roles,linked_to::text FROM principals WHERE tenant_id=$1::uuid AND id=$2::uuid FOR UPDATE`, p.TenantID, id).Scan(&kind, &status, &legacy, &linked); err != nil {
			return err
		}
		if linked != nil {
			return errAliasTarget
		}
		if kind == "agent" {
			for _, v := range legacy {
				if v == "system" || v == "importer" || v == "operator" || v == "embedding" || strings.HasPrefix(v, "quote_") {
					return ErrForbidden
				}
			}
		}
		want := "active"
		if next == "deactivated" {
			want = "deactivated"
		}
		if status == want {
			return errStatusUnchanged
		}
		if _, err := tx.Exec(r.Context(), `UPDATE principals SET status=$3 WHERE tenant_id=$1::uuid AND id=$2::uuid`, p.TenantID, id, want); err != nil {
			return err
		}
		typ := "principal.reactivated"
		if want == "deactivated" {
			typ = "principal.deactivated"
		}
		if err := appendEvent(r.Context(), tx, p, typ, map[string]any{"principal_id": id, "status": status}, map[string]any{"principal_id": id, "status": want}); err != nil {
			return err
		}
		var err error
		member, err = readMember(r.Context(), tx, p.TenantID, id)
		return err
	})
	if errors.Is(err, errAliasTarget) {
		apiFail(w, 409, "conflict", "principal_id", "Change the person this alias belongs to")
		return
	}
	if errors.Is(err, errStatusUnchanged) {
		reason := "This person is already active"
		if next == "deactivated" {
			reason = "This person is already deactivated"
		}
		apiFail(w, 409, "conflict", "principal_id", reason)
		return
	}
	if lastOwnerViolation(err) {
		apiFail(w, 409, "last_owner", "principal_id", "The last active owner cannot be deactivated. Make another person an owner first.")
		return
	}
	if err != nil {
		internalFail(w, err)
		return
	}
	reply(w, http.StatusOK, member)
}

var (
	errAliasTarget     = errors.New("alias target")
	errStatusUnchanged = errors.New("status unchanged")
)
