// SPDX-License-Identifier: AGPL-3.0-only

package activity

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/principallink"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

type commentSnapshot struct {
	CommentID string `json:"comment_id,omitempty"`
	Body      string `json:"body_markdown"`
	Deleted   bool   `json:"deleted,omitempty"`
}

func (m *module) writeComment(ctx context.Context, p tenant.Principal, node string, id int64, body string, remove bool) (Item, error) {
	var item Item
	err := db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		// Serialize lifecycle operations before reading their append-only state.
		if id != 0 {
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 59))`, p.TenantID+":"+strconv.FormatInt(id, 10)); err != nil {
				return err
			}
		}
		var found string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM nodes WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL FOR SHARE`, p.TenantID, node).Scan(&found); err != nil {
			return err
		}
		canonical, name, err := principallink.Resolve(ctx, tx, p.TenantID, p.ID)
		if err != nil {
			return err
		}
		p.ID = canonical
		avatar, err := hasAvatar(ctx, tx, p.TenantID, canonical)
		if err != nil {
			return err
		}
		change := events.Change{NodeID: &node, Type: "comment.created", After: commentSnapshot{Body: body}}
		var at time.Time
		if id != 0 {
			var author string
			var raw json.RawMessage
			if err := tx.QueryRow(ctx, `SELECT coalesce(person.linked_to,person.id)::text,e.at,e.after FROM events e JOIN principals person ON person.tenant_id=e.tenant_id AND person.id=e.actor_principal_id WHERE e.tenant_id=$1 AND e.node_id=$2 AND e.id=$3 AND e.type='comment.created'`, p.TenantID, node, id).Scan(&author, &at, &raw); err != nil {
				return err
			}
			if author != p.ID {
				return errForbidden
			}
			var current commentSnapshot
			if err := json.Unmarshal(raw, &current); err != nil {
				return err
			}
			var latest json.RawMessage
			err := tx.QueryRow(ctx, `SELECT after FROM events WHERE tenant_id=$1 AND node_id=$2 AND type IN ('comment.updated','comment.deleted') AND after->>'comment_id'=$3 ORDER BY id DESC LIMIT 1`, p.TenantID, node, strconv.FormatInt(id, 10)).Scan(&latest)
			if err != nil && err != pgx.ErrNoRows {
				return err
			}
			if err == nil {
				if err := json.Unmarshal(latest, &current); err != nil {
					return err
				}
			}
			if current.Deleted {
				return pgx.ErrNoRows
			}
			// Check the database clock after waiting for locks. Edits never extend
			// the original creation window; even an admin cannot override it.
			var allowed bool
			if err := tx.QueryRow(ctx, `SELECT clock_timestamp() >= $1::timestamptz AND clock_timestamp() < $1::timestamptz + interval '15 minutes'`, at).Scan(&allowed); err != nil {
				return err
			}
			if !allowed {
				return errForbidden
			}
			current.CommentID = strconv.FormatInt(id, 10)
			change.Before = current
			change.Type = "comment.updated"
			next := commentSnapshot{CommentID: current.CommentID, Body: body, Deleted: remove}
			if remove {
				change.Type = "comment.deleted"
				next.Body = current.Body
			}
			change.After = next
		}
		e, err := events.Append(ctx, tx, p, change)
		if err != nil {
			return err
		}
		if id == 0 {
			id, at = e.ID, e.At
		}
		item = Item{ID: strconv.FormatInt(id, 10), At: at, Type: "comment", Author: Author{ID: &p.ID, Name: name, HasAvatar: avatar}, BodyMarkdown: &body}
		return nil
	})
	return item, err
}
