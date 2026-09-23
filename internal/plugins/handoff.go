// SPDX-License-Identifier: AGPL-3.0-only

package plugins

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/tenant"
)

// Lookup returns a copy of a compiled plugin's declaration and bindings.
func (r *Registry) Lookup(id string) (Plugin, bool) { return r.plugin(id) }

// Step returns the compiled step implementation, if one is registered.
func (r *Registry) Step(id string) (StepPlugin, bool) {
	p, ok := r.plugin(id)
	return p.Steps, ok && p.Steps != nil
}

// Enabled checks a workflow operation against its compiled permission binding
// and the exact installed digest inside the caller's db.InTenant transaction.
// It authorizes starting handoff work, not a host change or a terminal result.
func Enabled(ctx context.Context, tx pgx.Tx, registry *Registry, tenantID, id, operation string) (bool, error) {
	p, ok := registry.plugin(id)
	if !ok {
		return false, nil
	}
	permission, ok := p.StepPermissions[operation]
	if !ok || !slices.Contains(p.Manifest.Permissions, permission) {
		return false, nil
	}
	row, err := loadInstall(ctx, tx, tenantID, id, false)
	if err != nil || row == nil {
		return false, err
	}
	return row.enabled && row.digest == p.Manifest.DigestSHA256 && slices.Contains(row.perms, permission), nil
}

// AuthorizeHandoff checks the permission-only boundary of a host-managed
// handoff. The host retains its transactional gates, evidence checks and
// one-use launch admission. This does not invoke the typed StepPlugin hooks:
// those require observed facts that do not yet exist when a handoff is queued.
// New work must also pass Enabled; an existing attempt may finish after disable.
func (r *Registry) AuthorizeHandoff(ctx context.Context, principal tenant.Principal, id, operation string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, ok := r.plugin(id)
	if !ok || p.Steps == nil {
		return ErrClosed
	}
	permission, ok := p.StepPermissions[operation]
	if !ok || principal.ID == "" || principal.TenantID == "" {
		return ErrDenied
	}
	call := Call{Principal: principal, Grant: grantOf(p.Manifest.Permissions).Narrow(permission)}
	if !call.Grant.Allows(permission) {
		return ErrDenied
	}
	return nil
}
