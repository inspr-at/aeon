// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"encoding/json"
	"github.com/inspr-at/aeon/internal/db"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/tenant"
)

func TestPolishedNodeList(t *testing.T) {
	p := newPrincipal(t, "polished")
	project := kindBySlug(t, p, "project")
	ticket := kindBySlug(t, p, "ticket")
	create := func(kind, key, title, state, parent string, fields map[string]any) nodeJSON {
		t.Helper()
		if fields == nil {
			fields = map[string]any{}
		}
		body := map[string]any{"kind_id": kind, "key": key, "title": title, "state": state, "fields": fields}
		if parent != "" {
			body["parent_id"] = parent
		}
		raw, _ := json.Marshal(body)
		return mustNode(t, p, string(raw))
	}
	root := create(project.ID, "PRJ-1", "Main", "active", "", nil)
	archived := create(project.ID, "PRJ-2", "Old", "archived", "", nil)
	a := create(ticket.ID, "PAI-2", "Alpha", "new", root.ID, map[string]any{"priority": "high", "assignee": p.ID})
	b := create(ticket.ID, "PAI-10", "Beta", "done", root.ID, map[string]any{"priority": "low"})
	c := create(ticket.ID, "PAI-3", "Gamma", "qa", a.ID, map[string]any{"priority": "medium"})
	_ = create(ticket.ID, "PAI-20", "Archived child", "archived", archived.ID, nil)
	get := func(path string) nodePage {
		t.Helper()
		status, body := call(t, &p, http.MethodGet, path, "")
		return decode[nodePage](t, status, body, http.StatusOK)
	}
	within := get("/api/nodes?within=" + root.ID + "&sort=key&facets=state,kind,priority,assignee")
	if len(within.Items) != 3 || within.Items[0].ID != a.ID || within.Items[1].ID != c.ID || within.Items[2].ID != b.ID {
		t.Fatalf("natural key order/within: %#v", within.Items)
	}
	if within.Facets["state"]["new"] != 1 || within.Facets["kind"]["ticket"] != 3 || within.Facets["priority"]["high"] != 1 || within.Facets["assignee"][p.ID] != 1 || within.Facets["assignee"]["none"] != 2 {
		t.Fatalf("facets: %#v", within.Facets)
	}
	row := within.Items[0]
	if row.KindSlug != "ticket" || row.KindLabel != "Ticket" || row.Priority == nil || *row.Priority != "high" || row.Assignee == nil || row.Assignee.ID != p.ID || row.Parent == nil || row.Parent.ID != root.ID || row.ChildrenCount != 1 || row.Project == nil || row.Project.ID != root.ID {
		t.Fatalf("projection: %#v", row)
	}
	if within.Items[1].Parent == nil || within.Items[1].Parent.ID != a.ID || within.Items[1].Project == nil || within.Items[1].Project.ID != root.ID {
		t.Fatalf("deep projection: %#v", within.Items[1])
	}
	if got := get("/api/nodes?kind=ticket&kind=project&state=new,qa&priority=high&priority=medium&within=" + root.ID); len(got.Items) != 2 {
		t.Fatalf("list filters: %#v", got.Items)
	}
	if got := get("/api/nodes?assignee=" + p.ID); len(got.Items) != 1 || got.Items[0].ID != a.ID {
		t.Fatalf("assignee filter: %#v", got.Items)
	}
	if got := get("/api/nodes?assignee=none&within=" + root.ID); len(got.Items) != 2 {
		t.Fatalf("unassigned: %#v", got.Items)
	}
	if got := get("/api/nodes?hide_closed=true&within=" + root.ID); len(got.Items) != 2 {
		t.Fatalf("hide closed: %#v", got.Items)
	}
	if got := get("/api/nodes?q=PAI-2&sort=key"); len(got.Items) != 2 || got.Items[0].ID != a.ID {
		t.Fatalf("key prefix search: %#v", got.Items)
	}
	if got := get("/api/nodes?q=gamMA"); len(got.Items) != 1 || got.Items[0].ID != c.ID {
		t.Fatalf("title search: %#v", got.Items)
	}
	for _, sort := range []string{"key", "title", "state", "priority", "kind", "updated_at", "created_at", "position", "state,-updated_at"} {
		t.Run(sort, func(t *testing.T) {
			base := "/api/nodes?within=" + root.ID + "&sort=" + sort + "&limit=1"
			seen := map[string]bool{}
			cursor := ""
			for i := 0; i < 4; i++ {
				path := base
				if cursor != "" {
					path += "&cursor=" + url.QueryEscape(cursor)
				}
				page := get(path)
				if len(page.Items) == 0 {
					break
				}
				if seen[page.Items[0].ID] {
					t.Fatalf("duplicate cursor item for %s", sort)
				}
				seen[page.Items[0].ID] = true
				if page.NextCursor == nil {
					break
				}
				cursor = *page.NextCursor
			}
			if len(seen) != 3 {
				t.Fatalf("cursor %s returned %d distinct nodes", sort, len(seen))
			}
		})
	}
	if got := get("/api/nodes?within=" + root.ID + "&sort=state,-updated_at"); got.Items[0].ID != a.ID || got.Items[1].ID != c.ID || got.Items[2].ID != b.ID {
		t.Fatalf("workflow order: %#v", got.Items)
	}
	for _, path := range []string{"/api/nodes?limit=501", "/api/nodes?within=bad", "/api/nodes?sort=bogus", "/api/nodes?facets=bogus", "/api/nodes?within=" + root.ID + "&parent_id=" + root.ID} {
		status, body := call(t, &p, http.MethodGet, path, "")
		if status != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", path, status, body)
		}
	}
	if got := get("/api/nodes?limit=500"); len(got.Items) != 6 {
		t.Fatalf("limit 500: %d", len(got.Items))
	}
	status, body := call(t, &p, http.MethodGet, "/api/projects", "")
	projects := decode[projectPage](t, status, body, http.StatusOK)
	if len(projects.Items) != 1 || projects.Items[0].ID != root.ID || projects.Items[0].Total != 3 || projects.Items[0].Open != 1 || projects.Items[0].InProgress != 1 || projects.Items[0].Done != 1 || projects.Items[0].LastActivity.IsZero() {
		t.Fatalf("projects: %#v", projects.Items)
	}
	status, body = call(t, &p, http.MethodGet, "/api/projects?include_archived=true", "")
	if got := decode[projectPage](t, status, body, http.StatusOK); len(got.Items) != 2 {
		t.Fatalf("archived projects: %#v", got.Items)
	}
	other := addPrincipal(t, "other-polished")
	assertTenantEmpty(t, other, "/api/nodes?facets=state,kind,priority,assignee")
	assertTenantEmpty(t, other, "/api/projects?include_archived=true")
	status, body = call(t, &other, http.MethodGet, "/api/nodes?within="+root.ID, "")
	if status != http.StatusOK {
		t.Fatalf("cross tenant within: %d %s", status, body)
	}
	if page := decode[nodePage](t, status, body, http.StatusOK); len(page.Items) != 0 {
		t.Fatalf("cross tenant within: %#v", page.Items)
	}
}
func assertTenantEmpty(t *testing.T, p tenant.Principal, path string) {
	t.Helper()
	status, body := call(t, &p, http.MethodGet, path, "")
	if status != http.StatusOK {
		t.Fatalf("%s: %d %s", path, status, body)
	}
	var page struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil || len(page.Items) != 0 {
		t.Fatalf("%s: %s (%v)", path, body, err)
	}
}

