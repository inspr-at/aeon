// SPDX-License-Identifier: AGPL-3.0-only

package harness_test

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/inspr-at/aeon/internal/harness"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// AEON-184: the Projects page asks once which agents are working where. Only
// fresh, unstopped, non-idle sessions count; project visibility decides what
// exists; names and session ids follow the caller's permissions.
func TestLiveAgents(t *testing.T) {
	f := fixture(t)
	ctx := t.Context()
	second := uid()
	f.tx(t, f.person, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO nodes(tenant_id,id,key,kind_id,title) SELECT $1,$2,'HTS-3',kind_id,'Second project' FROM nodes WHERE id=$3`, f.person.TenantID, second, f.project)
		return err
	})
	register := func(project string, ticket bool) string {
		t.Helper()
		body := map[string]any{"agent_principal_id": f.agent.ID, "harness": "claude", "host": "studio-mac", "management_mode": "unmanaged", "role": "worker", "harness_session_ref": "live-generation-" + uid(), "worker_lease": "live-worker-lease-" + uid()}
		if ticket {
			body["ticket_node_id"] = f.ticket
			body["work_shape"] = "ship"
		}
		w := f.call(f.person, "POST", "/api/projects/"+project+"/harness-sessions", body, "")
		expect(t, w, 201)
		return decode(t, w)["id"].(string)
	}
	// state sets what a heartbeat would; beat is how long ago it arrived (nil: never).
	state := func(id, phase, activity string, beat *string, stopped bool) {
		t.Helper()
		f.tx(t, f.person, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE harness_sessions SET phase=$2,activity=$3,heartbeat_at=CASE WHEN $4::text IS NULL THEN NULL ELSE clock_timestamp()-($4::text)::interval END,stopped_at=CASE WHEN $5 THEN clock_timestamp() END,stop_reason=CASE WHEN $5 THEN 'stopped' END WHERE id=$1`, id, phase, activity, beat, stopped)
			return err
		})
	}
	ago := func(s string) *string { return &s }
	working := register(f.project, true)
	state(working, "working", "busy", ago("20 seconds"), false)
	starting := register(second, false)
	state(starting, "starting", "unknown", ago("5 seconds"), false)
	stale := register(f.project, false)
	state(stale, "working", "busy", ago("3 minutes"), false)
	idle := register(f.project, false)
	state(idle, "working", "idle", ago("10 seconds"), false)
	yielded := register(f.project, false)
	state(yielded, "yielded", "busy", ago("10 seconds"), false)
	stopped := register(f.project, false)
	state(stopped, "stopped", "busy", ago("10 seconds"), true)
	register(f.project, false) // registered, never heartbeated

	person := func(name string) tenant.Principal {
		t.Helper()
		p := tenant.Principal{ID: uid(), TenantID: f.person.TenantID, Kind: tenant.Person}
		f.tx(t, p, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO principals(tenant_id,id,kind,name) VALUES($1,$2,'person',$3)`, p.TenantID, p.ID, name)
			return err
		})
		return p
	}
	bindProject := func(p tenant.Principal, role, project string) {
		t.Helper()
		if _, err := f.db.Admin.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type,scope_id) SELECT $1::uuid,$2::uuid,id,'project',$3::uuid FROM roles WHERE tenant_id=$1::uuid AND key=$4`, p.TenantID, p.ID, project, role); err != nil {
			t.Fatal(err)
		}
	}
	guest := person("guest on the first project")
	bindProject(guest, "guest", f.project)
	member := person("member of the second project")
	bindProject(member, "member", second)
	viewer := person("workspace viewer")
	if _, err := f.db.Admin.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) SELECT $1::uuid,$2::uuid,id,'workspace' FROM roles WHERE tenant_id=$1::uuid AND key='viewer'`, viewer.TenantID, viewer.ID); err != nil {
		t.Fatal(err)
	}
	reader := person("nodes reader")
	var roleID string
	if err := f.db.Admin.QueryRow(ctx, `INSERT INTO roles(tenant_id,key,name) VALUES($1::uuid,'nodes_reader','Nodes reader') RETURNING id::text`, reader.TenantID).Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Admin.Exec(ctx, `INSERT INTO role_permissions(tenant_id,role_id,permission) VALUES($1::uuid,$2::uuid,'nodes.read')`, reader.TenantID, roleID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Admin.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) VALUES($1::uuid,$2::uuid,$3::uuid,'workspace')`, reader.TenantID, reader.ID, roleID); err != nil {
		t.Fatal(err)
	}

	type item struct {
		harness.LiveAgent
		raw map[string]any
	}
	live := func(p tenant.Principal) []item {
		t.Helper()
		w := f.call(p, "GET", "/api/harness-sessions/live", nil, "")
		expect(t, w, 200)
		var page struct {
			Items        []json.RawMessage `json:"items"`
			At           string            `json:"at"`
			FreshSeconds int               `json:"fresh_seconds"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.At == "" || page.FreshSeconds != 120 {
			t.Fatalf("page clock %q fresh %d", page.At, page.FreshSeconds)
		}
		out := []item{}
		for _, raw := range page.Items {
			var v item
			if err := json.Unmarshal(raw, &v.LiveAgent); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &v.raw); err != nil {
				t.Fatal(err)
			}
			out = append(out, v)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ProjectID+out[i].Phase < out[j].ProjectID+out[j].Phase })
		return out
	}
	// Which (project, phase) pairs came back, and whether each names its agent.
	type seen struct {
		project, phase string
		named, linked  bool
	}
	summary := func(items []item) []seen {
		out := []seen{}
		for _, v := range items {
			_, hasName := v.raw["name"]
			_, hasPrincipal := v.raw["principal_id"]
			_, hasSession := v.raw["session_id"]
			if hasName != hasPrincipal {
				t.Fatalf("name and principal_id travel together: %v", v.raw)
			}
			out = append(out, seen{v.ProjectID, v.Phase, hasName, hasSession})
		}
		return out
	}
	want := func(name string, got []seen, expected ...seen) {
		t.Helper()
		sort.Slice(expected, func(i, j int) bool { return expected[i].project+expected[i].phase < expected[j].project+expected[j].phase })
		if len(got) != len(expected) {
			t.Fatalf("%s: got %+v want %+v", name, got, expected)
		}
		for i := range got {
			if got[i] != expected[i] {
				t.Fatalf("%s: got %+v want %+v", name, got, expected)
			}
		}
	}

	admin := live(f.person)
	want("admin", summary(admin), seen{f.project, "working", true, true}, seen{second, "starting", true, true})
	for _, v := range admin {
		if v.Phase == "working" {
			if v.SessionID != working || v.PrincipalID != f.agent.ID || v.Name != "worker" || v.Harness != "claude" || v.Management != "unmanaged" || v.Activity != "busy" {
				t.Fatalf("working agent %+v", v.LiveAgent)
			}
			if v.Ticket == nil || v.Ticket.ID != f.ticket || v.Ticket.Key != "HTS-2" || v.Ticket.Title != "Harness ticket" {
				t.Fatalf("ticket %+v", v.Ticket)
			}
			if v.Since.IsZero() || v.HeartbeatAt.IsZero() {
				t.Fatalf("times %+v", v.LiveAgent)
			}
		} else if v.SessionID != starting || v.Ticket != nil || v.raw["ticket"] != nil {
			t.Fatalf("starting agent %+v", v.raw)
		}
	}
	// A guest sees only its project, and that an agent works there, not which.
	want("guest", summary(live(guest)), seen{f.project, "working", false, false})
	if got := live(guest); got[0].Ticket == nil || got[0].Ticket.Key != "HTS-2" {
		t.Fatalf("guest ticket %+v", got[0].Ticket)
	}
	// A project member may know its agents' names, but the Agents workspace is not theirs.
	want("project member", summary(live(member)), seen{second, "starting", true, false})
	want("viewer", summary(live(viewer)), seen{f.project, "working", true, true}, seen{second, "starting", true, true})
	want("nodes reader", summary(live(reader)), seen{f.project, "working", false, false}, seen{second, "starting", false, false})
	if got := live(f.foreign); len(got) != 0 {
		t.Fatalf("tenant leak %+v", got)
	}
	// The agent's key reaches the route; the name needs a permission its scopes lack.
	want("agent", summary(live(f.agent)), seen{f.project, "working", false, false}, seen{second, "starting", false, false})

	// A ticket moved to the second project takes its agent along; a caller
	// who cannot see that project keeps the session, without the ticket.
	f.tx(t, f.person, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE nodes SET parent_id=$2 WHERE id=$1`, f.ticket, second)
		return err
	})
	want("admin after move", summary(live(f.person)), seen{f.project, "working", true, true}, seen{second, "starting", true, true}, seen{second, "working", true, true})
	moved := live(guest)
	want("guest after move", summary(moved), seen{f.project, "working", false, false})
	if moved[0].Ticket != nil {
		t.Fatalf("an invisible ticket leaked %+v", moved[0].Ticket)
	}
	// The session itself belongs to the first project, which this member cannot see.
	want("member after move", summary(live(member)), seen{second, "starting", true, false})

	expect(t, f.call(tenant.Principal{}, "GET", "/api/harness-sessions/live", nil, ""), 401)
	f.key = "invalid"
	expect(t, f.call(f.agent, "GET", "/api/harness-sessions/live", nil, ""), 403)
}
