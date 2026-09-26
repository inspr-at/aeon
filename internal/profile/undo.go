// SPDX-License-Identifier: AGPL-3.0-only
package profile

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/authz"
	"github.com/inspr-at/paimos/internal/events"
	"github.com/inspr-at/paimos/internal/tenant"
)

// UndoHandlers is registered by the coordinator with events.WithUndoHandlers.
func UndoHandlers() map[string]events.UndoFunc {
	return map[string]events.UndoFunc{"profile.updated": undo}
}
func undo(ctx context.Context, tx pgx.Tx, p tenant.Principal, e events.Event) (events.Change, error) {
	var expected State
	if json.Unmarshal(e.After, &expected) != nil || expected.PrincipalID == "" {
		return events.Change{}, events.ErrConflict
	}
	if expected.PrincipalID != p.ID {
		if authz.RequireTx(ctx, tx, p, "profile.manage", authz.Scope{}) != nil {
			return events.Change{}, events.ErrForbidden
		}
	}
	current, exists, err := read(ctx, tx, p.TenantID, expected.PrincipalID, true)
	if err != nil {
		return events.Change{}, err
	}
	if !exists || !same(current, expected) {
		return events.Change{}, events.ErrConflict
	}
	if len(e.Before) == 0 {
		_, err = tx.Exec(ctx, `DELETE FROM personal_profiles WHERE tenant_id=$1 AND principal_id=$2`, p.TenantID, expected.PrincipalID)
		if err != nil {
			return events.Change{}, err
		}
		return events.Change{Type: "profile.updated", Before: current, After: defaultState(expected.PrincipalID)}, nil
	}
	var prior State
	if err = json.Unmarshal(e.Before, &prior); err != nil {
		return events.Change{}, events.ErrConflict
	}
	if prior.PrincipalID != expected.PrincipalID {
		return events.Change{}, events.ErrConflict
	}
	prior.Revision = current.Revision + 1
	if err = save(ctx, tx, p.TenantID, prior); err != nil {
		return events.Change{}, err
	}
	return events.Change{Type: "profile.updated", Before: current, After: prior}, nil
}
