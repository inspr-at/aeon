// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/jackc/pgx/v5"
)

// remove deletes a draft that was never issued (QL1/AEON-109). The quote node
// and its projection are hidden, not erased: the commercial number stays
// allocated and "quote.deleted" is reversible through the event log (see
// UndoHandlers). Anything that was ever issued is immutable evidence and can
// only be archived. The migration trigger enforces the same guard.
func (m *Module) remove(w http.ResponseWriter, r *http.Request) {
	p, e := caller(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if !admin(p) {
		respond(w, 0, nil, denied())
		return
	}
	id, e := pathID(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	expected, err := strconv.ParseInt(r.URL.Query().Get("expected_revision"), 10, 64)
	if err != nil || expected < 1 {
		respond(w, 0, nil, bad("expected_revision is required"))
		return
	}
	e = m.tx(r.Context(), p, fence.PermNodesContribute, true, func(tx pgx.Tx) error {
		q, err := readQuote(r.Context(), tx, id, true)
		if err != nil {
			return err
		}
		if q.Revision != expected {
			return conflict("quote revision is stale")
		}
		if err := deletable(r.Context(), tx, q); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE business_quotes SET deleted_at=clock_timestamp(),deleted_by_principal_id=$2::uuid,revision=revision+1 WHERE quote_node_id=$1::uuid`, id, p.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE nodes SET deleted_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1::uuid`, id); err != nil {
			return err
		}
		after := q
		after.Revision++
		return appendEvent(r.Context(), tx, p, id, "quote.deleted", q, map[string]any{"deleted": true, "revision": after.Revision})
	})
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deletable is the guard the UI mirrors: a draft with no version ever frozen.
func deletable(ctx context.Context, tx pgx.Tx, q quote) error {
	if q.State != "draft" || q.CurrentVersion != 0 {
		return conflict("only a draft that was never issued can be deleted; archive it instead")
	}
	var frozen bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quote_versions WHERE quote_node_id=$1::uuid)`, q.QuoteNodeID).Scan(&frozen); err != nil {
		return err
	}
	if frozen {
		return conflict("only a draft that was never issued can be deleted; archive it instead")
	}
	return nil
}

// readDeleted reads a soft-deleted quote for its undo.
func readDeleted(ctx context.Context, tx pgx.Tx, id string) (quote, error) {
	var out quote
	err := tx.QueryRow(ctx, `SELECT quote_node_id::text,coalesce(project_node_id::text,''),customer_org_node_id::text,current_version,state,revision,coalesce(offer_no,''),archived_at IS NOT NULL,project_ref FROM business_quotes WHERE quote_node_id=$1::uuid AND deleted_at IS NOT NULL FOR UPDATE`, id).
		Scan(&out.QuoteNodeID, &out.ProjectNodeID, &out.CustomerOrgNodeID, &out.CurrentVersion, &out.State, &out.Revision, &out.OfferNo, &out.Archived, &out.ProjectRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, missing()
	}
	return out, err
}
