// SPDX-License-Identifier: AGPL-3.0-only

package releases

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/events"
	"github.com/inspr-at/paimos/internal/tenant"
)

func replace(ctx context.Context, tx pgx.Tx, p tenant.Principal, project, release string, in planInput) (Walker, error) {
	var out Walker
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_setting('aeon.tenant_id',true),0))`); err != nil {
		return out, err
	}
	var isPerson bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM principals WHERE id=$1 AND kind='person')`, p.ID).Scan(&isPerson); err != nil {
		return out, err
	}
	if !isPerson {
		return out, fail(403, "person required")
	}
	// Serialize plan changes with project actions and requirements generation.
	var current *string
	if err := tx.QueryRow(ctx, `SELECT current_release_node_id::text FROM journey_projects WHERE project_node_id=$1 FOR UPDATE`, project).Scan(&current); err != nil {
		return out, err
	}
	var state string
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT state,revision FROM journey_releases WHERE project_node_id=$1 AND release_node_id=$2 FOR UPDATE`, project, release).Scan(&state, &revision); err != nil {
		return out, err
	}
	if state != "planning" || current == nil || *current != release {
		return out, fail(409, "only the current planning release can change")
	}
	if revision != in.Revision {
		return out, fail(409, "release revision changed")
	}
	before, err := load(ctx, tx, project, release)
	if err != nil {
		return out, err
	}
	eligible := map[string]bool{}
	for _, t := range before.Tickets {
		eligible[t.NodeID] = true
	}
	if len(in.Order) != len(eligible) {
		return out, fail(409, "order must contain every current release and backlog ticket exactly once")
	}
	ordered := map[string]bool{}
	for _, id := range in.Order {
		if !eligible[id] || ordered[id] {
			return out, fail(409, "order contains an unavailable or duplicate ticket")
		}
		ordered[id] = true
	}
	included := map[string]bool{}
	for _, id := range in.Included {
		if !eligible[id] || included[id] {
			return out, fail(409, "selection contains an unavailable or duplicate ticket")
		}
		included[id] = true
	}
	// Apply order and membership together; a validation/event failure rolls back
	// the entire plan. Positions belonging to prior releases remain untouched.
	for pos, id := range in.Order {
		var assigned *string
		if included[id] {
			assigned = &release
		}
		if _, err = tx.Exec(ctx, `UPDATE journey_tickets SET release_node_id=$2,walker_position=$3 WHERE ticket_node_id=$1`, id, assigned, pos); err != nil {
			return out, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE journey_releases SET revision=revision+1,access_required=EXISTS(SELECT 1 FROM journey_tickets t JOIN nodes n ON n.tenant_id=t.tenant_id AND n.id=t.ticket_node_id WHERE t.release_node_id=$1 AND t.access_change AND n.deleted_at IS NULL) WHERE release_node_id=$1`, release)
	if err != nil {
		return out, err
	}
	// A plan changes the input to future journey gates. Invalidate project CAS
	// along with release CAS rather than leaving an old Start Build button live.
	if _, err = tx.Exec(ctx, `UPDATE journey_projects SET revision=revision+1,updated_at=now() WHERE project_node_id=$1`, project); err != nil {
		return out, err
	}
	out, err = load(ctx, tx, project, release)
	if err != nil {
		return out, err
	}
	_, err = events.Append(ctx, tx, p, events.Change{NodeID: &release, Type: "journey.release_planned", Before: before, After: out})
	return out, err
}
