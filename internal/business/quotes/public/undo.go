// SPDX-License-Identifier: AGPL-3.0-only

package public

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// UndoHandlers lets the coordinator register the reversible link creation
// with events.WithUndoHandlers. Revocation cannot restore a bearer token; a
// fresh link must be issued instead. Acceptance and receipts are immutable.
func UndoHandlers() map[string]events.UndoFunc {
	return map[string]events.UndoFunc{"quote.public_link_created": undoCreatedLink}
}
func undoCreatedLink(ctx context.Context, tx pgx.Tx, p tenant.Principal, original events.Event) (events.Change, error) {
	var after struct {
		LinkID              string `json:"link_id"`
		Version             int    `json:"version"`
		TargetContentSHA256 string `json:"target_content_sha256"`
	}
	if json.Unmarshal(original.After, &after) != nil || !uuidPattern.MatchString(after.LinkID) || !digestPattern.MatchString(after.TargetContentSHA256) || original.NodeID == nil {
		return events.Change{}, events.ErrConflict
	}
	var quoteID, digest string
	var version int
	var revoked bool
	var issuedEvent int64
	if err := tx.QueryRow(ctx, `SELECT 1 FROM business_quotes WHERE quote_node_id=$1::uuid FOR UPDATE`, *original.NodeID).Scan(new(int)); err != nil {
		return events.Change{}, err
	}
	err := tx.QueryRow(ctx, `SELECT quote_node_id::text,version,target_content_sha256,revoked_at IS NOT NULL,issued_event_id FROM quote_public_links WHERE id=$1::uuid FOR UPDATE`, after.LinkID).Scan(&quoteID, &version, &digest, &revoked, &issuedEvent)
	if errors.Is(err, pgx.ErrNoRows) {
		return events.Change{}, events.ErrNotFound
	}
	if err != nil {
		return events.Change{}, err
	}
	if quoteID != *original.NodeID || version != after.Version || digest != after.TargetContentSHA256 || issuedEvent != original.ID || revoked {
		return events.Change{}, events.ErrConflict
	}
	event, err := events.Append(ctx, tx, p, events.Change{NodeID: &quoteID, Type: "quote.public_link_revoked", After: map[string]any{"version": version, "link_id": after.LinkID}})
	if err != nil {
		return events.Change{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE quote_public_links SET revoked_at=clock_timestamp(),revoked_by_principal_id=$2::uuid,revoked_event_id=$3 WHERE id=$1::uuid`, after.LinkID, p.ID, event.ID); err != nil {
		return events.Change{}, err
	}
	return events.Change{NodeID: &quoteID, Type: "quote.public_link_undone", Before: map[string]any{"link_id": after.LinkID, "version": version}, After: map[string]any{"link_id": after.LinkID, "version": version, "revoked": true}}, nil
}
