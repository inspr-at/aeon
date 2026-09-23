// SPDX-License-Identifier: AGPL-3.0-only
package importer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
)

func fakeClassic(t *testing.T) (*HTTPSource, func()) {
	t.Helper()
	responses := map[string]string{
		"/api/projects?status=all":       `[{"id":3,"key":"PAI","name":"Paimos","description":"legacy","status":"active","rate_hourly":100}]`,
		"/api/projects?status=deleted":   `[]`,
		"/api/users":                     `[{"id":7,"username":"markus","email":"markus@example.test","role":"admin","locale":"de"}]`,
		"/api/users?status=deleted":      `[]`,
		"/api/issues?limit=100&offset=0": `{"issues":[{"id":99,"issue_key":"SPRINT-99","type":"sprint","title":"Sprint","status":"open"}],"has_more":false}`,
		"/api/issues/trash":              `[]`,
		"/api/projects/3/issues":         `[{"id":10,"project_id":3,"issue_key":"PAI-10","type":"epic","title":"Epic","status":"open","description":"root"},{"id":11,"project_id":3,"issue_key":"PAI-11","type":"ticket","title":"Ticket","status":"open","description":"body","parent_id":10,"priority":"high"}]`,
		"/api/projects/3/knowledge":      `[{"id":12,"project_id":3,"type":"memory","slug":"lesson","title":"Lesson","body":"keep","status":"active","metadata":{"a":1}}]`,
		"/api/issues/12":                 `{"id":12,"project_id":3,"issue_key":"PAI-12","type":"memory","title":"Lesson","status":"active"}`,
		"/api/issues/10/relations":       `[{"source_id":10,"target_id":11,"type":"parent"}]`,
		"/api/issues/11/relations":       `[{"source_id":11,"target_id":10,"type":"depends_on"}]`,
		"/api/issues/11/comments":        `[{"id":30,"issue_id":11,"body":"comment","visibility":"internal","created_at":"2026-01-01 12:00:00"}]`,
		"/api/issues/11/history":         `[{"id":40,"issue_id":11,"snapshot":{"status":"open"},"changed_at":"2026-01-01 12:00:00"}]`,
		"/api/issues/11/attachments":     `[{"id":50,"issue_id":11,"object_key":"private/object","filename":"a.txt","size_bytes":4,"created_at":"2026-01-01 12:00:00"}]`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("source method %s", r.Method)
			w.WriteHeader(405)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fake-key" {
			t.Error("missing bearer")
			w.WriteHeader(401)
			return
		}
		body, ok := responses[r.URL.RequestURI()]
		if !ok && strings.HasSuffix(r.URL.Path, "/relations") {
			body = "[]"
			ok = true
		}
		if !ok && strings.HasSuffix(r.URL.Path, "/comments") {
			body = "[]"
			ok = true
		}
		if !ok && strings.HasSuffix(r.URL.Path, "/history") {
			body = "[]"
			ok = true
		}
		if !ok && strings.HasSuffix(r.URL.Path, "/attachments") {
			body = "[]"
			ok = true
		}
		if !ok {
			t.Errorf("unexpected source GET %s", r.URL.RequestURI())
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	file := filepath.Join(t.TempDir(), "api-key")
	if err := os.WriteFile(file, []byte("fake-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	src, err := NewHTTPSource(server.URL, file, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return src, server.Close
}

func TestImportDryRunAndRerun(t *testing.T) {
	source, closeServer := fakeClassic(t)
	defer closeServer()
	ctx := context.Background()
	dry, err := (Importer{Source: source}).Run(ctx, "test", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if dry.Counts["projects"] != 1 || dry.Counts["ticket"] != 1 || dry.Counts["memory"] != 1 || dry.Counts["sprint"] != 1 || dry.Counts["comments"] != 1 {
		t.Fatalf("wrong dry run: %+v", dry)
	}
	if !strings.Contains(strings.Join(dry.UnmappedFields, ","), "issues.priority") {
		t.Fatalf("missing unmapped field: %+v", dry)
	}
	d := dbtest.Open(t)
	if err := db.EnsureTenant(ctx, d.Admin, "test", "Test"); err != nil {
		t.Fatal(err)
	}
	job := Importer{Source: source, Writer: PostgresWriter{Pool: d.App}}
	first, err := job.Run(ctx, "test", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Created != 5 {
		t.Fatalf("created %d", first.Created)
	}
	second, err := job.Run(ctx, "test", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.Updated != 0 {
		t.Fatalf("rerun changed nodes: %+v", second)
	}
	var count int
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM nodes WHERE key IN ('PAI-10','PAI-11','PAI-12','SPRINT-99')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("issue keys lost: %d", count)
	}
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM nodes child JOIN nodes parent ON child.parent_id=parent.id WHERE child.key='PAI-11' AND parent.key='PAI-10'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("parent edge missing")
	}
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM node_relations r JOIN nodes source ON r.source_node_id=source.id JOIN nodes target ON r.target_node_id=target.id WHERE r.type='blocks' AND source.key='PAI-10' AND target.key='PAI-11'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("dependency direction lost")
	}
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM events WHERE type IN ('import.comment','import.history','import.attachment')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("auxiliary events duplicated or lost: %d", count)
	}
	var commentAt time.Time
	if err := d.Admin.QueryRow(ctx, `SELECT at FROM events WHERE type='import.comment'`).Scan(&commentAt); err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC); !commentAt.Equal(want) {
		t.Fatalf("classic comment time: got %s, want %s", commentAt, want)
	}
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM events WHERE type='import.parent_changed'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("parent events duplicated or lost: %d", count)
	}
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM identities WHERE issuer='paimos-classic'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("identities: %d", count)
	}
	var fields []byte
	if err := d.Admin.QueryRow(ctx, `SELECT fields FROM nodes WHERE key='PAI-11'`).Scan(&fields); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(fields, &data); err != nil {
		t.Fatal(err)
	}
	if data["classic"].(map[string]any)["record"].(map[string]any)["priority"] != "high" {
		t.Fatal("unmapped field lost")
	}
}

func TestAllClassicIssueKindsUseR1Nodes(t *testing.T) {
	d := dbtest.Open(t)
	ctx := context.Background()
	if err := db.EnsureTenant(ctx, d.Admin, "kinds", "Kinds"); err != nil {
		t.Fatal(err)
	}
	kinds := []string{"task", "release", "cost_unit", "runbook", "guideline", "external_system", "related_project"}
	p := Project{Record: Record{"id": 1, "key": "PAI", "name": "Paimos", "status": "active"}}
	for n, kind := range kinds {
		p.Issues = append(p.Issues, Record{"id": n + 101, "issue_key": "PAI-" + strconv.Itoa(n+101), "type": kind, "title": kind, "status": "open"})
	}
	snap := Snapshot{SourceID: "test-source", Projects: []Project{p}, Details: map[int64]Details{}}
	result, err := (PostgresWriter{Pool: d.App}).Write(ctx, snap, "kinds")
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != len(kinds)+1 {
		t.Fatalf("created %d", result.Created)
	}
	for _, kind := range kinds {
		var count int
		if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM nodes n JOIN node_kinds k ON n.kind_id=k.id WHERE k.slug=$1`, kind).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("kind %s missing", kind)
		}
	}
}