func TestList6000Performance(t *testing.T) {
	p := newPrincipal(t, "large-list")
	project := kindBySlug(t, p, "project")
	ticket := kindBySlug(t, p, "ticket")
	root := mustNode(t, p, `{"kind_id":"`+project.ID+`","title":"Large project"}`)
	err := db.InTenant(t.Context(), appPool, p.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO nodes (tenant_id,key,kind_id,title,fields,state,parent_id,position)
            SELECT $1::uuid,'PERF-'||g,$2::uuid,'Item '||g,
                jsonb_build_object('priority',CASE g%3 WHEN 0 THEN 'high' WHEN 1 THEN 'medium' ELSE 'low' END),
                CASE g%4 WHEN 0 THEN 'new' WHEN 1 THEN 'active' WHEN 2 THEN 'qa' ELSE 'done' END,
                $3::uuid,g FROM generate_series(1,6000) AS g`, p.TenantID, ticket.ID, root.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/nodes?within=" + root.ID + "&sort=state,-updated_at&limit=50&facets=state,kind,priority,assignee"
	var fastest time.Duration
	for i := 0; i < 3; i++ {
		start := time.Now()
		status, body := call(t, &p, http.MethodGet, path, "")
		elapsed := time.Since(start)
		page := decode[nodePage](t, status, body, http.StatusOK)
		if len(page.Items) != 50 || page.NextCursor == nil || page.Facets["kind"]["ticket"] != 6000 {
			t.Fatalf("large list result: %d, %#v", len(page.Items), page.Facets)
		}
		if fastest == 0 || elapsed < fastest {
			fastest = elapsed
		}
	}
	t.Logf("6000-node list with facets: fastest of 3 = %s", fastest)
	// Shared CI runners are slower and noisier than a workstation; keep a
	// regression guard there without failing on runner variance.
	limit := 150 * time.Millisecond
	if os.Getenv("CI") != "" {
		limit = 600 * time.Millisecond
	}
	if fastest >= limit {
		t.Fatalf("6000-node list exceeded %s: %s", limit, fastest)
	}
	start := time.Now()
	status, body := call(t, &p, http.MethodGet, "/api/projects", "")
	projects := decode[projectPage](t, status, body, http.StatusOK)
	t.Logf("6000-node projects: %s", time.Since(start))
	if len(projects.Items) != 1 || projects.Items[0].Total != 6000 {
		t.Fatalf("large project: %#v", projects.Items)
	}
}
