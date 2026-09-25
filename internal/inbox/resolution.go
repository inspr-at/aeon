// SPDX-License-Identifier: AGPL-3.0-only

package inbox

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/inspr-at/aeon/internal/authz"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

const resolutionEvent = "inbox.action_resolved"

type HeldResolution struct {
	MessageID string    `json:"message_id"`
	Decision  string    `json:"decision"`
	CreatedAt time.Time `json:"created_at"`
}

// resolveMessage records a human decision without mutating the held message,
// its delivery, or its reply obligation. The event is the immutable projection.
// Auth middleware establishes the person session; explicit API authorization
// and agent attribution are rejected even alongside an authenticated person.
func (m *messaging) resolveMessage(w http.ResponseWriter, r *http.Request) {
	p, project, ok := m.messagingPrincipal(w, r, true)
	if !ok {
		return
	}
	if p.Kind != tenant.Person || len(r.Header.Values("Authorization")) != 0 || r.Header.Get("X-Paimos-Agent-Name") != "" || r.Header.Get("X-Aeon-Agent-Name") != "" {
		messagingFailure(w, errForbidden)
		return
	}
	id, ok := parseUUID(r.PathValue("messageId"))
	if !ok {
		messagingFailure(w, errNotFound)
		return
	}
	var in struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if !decodeJSON(w, r, 64<<10, &in) {
		return
	}
	if (in.Decision != "resolved" && in.Decision != "dismissed") || !utf8.ValidString(in.Note) || utf8.RuneCountInString(in.Note) > 8000 || strings.ContainsRune(in.Note, 0) {
		messagingFailure(w, badRequest("invalid decision or note"))
		return
	}
	var out HeldResolution
	ctx := r.Context()
	err := db.InTenant(ctx, m.base.pool, p.TenantID, func(tx pgx.Tx) error {
		if err := messagingProject(ctx, tx, project); err != nil {
			return err
		}
		// Match the send path's lock order before appending a tenant event.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,55))`, p.TenantID); err != nil {
			return err
		}
		if err := authz.RequireTx(ctx, tx, p, "inbox.manage", authz.Scope{ProjectID: project}); err != nil {
			return errForbidden
		}
		var held bool
		if err := tx.QueryRow(ctx, `SELECT is_action_request FROM inbox_compat_messages WHERE project_id=$1::uuid AND id=$2::uuid FOR UPDATE`, project, id).Scan(&held); err != nil {
			return err
		}
		if !held {
			return &httpError{409, "conflict", "only held action requests can be resolved"}
		}
		var raw json.RawMessage
		err := tx.QueryRow(ctx, `SELECT after FROM events WHERE type=$1 AND node_id=$2::uuid AND after->>'message_id'=$3 ORDER BY id LIMIT 1`, resolutionEvent, project, id).Scan(&raw)
		var prior struct {
			HeldResolution
			NoteDigest string `json:"note_digest"`
		}
		if err == nil {
			if err = json.Unmarshal(raw, &prior); err != nil {
				return err
			}
			if prior.Decision != in.Decision || prior.NoteDigest != messageDigest(in.Note) {
				return &httpError{409, "conflict", "held action request already has a different resolution"}
			}
			out = prior.HeldResolution
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		out = HeldResolution{MessageID: id, Decision: in.Decision}
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&out.CreatedAt); err != nil {
			return err
		}
		_, err = events.Append(ctx, tx, p, events.Change{Type: resolutionEvent, NodeID: &project, After: struct {
			HeldResolution
			NoteDigest string `json:"note_digest"`
		}{out, messageDigest(in.Note)}})
		return err
	})
	if err != nil {
		messagingFailure(w, err)
		return
	}
	writeJSON(w, 200, out)
}
