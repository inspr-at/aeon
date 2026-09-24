// SPDX-License-Identifier: AGPL-3.0-only
package attachments

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

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
	if !reflect.DeepEqual(current, expected) {
		return events.Change{}, events.ErrConflict
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
