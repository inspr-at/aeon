// SPDX-License-Identifier: AGPL-3.0-only

package harness

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/inspr-at/aeon/internal/authz"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// LiveWindow is how fresh a heartbeat must be for a session to count as live:
// the same two minutes the orchestrator resolution and the web app's
// HEARTBEAT_STALE_MS use.
const LiveWindow = 2 * time.Minute

// maxLive bounds one answer; a workspace has a handful of live sessions.
const maxLive = 500

// LiveAgent is one agent actively working in a project right now (AEON-184):
// a session that is not stopped, heartbeated within LiveWindow, is starting,
// working or stopping, and has not reported itself idle. The project is the
// session's own project, or the project its bound ticket belongs to now.
//
// Who it is (principal_id and name) is present only when the caller holds
// members.read or harness.read at that project, like approval agent names
// (AEON-171). session_id, the key to the Agents workspace, is present only
// with harness.read in the workspace, which that workspace requires.
type LiveAgent struct {
	ProjectID   string       `json:"project_id"`
	SessionID   string       `json:"session_id,omitempty"`
	PrincipalID string       `json:"principal_id,omitempty"`
	Name        string       `json:"name,omitempty"`
	Harness     string       `json:"harness"`
	Management  string       `json:"management_mode"`
	Role        string       `json:"role"`
	Phase       string       `json:"phase"`
	Activity    string       `json:"activity"`
	Ticket      *NodeSummary `json:"ticket"`
	Since       time.Time    `json:"since"`
	HeartbeatAt time.Time    `json:"heartbeat_at"`
}

// LivePage answers GET /api/harness-sessions/live. At is the server's clock,
// so a client can show elapsed time without trusting its own.
type LivePage struct {
	Items        []LiveAgent `json:"items"`
	At           time.Time   `json:"at"`
	FreshSeconds int         `json:"fresh_seconds"`
}

type liveAccess struct{ name, session bool }

// live reads inside the caller's transaction, so tenant row-level security and
// project visibility (ADR-003 P2) decide which sessions and tickets exist for
// it: a project the caller cannot see contributes nothing, not even a count.
func (m *Module) live(r *http.Request, tx pgx.Tx, p tenant.Principal) (any, error) {
	ctx := r.Context()
	rows, err := tx.Query(ctx, `SELECT s.id::text,s.project_id::text,s.agent_principal_id::text,coalesce(a.name,''),s.harness,s.management,s.role,s.phase,s.activity,
       t.id::text,t.key,t.title,t.project_id::text,s.created_at,s.heartbeat_at,clock_timestamp()
  FROM harness_sessions s
  LEFT JOIN principals a ON a.tenant_id=s.tenant_id AND a.id=s.agent_principal_id
  LEFT JOIN nodes t ON t.tenant_id=s.tenant_id AND t.id=s.ticket_node_id AND t.deleted_at IS NULL
 WHERE s.stopped_at IS NULL AND s.heartbeat_at>clock_timestamp()-make_interval(secs=>$1)
   AND s.phase IN ('starting','working','stopping') AND s.activity<>'idle'
 ORDER BY s.created_at,s.id LIMIT $2`, LiveWindow.Seconds(), maxLive)
	if err != nil {
		return nil, err
	}
	type row struct {
		agent         LiveAgent
		ticketProject *string
	}
	found := []row{}
	now := time.Now()
	for rows.Next() {
		var v row
		var ticketID, ticketKey, ticketTitle *string
		var heartbeat *time.Time
		if err = rows.Scan(&v.agent.SessionID, &v.agent.ProjectID, &v.agent.PrincipalID, &v.agent.Name, &v.agent.Harness, &v.agent.Management, &v.agent.Role, &v.agent.Phase, &v.agent.Activity,
			&ticketID, &ticketKey, &ticketTitle, &v.ticketProject, &v.agent.Since, &heartbeat, &now); err != nil {
			rows.Close()
			return nil, err
		}
		if heartbeat != nil {
			v.agent.HeartbeatAt = *heartbeat
		}
		if ticketID != nil && ticketKey != nil && ticketTitle != nil {
			v.agent.Ticket = &NodeSummary{ID: *ticketID, Key: *ticketKey, Title: *ticketTitle}
		}
		found = append(found, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := LivePage{Items: []LiveAgent{}, At: now.UTC(), FreshSeconds: int(LiveWindow.Seconds())}
	if len(found) == 0 {
		return out, nil
	}
	workspace, err := accessIn(ctx, tx, p, authz.Scope{})
	if err != nil {
		return nil, err
	}
	perProject := map[string]liveAccess{}
	accessFor := func(projectID string) (liveAccess, error) {
		if workspace.name {
			return workspace, nil
		}
		if a, ok := perProject[projectID]; ok {
			return a, nil
		}
		a, err := accessIn(ctx, tx, p, authz.Scope{ProjectID: projectID})
		if err != nil {
			return a, err
		}
		a.session = workspace.session
		perProject[projectID] = a
		return a, nil
	}
	for _, v := range found {
		projects := []string{v.agent.ProjectID}
		// A ticket moved on to another project takes its agent along; the
		// ticket row is visible only when that project is.
		if v.ticketProject != nil && *v.ticketProject != v.agent.ProjectID {
			projects = append(projects, *v.ticketProject)
		}
		for _, projectID := range projects {
			agent := v.agent
			agent.ProjectID = projectID
			access, err := accessFor(projectID)
			if err != nil {
				return nil, err
			}
			if !access.name {
				agent.PrincipalID, agent.Name = "", ""
			}
			if !access.session {
				agent.SessionID = ""
			}
			out.Items = append(out.Items, agent)
		}
	}
	return out, nil
}

// accessIn says whether the caller may know which agent it is (members.read or
// harness.read, AEON-171) and may open the session (harness.read) in scope.
func accessIn(ctx context.Context, tx pgx.Tx, p tenant.Principal, scope authz.Scope) (liveAccess, error) {
	allowed := func(permission string) (bool, error) {
		err := authz.RequireTx(ctx, tx, p, permission, scope)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, authz.ErrForbidden) {
			return false, nil
		}
		return false, err
	}
	harness, err := allowed("harness.read")
	if err != nil {
		return liveAccess{}, err
	}
	members := harness
	if !members {
		if members, err = allowed("members.read"); err != nil {
			return liveAccess{}, err
		}
	}
	return liveAccess{name: members, session: harness}, nil
}
