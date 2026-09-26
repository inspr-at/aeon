// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/inspr-at/aeon/internal/authz"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// UndoHandlers lets the event module reverse a move while retaining both key
// histories, and a bulk change as a whole. A later edit or move closes the
// stale undo path.
func UndoHandlers() map[string]events.UndoFunc {
	return map[string]events.UndoFunc{evNodeMoved: undoMove, evNodeProjectMoved: undoProjectMove, evNodeBulkChanged: undoBulk}
}

func undoMove(ctx context.Context, tx pgx.Tx, p tenant.Principal, e events.Event) (events.Change, error) {
	var before, after nodeJSON
	if e.NodeID == nil || json.Unmarshal(e.Before, &before) != nil || json.Unmarshal(e.After, &after) != nil ||
		before.ID != *e.NodeID || after.ID != *e.NodeID || before.ID != after.ID {
		return events.Change{}, events.ErrConflict
	}
	if err := lockTree(ctx, tx); err != nil {
		return events.Change{}, err
	}
	restored, err := restoreMovedNode(ctx, tx, p, before, after)
	if err != nil {
		return events.Change{}, err
	}
	return events.Change{NodeID: e.NodeID, Type: evNodeMoved, Before: after, After: restored}, nil
}

func undoProjectMove(ctx context.Context, tx pgx.Tx, p tenant.Principal, e events.Event) (events.Change, error) {
	var before, after projectMoveSnapshot
	if e.NodeID == nil || json.Unmarshal(e.Before, &before) != nil || json.Unmarshal(e.After, &after) != nil ||
		before.Node.ID != *e.NodeID || after.Node.ID != *e.NodeID || before.Node.ID != after.Node.ID {
		return events.Change{}, events.ErrConflict
	}
	if err := lockTree(ctx, tx); err != nil {
		return events.Change{}, err
	}
	currentJourney, err := loadJourneyMembership(ctx, tx, after.Node.ID)
	if err != nil {
		return events.Change{}, err
	}
	if !reflect.DeepEqual(currentJourney, after.Journey) {
		return events.Change{}, events.ErrConflict
	}
	// The undo rewrites journey rows of both projects (ADR-003 P2).
	var journeyProjects []string
	for _, j := range []*journeyMembership{before.Journey, after.Journey} {
		if j != nil {
			journeyProjects = append(journeyProjects, j.ProjectID)
		}
	}
	if err := authz.RequireInProjects(ctx, tx, p, "nodes.move", journeyProjects...); err != nil {
		return events.Change{}, events.ErrForbidden
	}
	restored, err := restoreMovedNode(ctx, tx, p, before.Node, after.Node)
	if err != nil {
		return events.Change{}, err
	}
	if before.Journey == nil {
		if after.Journey != nil {
			return events.Change{}, events.ErrConflict
		}
	} else if after.Journey == nil {
		_, err = tx.Exec(ctx, `INSERT INTO journey_tickets(tenant_id,ticket_node_id,project_node_id,feature_node_id,release_node_id,
		 walker_position,source,scope_revision_required,access_change,estimated_hours)
		 VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,$7,$8,$9,$10::numeric)`,
			p.TenantID, after.Node.ID, before.Journey.ProjectID, before.Journey.FeatureID, before.Journey.ReleaseID,
			before.Journey.WalkerPosition, before.Journey.Source, before.Journey.ScopeRevisionRequired,
			before.Journey.AccessChange, before.Journey.EstimatedHours)
	} else {
		_, err = tx.Exec(ctx, `UPDATE journey_tickets SET project_node_id=$2::uuid,feature_node_id=$3::uuid,release_node_id=$4::uuid,
		 walker_position=$5,source=$6,scope_revision_required=$7,access_change=$8,estimated_hours=$9::numeric
		 WHERE ticket_node_id=$1::uuid`, after.Node.ID, before.Journey.ProjectID, before.Journey.FeatureID,
			before.Journey.ReleaseID, before.Journey.WalkerPosition, before.Journey.Source,
			before.Journey.ScopeRevisionRequired, before.Journey.AccessChange, before.Journey.EstimatedHours)
	}
	if err != nil {
		return events.Change{}, events.ErrConflict
	}
	if before.Journey != nil {
		if _, err := tx.Exec(ctx, `UPDATE journey_projects SET revision=revision+1,updated_at=now()
		 WHERE project_node_id=$1::uuid OR project_node_id=$2::uuid`, before.Journey.ProjectID, after.Node.ParentID); err != nil {
			return events.Change{}, err
		}
	}
	return events.Change{NodeID: e.NodeID, Type: evNodeProjectMoved,
		Before: projectMoveSnapshot{Node: after.Node, Journey: after.Journey},
		After:  projectMoveSnapshot{Node: restored, Journey: before.Journey}}, nil
}

// restoreMovedNode moves a node back to its former parent. Like a move, the
// undo needs nodes.move in every project it changes: where the node is now and
// where it returns to (ADR-003 P2). An event's author or a project's admin
// cannot reverse a move into a project where they hold less.
func restoreMovedNode(ctx context.Context, tx pgx.Tx, p tenant.Principal, before, after nodeJSON) (nodeJSON, error) {
	current, err := loadNode(ctx, tx, after.ID, true)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nodeJSON{}, events.ErrConflict
		}
		return nodeJSON{}, err
	}
	if err := requireMove(ctx, tx, p, after.ID, before.ParentID); err != nil {
		if errors.Is(err, errMoveForbidden) {
			return nodeJSON{}, events.ErrForbidden
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nodeJSON{}, events.ErrConflict
		}
		return nodeJSON{}, err
	}
	if current.Key != after.Key || current.Position != after.Position || !sameString(current.ParentID, after.ParentID) ||
		!current.UpdatedAt.Equal(after.UpdatedAt) {
		return nodeJSON{}, events.ErrConflict
	}
	if before.Key != after.Key {
		command, err := tx.Exec(ctx, `DELETE FROM node_key_aliases WHERE key=$1 AND node_id=$2::uuid`, before.Key, after.ID)
		if err != nil || command.RowsAffected() != 1 {
			return nodeJSON{}, events.ErrConflict
		}
	}
	restored, err := scanNode(tx.QueryRow(ctx, `UPDATE nodes SET key=$1,parent_id=$2::uuid,position=$3::numeric,
	 updated_at=greatest(clock_timestamp(),date_trunc('second',updated_at)+interval '1 second')
	 WHERE id=$4::uuid RETURNING `+nodeReturning, before.Key, before.ParentID, before.Position, before.ID))
	if err != nil {
		return nodeJSON{}, events.ErrConflict
	}
	if before.Key != after.Key {
		if _, err := tx.Exec(ctx, `INSERT INTO node_key_aliases(tenant_id,key,node_id) VALUES($1::uuid,$2,$3::uuid)`, p.TenantID, after.Key, after.ID); err != nil {
			return nodeJSON{}, events.ErrConflict
		}
	}
	return restored, nil
}
