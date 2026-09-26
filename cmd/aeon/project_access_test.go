// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/config"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/jackc/pgx/v5"
)

type accessWorld struct {
	t       *testing.T
	d       *dbtest.DB
	base    string
	tid     string
	ids     map[string]string // fixture name -> node, attachment or role id
	clients map[string]*http.Client
	tokens  map[string]string
	people  map[string]string // name -> principal id
}

// ADR-003 P2 end to end: the production server, its real middleware and
// modules, running as the NOSUPERUSER NOBYPASSRLS app role. A guest bound to
// project A must not see anything of project B through any API, search, SSE
// or export, and project access changes take effect at once.
func TestProjectAccessOverHTTP(t *testing.T) {
	t.Setenv("AEON_ENV", "dev")
	w := &accessWorld{t: t, d: dbtest.Open(t), ids: map[string]string{}, clients: map[string]*http.Client{}, tokens: map[string]string{}, people: map[string]string{}}
	ctx := context.Background()
	// Production migrates as the table-owning app role as well.
	for _, stmt := range []string{`GRANT CREATE ON SCHEMA public TO ` + pgx.Identifier{w.d.Role}.Sanitize(), `ALTER TABLE schema_migrations OWNER TO ` + pgx.Identifier{w.d.Role}.Sanitize()} {
		if _, err := w.d.Admin.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DatabaseURL: w.d.AppURL, Env: "dev", PublicURL: "http://127.0.0.1", BootstrapTenantSlug: "p2-http", BootstrapTenantName: "P2 HTTP", FilesDir: t.TempDir()}
	serveCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- serveListener(serveCtx, cfg, ln) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Error("shutdown timed out")
		}
	})
	w.base = "http://" + ln.Addr().String()
	deadline := time.Now().Add(15 * time.Second)
	for {
		resp, err := http.Get(w.base + "/api/health")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	w.seed()

	guest := "guest"
	// The guest loads the app: its profile and its effective access.
	w.expect(guest, "GET", "/api/me", "", 200)
	var perms struct {
		Workspace struct{ Permissions []string } `json:"workspace"`
		Project   *struct {
			Role        struct{ Key string } `json:"role"`
			Permissions []string             `json:"permissions"`
		} `json:"project"`
	}
	w.decode(w.expect(guest, "GET", "/api/me/permissions?project_id="+w.ids["A"], "", 200), &perms)
	if len(perms.Workspace.Permissions) != 0 || perms.Project == nil || perms.Project.Role.Key != "guest" || !contains(perms.Project.Permissions, "comments.write") || contains(perms.Project.Permissions, "nodes.write") {
		t.Fatalf("guest permissions %+v", perms)
	}

	// Nothing of project B, and nothing workspace-wide, through any read.
	for _, path := range []string{
		"/api/projects", "/api/nodes", "/api/nodes?limit=200", "/api/nodes/tree",
		"/api/nodes/lookup?ids=" + w.ids["TA"] + "," + w.ids["TB"] + "," + w.ids["GB"],
		"/api/search?q=zebra", "/api/events", "/api/events?limit=200", "/api/knowledge",
		"/api/knowledge/graph?project_id=" + w.ids["A"],
		"/api/relations?node_id=" + w.ids["TA"], "/api/nodes/" + w.ids["TA"], "/api/nodes/" + w.ids["TA"] + "/activity",
		"/api/nodes/" + w.ids["TA"] + "/attachments", "/api/views", "/api/kinds", "/api/me", "/api/me/profile",
	} {
		body := w.expect(guest, "GET", path, "", 200)
		w.noLeak(guest, path, body)
	}
	for _, want := range []struct{ path, needle string }{
		{"/api/projects", w.ids["A"]}, {"/api/nodes", "TA-1"}, {"/api/search?q=zebra", "TA-1"},
		{"/api/knowledge", "GA-1"}, {"/api/nodes/" + w.ids["TA"] + "/activity", "guest-visible comment"},
		{"/api/relations?node_id=" + w.ids["TA"], w.ids["GA"]},
	} {
		if body := w.expect(guest, "GET", want.path, "", 200); !strings.Contains(body, want.needle) {
			t.Errorf("guest %s: missing its own project's %s", want.path, want.needle)
		}
	}
	// Project B's resources by id, key and path are closed.
	for _, path := range []string{
		"/api/nodes/" + w.ids["TB"], "/api/node-keys/TB-1", "/api/nodes/" + w.ids["TB"] + "/activity",
		"/api/nodes/" + w.ids["TB"] + "/attachments", "/api/attachments/" + w.ids["attB"] + "/content",
		"/api/knowledge/" + w.ids["GB"], "/api/projects/" + w.ids["B"] + "/journey",
		"/api/projects/" + w.ids["B"] + "/requirements", "/api/projects/" + w.ids["B"] + "/intake",
		"/api/relations?node_id=" + w.ids["TB"], "/api/events?node_id=" + w.ids["TB"],
		"/api/knowledge/resolve?project_id=" + w.ids["B"] + "&type=guideline&slug=gb",
		"/api/projects/" + w.ids["B"] + "/members", "/api/knowledge/graph?project_id=" + w.ids["B"],
	} {
		body, status := w.call(guest, "GET", path, "")
		if status == 200 {
			w.noLeak(guest, path, body)
		} else if status != 403 && status != 404 && status != 400 {
			t.Errorf("guest %s: %d %s", path, status, body)
		}
	}
	// Workspace-wide areas stay closed to a project-only principal.
	for _, path := range []string{"/api/members", "/api/quotes", "/api/crm/organisations", "/api/time-entries",
		"/api/work-orders", "/api/harness-sessions", "/api/approvals", "/api/business/principals", "/api/roles",
		"/api/agent-keys", "/api/project-groups", "/api/releases", "/api/inbox/messages", "/api/runs", "/api/imports"} {
		w.expect(guest, "GET", path, "", 403)
	}
	// Guest writes: comment in A only; no edits anywhere.
	w.expect(guest, "POST", "/api/nodes/"+w.ids["TA"]+"/comments", `{"body_markdown":"from the guest"}`, 201)
	w.expect(guest, "POST", "/api/nodes/"+w.ids["TB"]+"/comments", `{"body_markdown":"sneaky"}`, 403)
	w.expect(guest, "PATCH", "/api/nodes/"+w.ids["TA"], `{"title":"renamed"}`, 403)
	w.expect(guest, "POST", "/api/nodes", `{"kind_id":"`+w.ids["ticketKind"]+`","parent_id":"`+w.ids["A"]+`","title":"new"}`, 403)
	// Live updates: the SSE replay carries A's events and none of B's.
	stream := w.stream(guest)
	if !strings.Contains(stream, w.ids["TA"]) {
		t.Errorf("guest stream lacks project A events")
	}
	w.noLeak(guest, "/api/events/stream", stream)

	// Workspace roles see everything; a customer sees no project.
	for _, who := range []string{"owner", "member", "viewer"} {
		for _, needle := range []string{w.ids["A"], w.ids["B"]} {
			if body := w.expect(who, "GET", "/api/projects", "", 200); !strings.Contains(body, needle) {
				t.Errorf("%s misses a project", who)
			}
		}
		if body := w.expect(who, "GET", "/api/search?q=zebra", "", 200); !strings.Contains(body, "TB-1") || !strings.Contains(body, "TA-1") {
			t.Errorf("%s search misses a project", who)
		}
	}
	w.expect("customer", "GET", "/api/projects", "", 403)
	w.expect("customer", "GET", "/api/nodes/"+w.ids["TA"], "", 403)
	w.expect("customer", "GET", "/api/me", "", 200)

	// Agents: a workspace agent sees both projects, a project-scoped agent and
	// a key minted by the guest see project A only.
	if body := w.expect("agent-ws", "GET", "/api/search?q=zebra", "", 200); !strings.Contains(body, "TB-1") {
		t.Errorf("workspace agent misses project B")
	}
	for _, agent := range []string{"agent-a", "agent-by-guest"} {
		for _, path := range []string{"/api/nodes", "/api/search?q=zebra", "/api/projects"} {
			body := w.expect(agent, "GET", path, "", 200)
			w.noLeak(agent, path, body)
			if !strings.Contains(body, w.ids["A"]) && !strings.Contains(body, "TA-1") {
				t.Errorf("%s %s misses project A", agent, path)
			}
		}
		w.expect(agent, "GET", "/api/nodes/"+w.ids["TB"], "", 403)
	}

	// A member of A who is only a guest on B cannot carry work into B.
	w.expect("mixed", "PATCH", "/api/nodes/"+w.ids["TA2"], `{"title":"member edit"}`, 200)
	w.expect("mixed", "PATCH", "/api/nodes/"+w.ids["TB"], `{"title":"guest edit"}`, 403)
	w.expect("mixed", "POST", "/api/nodes/"+w.ids["TA2"]+"/project-move", `{"project_id":"`+w.ids["B"]+`"}`, 403)
	w.expect("mixed", "POST", "/api/nodes/"+w.ids["TA2"]+"/move", `{"parent_id":"`+w.ids["B"]+`"}`, 403)
	w.expect("mixed", "POST", "/api/nodes/"+w.ids["TB"]+"/comments", `{"body_markdown":"as guest"}`, 201)

	// Project members: one row per person, the reason and the role.
	var members []struct {
		PrincipalID string               `json:"principal_id"`
		Via         string               `json:"via"`
		Role        struct{ Key string } `json:"role"`
		HasAvatar   *bool                `json:"has_avatar"`
	}
	w.decode(w.expect("owner", "GET", "/api/projects/"+w.ids["A"]+"/members", "", 200), &members)
	seen := map[string]string{}
	for _, m := range members {
		seen[m.PrincipalID] = m.Via + ":" + m.Role.Key
		if m.HasAvatar == nil {
			t.Errorf("member row lacks has_avatar")
		}
	}
	if seen[w.people["guest"]] != "project:guest" || seen[w.people["owner"]] != "workspace:owner" || seen[w.people["mixed"]] != "project:member" {
		t.Errorf("project members %v", seen)
	}
	if _, ok := seen[w.people["customer"]]; ok {
		t.Errorf("customer listed as a project member")
	}
	w.expect("member", "PUT", "/api/projects/"+w.ids["B"]+"/members/"+w.people["guest"], `{"role_id":"`+w.ids["role:guest"]+`"}`, 403)
	w.expect("owner", "PUT", "/api/projects/"+w.ids["B"]+"/members/"+w.people["guest"], `{"role_id":"`+w.ids["role:owner"]+`"}`, 400)
	w.expect("owner", "PUT", "/api/members/"+w.people["customer"]+"/workspace-role", `{"role_id":"`+w.ids["role:guest"]+`"}`, 400)
	w.expect("owner", "DELETE", "/api/projects/"+w.ids["B"]+"/members/"+w.people["member"], "", 409)
	// Granting B opens it at once; removing it closes it at once.
	w.expect("owner", "PUT", "/api/projects/"+w.ids["B"]+"/members/"+w.people["guest"], `{"role_id":"`+w.ids["role:guest"]+`"}`, 200)
	if body := w.expect(guest, "GET", "/api/projects", "", 200); !strings.Contains(body, w.ids["B"]) {
		t.Errorf("granted project B is not visible")
	}
	w.expect(guest, "GET", "/api/nodes/"+w.ids["TB"], "", 200)
	w.expect("owner", "DELETE", "/api/projects/"+w.ids["B"]+"/members/"+w.people["guest"], "", 204)
	w.expect("owner", "DELETE", "/api/projects/"+w.ids["B"]+"/members/"+w.people["guest"], "", 404)
	body := w.expect(guest, "GET", "/api/projects", "", 200)
	w.noLeak(guest, "/api/projects after removal", body)
	w.expect(guest, "GET", "/api/nodes/"+w.ids["TB"], "", 403)
	// Each change is an event on the project, visible with the project only.
	var changes int
	if err := w.d.Admin.QueryRow(ctx, `SELECT count(*) FROM events WHERE node_id=$1 AND type IN ('authz.binding_set','authz.binding_removed')`, w.ids["B"]).Scan(&changes); err != nil {
		t.Fatal(err)
	}
	if changes != 2 {
		t.Errorf("binding events on project B: %d", changes)
	}
	w.noLeak(guest, "/api/events after removal", w.expect(guest, "GET", "/api/events?limit=200", "", 200))
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func (w *accessWorld) seed() {
	t := w.t
	ctx := dbtest.Seed(context.Background())
	if err := w.d.Admin.QueryRow(ctx, `SELECT id::text FROM tenants WHERE slug='p2-http'`).Scan(&w.tid); err != nil {
		t.Fatal(err)
	}
	err := db.InTenant(ctx, w.d.App, w.tid, func(tx pgx.Tx) error {
		var writer string
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1,'person','Writer') RETURNING id::text`, w.tid).Scan(&writer); err != nil {
			return err
		}
		node := func(name, kind, key, parent string) {
			var parentID any
			if parent != "" {
				parentID = w.ids[parent]
			}
			fields := `{}`
			if kind == "guideline" {
				fields = `{"slug":"` + strings.ToLower(key[:2]) + `"}`
			}
			var nodeID string
			if err := tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,kind_id,key,title,body,fields,parent_id) SELECT $1,id,$3,'zebra '||$3,'zebra body',$5::jsonb,$4 FROM node_kinds WHERE tenant_id=$1 AND slug=$2 RETURNING id::text`, w.tid, kind, key, parentID, fields).Scan(&nodeID); err != nil {
				t.Fatalf("node %s: %v", key, err)
			}
			w.ids[name] = nodeID
		}
		node("A", "project", "PA-1", "")
		node("B", "project", "PB-1", "")
		node("TA", "ticket", "TA-1", "A")
		node("TA2", "ticket", "TA-2", "A")
		node("TB", "ticket", "TB-1", "B")
		node("GA", "guideline", "GA-1", "A")
		node("GB", "guideline", "GB-1", "B")
		var ticketKind string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM node_kinds WHERE tenant_id=$1 AND slug='ticket'`, w.tid).Scan(&ticketKind); err != nil {
			return err
		}
		w.ids["ticketKind"] = ticketKind
		for _, rel := range [][2]string{{"TA", "TB"}, {"TA", "GA"}, {"TB", "GB"}} {
			if _, err := tx.Exec(ctx, `INSERT INTO node_relations(tenant_id,source_node_id,target_node_id,type) VALUES($1,$2,$3,'cites')`, w.tid, w.ids[rel[0]], w.ids[rel[1]]); err != nil {
				return err
			}
		}
		for _, c := range [][2]string{{"TA", "guest-visible comment"}, {"TB", "secret comment of B"}} {
			if _, err := tx.Exec(ctx, `INSERT INTO events(tenant_id,actor_principal_id,node_id,type,after) VALUES($1,$2,$3,'comment.created',jsonb_build_object('body_markdown',$4::text))`, w.tid, writer, w.ids[c[0]], c[1]); err != nil {
				return err
			}
		}
		for _, a := range [][2]string{{"TA", "attA"}, {"TB", "attB"}} {
			var attachment string
			if err := tx.QueryRow(ctx, `INSERT INTO attachments(tenant_id,node_id,sha256,name,content_type,size,created_by) VALUES($1,$2,repeat('a',64),$3||'.txt','text/plain',1,$4) RETURNING id::text`, w.tid, w.ids[a[0]], a[1], writer).Scan(&attachment); err != nil {
				return err
			}
			w.ids[a[1]] = attachment
		}
		for _, p := range []string{"A", "B"} {
			if _, err := tx.Exec(ctx, `INSERT INTO journey_projects(tenant_id,project_node_id) VALUES($1,$2)`, w.tid, w.ids[p]); err != nil {
				return err
			}
		}
		// Workspace activity by someone else, never shown to a project guest.
		_, err := tx.Exec(ctx, `INSERT INTO events(tenant_id,actor_principal_id,type,after) VALUES($1,$2,'profile.updated','{"secret":"workspace activity"}')`, w.tid, writer)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := w.d.Admin.Query(ctx, `SELECT key,id::text FROM roles WHERE tenant_id=$1 AND builtin`, w.tid)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var key, roleID string
		if err := rows.Scan(&key, &roleID); err != nil {
			t.Fatal(err)
		}
		w.ids["role:"+key] = roleID
	}
	rows.Close()
	person := func(name string) string {
		var identity, pid string
		email := name + "@p2.example.test"
		if err := w.d.Admin.QueryRow(ctx, `INSERT INTO identities(issuer,subject,email) VALUES('p2-test',$1,$2) RETURNING id::text`, name, email).Scan(&identity); err != nil {
			t.Fatal(err)
		}
		if err := w.d.Admin.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,identity_id) VALUES($1,'person',$2,$3) RETURNING id::text`, w.tid, name, identity).Scan(&pid); err != nil {
			t.Fatal(err)
		}
		w.people[name] = pid
		return pid
	}
	bindProject := func(pid, role, project string) {
		if _, err := w.d.Admin.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type,scope_id) VALUES($1,$2,$3,'project',$4)`, w.tid, pid, w.ids["role:"+role], w.ids[project]); err != nil {
			t.Fatal(err)
		}
	}
	for _, role := range []string{"owner", "member", "viewer", "customer"} {
		dbtest.BindRole(t, w.d, w.tid, person(role), role)
	}
	bindProject(person("guest"), "guest", "A")
	mixed := person("mixed")
	bindProject(mixed, "member", "A")
	bindProject(mixed, "guest", "B")
	for _, name := range []string{"owner", "member", "viewer", "customer", "guest", "mixed"} {
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar, Timeout: 20 * time.Second}
		resp, err := client.Post(w.base+"/api/auth/dev-login", "application/json", strings.NewReader(`{"email":"`+name+`@p2.example.test","tenant":"p2-http"}`))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("dev login %s: %d %s", name, resp.StatusCode, body)
		}
		w.clients[name] = client
	}
	// Agents, with the migrated custom-role shape: scopes narrow what a key
	// may call; bindings (and the key creator) decide what it can see.
	var agentRole string
	if err := w.d.Admin.QueryRow(ctx, `INSERT INTO roles(tenant_id,key,name) VALUES($1,'p2_agent','P2 agent') RETURNING id::text`, w.tid).Scan(&agentRole); err != nil {
		t.Fatal(err)
	}
	if _, err := w.d.Admin.Exec(ctx, `INSERT INTO role_permissions(tenant_id,role_id,permission) SELECT $1,$2,unnest(ARRAY['nodes.read','search.read','events.read'])`, w.tid, agentRole); err != nil {
		t.Fatal(err)
	}
	agent := func(name, creator string) string {
		var pid string
		if err := w.d.Admin.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1,'agent',$2) RETURNING id::text`, w.tid, name).Scan(&pid); err != nil {
			t.Fatal(err)
		}
		nameSum := sha256.Sum256([]byte(name))
		prefix := strings.ReplaceAll(w.tid, "-", "") + hex.EncodeToString(nameSum[:])[:16]
		secret := "secret-" + name
		sum := sha256.Sum256([]byte(secret))
		var creatorID any
		if creator != "" {
			creatorID = w.people[creator]
		}
		if _, err := w.d.Admin.Exec(ctx, `INSERT INTO agent_keys(tenant_id,principal_id,name,prefix,hash,scopes,created_by_principal_id) VALUES($1,$2,'p2',$3,$4,ARRAY['nodes.read','search.read','events.read'],$5)`, w.tid, pid, prefix, hex.EncodeToString(sum[:]), creatorID); err != nil {
			t.Fatal(err)
		}
		w.tokens[name] = "aeon_" + prefix + "_" + secret
		w.clients[name] = &http.Client{Timeout: 20 * time.Second}
		return pid
	}
	for _, name := range []string{"agent-ws", "agent-by-guest"} {
		creator := ""
		if name == "agent-by-guest" {
			creator = "guest"
		}
		pid := agent(name, creator)
		if _, err := w.d.Admin.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) VALUES($1,$2,$3,'workspace')`, w.tid, pid, agentRole); err != nil {
			t.Fatal(err)
		}
	}
	bindProject(agent("agent-a", ""), "member", "A")
}

func (w *accessWorld) call(who, method, path, body string) (string, int) {
	req, err := http.NewRequest(method, w.base+path, strings.NewReader(body))
	if err != nil {
		w.t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token := w.tokens[who]; token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := w.clients[who].Do(req)
	if err != nil {
		w.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return string(raw), resp.StatusCode
}

func (w *accessWorld) expect(who, method, path, body string, status int) string {
	w.t.Helper()
	got, code := w.call(who, method, path, body)
	if code != status {
		w.t.Errorf("%s %s %s: %d, want %d: %.300s", who, method, path, code, status, got)
	}
	return got
}

func (w *accessWorld) decode(body string, v any) {
	w.t.Helper()
	if err := json.Unmarshal([]byte(body), v); err != nil {
		w.t.Fatalf("decode %.200s: %v", body, err)
	}
}

// noLeak fails when a response names anything of project B or someone
// else's workspace activity.
func (w *accessWorld) noLeak(who, path, body string) {
	w.t.Helper()
	for _, secret := range []string{w.ids["B"], w.ids["TB"], w.ids["GB"], w.ids["attB"], "PB-1", "TB-1", "GB-1", "secret comment of B", "workspace activity"} {
		if secret != "" && strings.Contains(body, secret) {
			w.t.Errorf("%s %s leaks %q", who, path, secret)
		}
	}
}

// stream reads the SSE replay for a moment and returns what arrived.
func (w *accessWorld) stream(who string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", w.base+"/api/events/stream", nil)
	if err != nil {
		w.t.Fatal(err)
	}
	resp, err := w.clients[who].Do(req)
	if err != nil {
		w.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		w.t.Fatalf("stream %s: %d", who, resp.StatusCode)
	}
	var out bytes.Buffer
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		out.WriteString(scanner.Text())
		out.WriteByte('\n')
	}
	return out.String()
}
