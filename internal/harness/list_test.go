// SPDX-License-Identifier: AGPL-3.0-only

package harness_test

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/harness"
	"github.com/inspr-at/paimos/internal/tenant"
)

func TestTenantHarnessList(t *testing.T) {
	f := fixture(t)
	secondProject := uid()
	f.tx(t, f.person, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO nodes(tenant_id,id,key,kind_id,title) SELECT $1,$2,'HTS-3',kind_id,'Second project' FROM nodes WHERE id=$3`, f.person.TenantID, secondProject, f.project)
		return err
	})
	ids := []string{}
	for i, project := range []string{f.project, secondProject, f.project} {
		body := map[string]any{"agent_principal_id": f.agent.ID, "harness": "codex", "host": "test-host", "management_mode": "unmanaged", "role": "worker", "harness_session_ref": "test-generation-" + uid(), "worker_lease": "test-worker-lease-" + uid()}
		if i == 0 {
			body["ticket_node_id"] = f.ticket
			body["work_shape"] = "ship"
		}
		w := f.call(f.person, "POST", "/api/projects/"+project+"/harness-sessions", body, "")
		expect(t, w, 201)
		ids = append(ids, decode(t, w)["id"].(string))
	}
	// Equal timestamps exercise the ID tie-break; stopped overrides phase.
	f.tx(t, f.person, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE harness_sessions SET created_at='2026-01-01T00:00:00Z',stopped_at=CASE WHEN id=$1 THEN now() END,phase=CASE WHEN id=$1 THEN 'stopped' ELSE 'starting' END`, ids[2])
		return err
	})
	type page struct {
		Items []harness.SessionSummary `json:"items"`
		Next  *string                  `json:"next_cursor"`
	}
	get := func(p tenant.Principal, path string) page {
		w := f.call(p, "GET", path, nil, "")
		expect(t, w, 200)
		var out page
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	seen := map[string]bool{}
	path := "/api/harness-sessions?limit=1"
	var firstCursor string
	for {
		out := get(f.person, path)
		if len(out.Items) != 1 {
			t.Fatalf("page: %+v", out)
		}
		s := out.Items[0]
		if seen[s.ID] {
			t.Fatal("duplicate page item")
		}
		seen[s.ID] = true
		if s.Project.ID != s.ProjectID || s.Project.Key == "" || s.Project.Title == "" {
			t.Fatalf("project summary %+v", s)
		}
		if s.TicketNodeID != nil && (s.Ticket == nil || s.Ticket.ID != f.ticket || s.Ticket.Key != "HTS-2") {
			t.Fatal("ticket summary missing")
		}
		// The agent's own name, not the host it runs on.
		if s.Agent == nil || s.Agent.ID != f.agent.ID || s.Agent.Name != "worker" || s.Host != "test-host" {
			t.Fatalf("agent summary %+v", s.Agent)
		}
		if out.Next == nil {
			break
		}
		if firstCursor == "" {
			firstCursor = *out.Next
		}
		path = "/api/harness-sessions?limit=1&cursor=" + url.QueryEscape(*out.Next)
	}
	if len(seen) != 3 {
		t.Fatal(seen)
	}
	for query, want := range map[string]int{"state=starting": 2, "state=stopped": 1, "harness=codex": 3, "harness=claude": 0, "agent=" + f.agent.ID: 3, "agent=" + f.person.ID: 0, "project=" + secondProject: 1, "ticket=" + f.ticket: 1, "project=" + f.project + "&ticket=" + f.ticket: 1} {
		if got := len(get(f.person, "/api/harness-sessions?"+query).Items); got != want {
			t.Fatalf("%s: %d want %d", query, got, want)
		}
	}
	if len(get(f.foreign, "/api/harness-sessions?project="+f.project).Items) != 0 {
		t.Fatal("tenant leak")
	}
	if len(get(f.agent, "/api/harness-sessions").Items) != 3 {
		t.Fatal("scoped agent list")
	}
	for _, query := range []string{"limit=201", "limit=0", "cursor=bad", "agent=no", "project=no", "ticket=no", "harness=bad", "state=bad", "state=starting&cursor=" + firstCursor} {
		expect(t, f.call(f.person, "GET", "/api/harness-sessions?"+query, nil, ""), 400)
	}
	expect(t, f.call(f.foreign, "GET", "/api/harness-sessions?cursor="+firstCursor, nil, ""), 400)
	expect(t, f.call(tenant.Principal{}, "GET", "/api/harness-sessions", nil, ""), 401)
	f.key = "invalid"
	expect(t, f.call(f.agent, "GET", "/api/harness-sessions", nil, ""), 403)
}
