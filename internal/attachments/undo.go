// SPDX-License-Identifier: AGPL-3.0-only
package attachments

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/inspr-at/aeon/internal/authz"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// UndoHandlers is passed by the coordinator to events.WithUndoHandlers.
func UndoHandlers() map[string]events.UndoFunc {
	return map[string]events.UndoFunc{"attachment.added": undo, "attachment.updated": undo, "attachment.removed": undo}
}
func undo(ctx context.Context, tx pgx.Tx, p tenant.Principal, e events.Event) (events.Change, error) {
	var previous, expected Attachment
	if len(e.After) == 0 || json.Unmarshal(e.After, &expected) != nil || !uuid(expected.ID) {
		return events.Change{}, events.ErrConflict
	}
	if len(e.Before) > 0 && json.Unmarshal(e.Before, &previous) != nil {
		return events.Change{}, events.ErrConflict
	}
	current, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM attachments WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, expected.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return events.Change{}, events.ErrNotFound
	}
	if err != nil {
		return events.Change{}, err
	}
	if !reflect.DeepEqual(comparable(current), comparable(expected)) {
		return events.Change{}, events.ErrConflict
	}
	// Undo needs what the reversing change needs, in the attachment's
	// project: removing an added file, or writing one back (ADR-003 P2).
	permission := "attachments.write"
	if e.Type == "attachment.added" {
		permission = "attachments.delete"
	}
	var project *string
	if err := tx.QueryRow(ctx, `SELECT project_id::text FROM nodes WHERE tenant_id=$1 AND id=$2`, p.TenantID, current.NodeID).Scan(&project); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return events.Change{}, events.ErrConflict
		}
		return events.Change{}, err
	}
	scope := ""
	if project != nil {
		scope = *project
	}
	if authz.RequireInProjects(ctx, tx, p, permission, scope) != nil {
		return events.Change{}, events.ErrForbidden
	}
	var restored Attachment
	if e.Type == "attachment.added" {
		restored, err = scan(tx.QueryRow(ctx, `UPDATE attachments SET deleted_at=clock_timestamp(),updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 RETURNING `+columns, p.TenantID, current.ID))
	} else {
		restored, err = scan(tx.QueryRow(ctx, `UPDATE attachments SET caption=$3,position=$4,deleted_at=$5,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 RETURNING `+columns, p.TenantID, current.ID, previous.Caption, previous.Position, previous.DeletedAt))
	}
	if err != nil {
		return events.Change{}, err
	}
	return events.Change{NodeID: &current.NodeID, Type: e.Type, Before: current, After: restored}, nil
}

// comparable normalises the timestamps of an attachment snapshot so a row
// scanned from Postgres and one decoded from event JSON compare equal whatever
// the host time zone: same instant, UTC location, microsecond precision.
func comparable(a Attachment) Attachment {
	a.CreatedAt = a.CreatedAt.UTC().Truncate(time.Microsecond)
	a.UpdatedAt = a.UpdatedAt.UTC().Truncate(time.Microsecond)
	if a.DeletedAt != nil {
		t := a.DeletedAt.UTC().Truncate(time.Microsecond)
		a.DeletedAt = &t
	}
	return a
}
