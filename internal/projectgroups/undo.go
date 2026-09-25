// SPDX-License-Identifier: AGPL-3.0-only

package projectgroups

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/authz"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
)

// UndoHandlers reverses every shared group event. The coordinator registers it
// with events.WithUndoHandlers. Each handler requires an admin, locks the
// tenant's groups, and refuses (409) when the group changed since the event.
func UndoHandlers() map[string]events.UndoFunc {
	return map[string]events.UndoFunc{
		EventCreated:  undoCreated,
		EventUpdated:  undoUpdated,
		EventDeleted:  undoDeleted,
		EventAssigned: undoAssigned,
	}
}

func undoStart(ctx context.Context, tx pgx.Tx, p tenant.Principal) error {
	if authz.RequireTx(ctx, tx, p, "project_groups.write", authz.Scope{}) != nil {
		return events.ErrForbidden
	}
	return lockGroups(ctx, tx)
}

// A creation is undone by deleting the group, as long as it is as created, and
// giving its projects back to the shared groups they came from.
func undoCreated(ctx context.Context, tx pgx.Tx, p tenant.Principal, e events.Event) (events.Change, error) {
	var created snapshot
	if json.Unmarshal(e.After, &created) != nil || !validUUID(created.ID) {
		return events.Change{}, events.ErrConflict
	}
	if err := undoStart(ctx, tx, p); err != nil {
		return events.Change{}, err
	}
	current, err := loadGroup(ctx, tx, created.ID)
	if err != nil {
		return events.Change{}, events.ErrConflict
	}
	if current.Name != created.Name || current.Position != created.Position || !slices.Equal(current.ProjectIDs, created.ProjectIDs) {
		return events.Change{}, events.ErrConflict
	}
	if _, err := tx.Exec(ctx, `DELETE FROM project_groups WHERE id = $1::uuid`, created.ID); err != nil {
		return events.Change{}, err
	}
	for _, a := range created.Previous {
		if a.GroupID == nil {
			continue
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM project_groups WHERE id = $1::uuid)`, *a.GroupID).Scan(&exists); err != nil {
			return events.Change{}, err
		}
		if !exists {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO project_group_members (tenant_id, project_id, group_id)
			SELECT current_setting('aeon.tenant_id')::uuid, $1::uuid, $2::uuid
			WHERE EXISTS (SELECT 1 FROM nodes WHERE id = $1::uuid AND deleted_at IS NULL)
			ON CONFLICT (tenant_id, project_id) DO NOTHING`, a.ProjectID, *a.GroupID); err != nil {
			return events.Change{}, err
		}
	}
	return events.Change{Type: EventDeleted, Before: snapshot{Group: current}}, nil
}

// A deletion is undone by bringing the group back under its id and name, with
// the projects that are still live and not in another shared group since.
func undoDeleted(ctx context.Context, tx pgx.Tx, p tenant.Principal, e events.Event) (events.Change, error) {
	var deleted snapshot
	if json.Unmarshal(e.Before, &deleted) != nil || !validUUID(deleted.ID) || deleted.Name == "" {
		return events.Change{}, events.ErrConflict
	}
	if err := undoStart(ctx, tx, p); err != nil {
		return events.Change{}, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM project_groups WHERE id = $1::uuid)`, deleted.ID).Scan(&exists); err != nil {
		return events.Change{}, err
	}
	if exists {
		return events.Change{}, events.ErrConflict
	}
	if taken, err := nameTaken(ctx, tx, deleted.Name, ""); err != nil || taken {
		if err == nil {
			err = events.ErrConflict
		}
		return events.Change{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO project_groups (tenant_id, id, name, position, created_by, created_at)
		VALUES (current_setting('aeon.tenant_id')::uuid, $1::uuid, $2, $3, $4::uuid, $5)`,
		deleted.ID, deleted.Name, deleted.Position, deleted.CreatedBy, deleted.CreatedAt); err != nil {
		return events.Change{}, err
	}
	live, err := liveProjects(ctx, tx, deleted.ProjectIDs)
	if err != nil {
		return events.Change{}, err
	}
	back := make([]string, 0, len(deleted.ProjectIDs))
	current, err := currentAssignments(ctx, tx, deleted.ProjectIDs)
	if err != nil {
		return events.Change{}, err
	}
	for _, a := range current {
		if a.GroupID == nil && live[a.ProjectID] {
			back = append(back, a.ProjectID)
		}
	}
	id := deleted.ID
	if err := setMembers(ctx, tx, back, &id); err != nil {
		return events.Change{}, err
	}
	restored, err := loadGroup(ctx, tx, deleted.ID)
	if err != nil {
		return events.Change{}, err
	}
	return events.Change{Type: EventCreated, After: snapshot{Group: restored}}, nil
}

// A rename or reorder is undone while the group still reads as the event left it.
func undoUpdated(ctx context.Context, tx pgx.Tx, p tenant.Principal, e events.Event) (events.Change, error) {
	var before, after meta
	if json.Unmarshal(e.Before, &before) != nil || json.Unmarshal(e.After, &after) != nil || before.ID != after.ID || !validUUID(before.ID) {
		return events.Change{}, events.ErrConflict
	}
	if err := undoStart(ctx, tx, p); err != nil {
		return events.Change{}, err
	}
	current, err := loadMeta(ctx, tx, before.ID)
	if err != nil {
		return events.Change{}, events.ErrConflict
	}
	if current != after {
		return events.Change{}, events.ErrConflict
	}
	if taken, err := nameTaken(ctx, tx, before.Name, before.ID); err != nil || taken {
		if err == nil {
			err = events.ErrConflict
		}
		return events.Change{}, err
	}
	if err := writeMeta(ctx, tx, before); err != nil {
		return events.Change{}, err
	}
	return events.Change{Type: EventUpdated, Before: current, After: before}, nil
}

// Moves are undone while every project is still where the move put it.
func undoAssigned(ctx context.Context, tx pgx.Tx, p tenant.Principal, e events.Event) (events.Change, error) {
	var before, after assignments
	if json.Unmarshal(e.Before, &before) != nil || json.Unmarshal(e.After, &after) != nil || len(before.Assignments) == 0 || len(before.Assignments) != len(after.Assignments) {
		return events.Change{}, events.ErrConflict
	}
	if err := undoStart(ctx, tx, p); err != nil {
		return events.Change{}, err
	}
	ids := make([]string, 0, len(after.Assignments))
	for i, a := range after.Assignments {
		if !validUUID(a.ProjectID) || before.Assignments[i].ProjectID != a.ProjectID {
			return events.Change{}, events.ErrConflict
		}
		ids = append(ids, a.ProjectID)
	}
	current, err := currentAssignments(ctx, tx, ids)
	if err != nil {
		return events.Change{}, err
	}
	for i, a := range current {
		if !sameGroup(a.GroupID, after.Assignments[i].GroupID) {
			return events.Change{}, events.ErrConflict
		}
	}
	for _, a := range before.Assignments {
		if a.GroupID != nil {
			if _, err := loadMeta(ctx, tx, *a.GroupID); err != nil {
				return events.Change{}, events.ErrConflict
			}
		}
		if err := setMembers(ctx, tx, []string{a.ProjectID}, a.GroupID); err != nil {
			return events.Change{}, err
		}
	}
	change := events.Change{Type: EventAssigned, Before: after, After: before}
	if len(ids) == 1 {
		change.NodeID = &ids[0]
	}
	return change, nil
}
