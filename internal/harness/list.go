// SPDX-License-Identifier: AGPL-3.0-only

package harness

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/inspr-at/aeon/internal/workorders"
	"github.com/jackc/pgx/v5"
)

type NodeSummary struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
}

type SessionSummary struct {
	Session
	Project NodeSummary  `json:"project"`
	Ticket  *NodeSummary `json:"ticket"`
}

type sessionPage struct {
	Items      []SessionSummary `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}

type sessionCursor struct {
	Fingerprint string    `json:"query"`
	At          time.Time `json:"at"`
	ID          string    `json:"id"`
}

// listAll is mounted by New; Plugin remains the coordinator's manifest entry.
// Paging is exclusive on the immutable (created_at,id) pair, including ties.
func (m *Module) listAll(r *http.Request, tx pgx.Tx, p tenant.Principal) (any, error) {
	q := r.URL.Query()
	limit, err := workorders.Limit(r)
	if err != nil {
		return nil, err
	}
	state, harness := q.Get("state"), q.Get("harness")
	if state != "" && state != "stopped" && !validPhase(state) {
		return nil, workorders.Fail(400, "invalid state")
	}
	if harness != "" && !validHarness(harness) {
		return nil, workorders.Fail(400, "invalid harness")
	}
	agent, projectID, ticket := q.Get("agent"), q.Get("project"), q.Get("ticket")
	for _, id := range []string{agent, projectID, ticket} {
		if id != "" && !workorders.UUID(id) {
			return nil, workorders.Fail(400, "invalid filter id")
		}
	}
	filters, _ := json.Marshal([]string{p.TenantID, p.ID, state, harness, agent, projectID, ticket})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(filters))
	var cursor sessionCursor
	if raw := q.Get("cursor"); raw != "" {
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 2048 || e != nil || json.Unmarshal(b, &cursor) != nil || cursor.Fingerprint != fingerprint || cursor.At.IsZero() || !workorders.UUID(cursor.ID) {
			return nil, workorders.Fail(400, "invalid cursor")
		}
	}
	rows, err := tx.Query(r.Context(), `SELECT `+sessionColumns+` FROM harness_sessions
 WHERE ($1='' OR CASE WHEN stopped_at IS NOT NULL THEN 'stopped' ELSE phase END=$1)
 AND ($2='' OR harness=$2) AND ($3::uuid IS NULL OR agent_principal_id=$3)
 AND ($4::uuid IS NULL OR project_id=$4) AND ($5::uuid IS NULL OR ticket_node_id=$5)
 AND ($6::timestamptz IS NULL OR (created_at,id)<($6,$7::uuid))
 ORDER BY created_at DESC,id DESC LIMIT $8`, state, harness, nullable(agent), nullable(projectID), nullable(ticket), cursorTime(cursor), nullable(cursor.ID), limit+1)
	if err != nil {
		return nil, err
	}
	out := sessionPage{Items: []SessionSummary{}}
	for rows.Next() {
		s, e := scanSession(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		out.Items = append(out.Items, SessionSummary{Session: s})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		b, _ := json.Marshal(sessionCursor{fingerprint, last.CreatedAt, last.ID})
		next := base64.RawURLEncoding.EncodeToString(b)
		out.NextCursor = &next
	}
	// Fetch the bounded set of node summaries in one query after closing rows.
	ids := []string{}
	for _, s := range out.Items {
		ids = append(ids, s.ProjectID)
		if s.TicketNodeID != nil {
			ids = append(ids, *s.TicketNodeID)
		}
	}
	nodes, err := tx.Query(r.Context(), `SELECT id::text,key,title FROM nodes WHERE id=ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer nodes.Close()
	summaries := map[string]NodeSummary{}
	for nodes.Next() {
		var n NodeSummary
		if err = nodes.Scan(&n.ID, &n.Key, &n.Title); err != nil {
			return nil, err
		}
		summaries[n.ID] = n
	}
	if err = nodes.Err(); err != nil {
		return nil, err
	}
	for i := range out.Items {
		s := &out.Items[i]
		s.Project = summaries[s.ProjectID]
		if s.TicketNodeID != nil {
			n := summaries[*s.TicketNodeID]
			s.Ticket = &n
		}
	}
	return out, nil
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func cursorTime(c sessionCursor) any {
	if c.At.IsZero() {
		return nil
	}
	return c.At
}
