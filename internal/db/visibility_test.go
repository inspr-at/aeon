// SPDX-License-Identifier: AGPL-3.0-only

package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/jackc/pgx/v5"
)

type visibilityFixture struct {
	d                         *dbtest.DB
	tenant                    string
	actor                     string
	projectA, projectB        string
	ticketA, ticketB, orgNode string
	kinds                     map[string]string
}

func newVisibilityFixture(t *testing.T, d *dbtest.DB, slug string) *visibilityFixture {
	t.Helper()
	ctx := dbtest.Seed(t.Context())
	f := &visibilityFixture{d: d, kinds: map[string]string{}}
	if err := d.Admin.QueryRow(ctx, `INSERT INTO tenants(slug,name) VALUES($1,$1) RETURNING id::text`, slug).Scan(&f.tenant); err != nil {
		t.Fatal(err)
	}
	err := db.InTenant(ctx, d.App, f.tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO node_kinds(tenant_id,slug,label,short_prefix,icon) VALUES($1,'organisation','Organisation','ORG','organisation')`, f.tenant); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT slug,id::text FROM node_kinds`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var slug, id string
			if err := rows.Scan(&slug, &id); err != nil {
				rows.Close()
				return err
			}
			f.kinds[slug] = id
		}
		rows.Close()
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1,'person','Actor') RETURNING id::text`, f.tenant).Scan(&f.actor); err != nil {
			return err
		}
		f.projectA = insertNode(ctx, t, tx, f, "project", "PA-1", nil)
		f.projectB = insertNode(ctx, t, tx, f, "project", "PB-1", nil)
		f.ticketA = insertNode(ctx, t, tx, f, "ticket", "TA-1", &f.projectA)
		f.ticketB = insertNode(ctx, t, tx, f, "ticket", "TB-1", &f.projectB)
		f.orgNode = insertNode(ctx, t, tx, f, "organisation", "ORG-1", nil)
		if _, err := tx.Exec(ctx, `INSERT INTO node_relations(tenant_id,source_node_id,target_node_id,type) VALUES($1,$2,$3,'blocks')`, f.tenant, f.ticketA, f.ticketB); err != nil {
			return err
		}
		for _, node := range []string{f.ticketA, f.ticketB} {
			if _, err := tx.Exec(ctx, `INSERT INTO events(tenant_id,actor_principal_id,node_id,type,after) VALUES($1,$2,$3,'comment.created','{"body_markdown":"x"}')`, f.tenant, f.actor, node); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO attachments(tenant_id,node_id,sha256,name,content_type,size,created_by) VALUES($1,$2,repeat('a',64),'a.txt','text/plain',1,$3)`, f.tenant, node, f.actor); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO events(tenant_id,actor_principal_id,type,after) VALUES($1,$2,'kind.created','{"x":1}')`, f.tenant, f.actor)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func insertNode(ctx context.Context, t *testing.T, tx pgx.Tx, f *visibilityFixture, kind, key string, parent *string) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,kind_id,key,title,parent_id) VALUES($1,$2,$3,$3,$4) RETURNING id::text`, f.tenant, f.kinds[kind], key, parent).Scan(&id); err != nil {
		t.Fatalf("insert %s: %v", key, err)
	}
	return id
}

type visibleCounts struct {
	nodes, relations, nodeEvents, workspaceEvents, attachments, embeddingJobs int
}

func countVisible(t *testing.T, ctx context.Context, f *visibilityFixture) visibleCounts {
	t.Helper()
	var c visibleCounts
	err := db.InTenant(ctx, f.d.App, f.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT
		  (SELECT count(*) FROM nodes),(SELECT count(*) FROM node_relations),
		  (SELECT count(*) FROM events WHERE node_id IS NOT NULL),(SELECT count(*) FROM events WHERE node_id IS NULL),
		  (SELECT count(*) FROM attachments),(SELECT count(*) FROM node_embedding_jobs)`).Scan(
			&c.nodes, &c.relations, &c.nodeEvents, &c.workspaceEvents, &c.attachments, &c.embeddingJobs)
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// ADR-003 P2: with visibility unset, nothing project-scoped is visible; an
// explicit service visibility opens all projects or exactly the listed ones.
func TestProjectVisibilityFailsClosed(t *testing.T) {
	d := dbtest.Open(t)
	f := newVisibilityFixture(t, d, "p2-closed")
	ctx := t.Context()

	none := countVisible(t, ctx, f)
	if none.nodes != 0 || none.relations != 0 || none.nodeEvents != 0 || none.attachments != 0 || none.embeddingJobs != 0 {
		t.Fatalf("unset visibility shows project data: %+v", none)
	}
	// Workspace events are not project data; system code without a principal
	// (sign-in) still reads them.
	if none.workspaceEvents == 0 {
		t.Fatalf("unset visibility hides workspace events: %+v", none)
	}
	if got := countVisible(t, db.NoProjects(ctx, "test"), f); got != none {
		t.Fatalf("NoProjects %+v, unset %+v", got, none)
	}
	all := countVisible(t, db.AllProjects(ctx, "test"), f)
	if all.nodes != 5 || all.relations != 1 || all.nodeEvents != 2 || all.attachments != 2 || all.embeddingJobs != 5 {
		t.Fatalf("all projects: %+v", all)
	}
	onlyA := countVisible(t, db.OnlyProjects(ctx, f.projectA), f)
	// Project A and its ticket; not B, not the workspace-level organisation;
	// the relation to B is hidden because one end is invisible.
	if onlyA.nodes != 2 || onlyA.relations != 0 || onlyA.nodeEvents != 1 || onlyA.attachments != 1 || onlyA.embeddingJobs != 2 {
		t.Fatalf("only project A: %+v", onlyA)
	}
	if got := countVisible(t, db.OnlyProjects(ctx), f); got.nodes != 0 || got.nodeEvents != 0 {
		t.Fatalf("empty project list: %+v", got)
	}

	// Writes fail closed too: nothing can be created or changed unseen.
	err := db.InTenant(ctx, d.App, f.tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO nodes(tenant_id,kind_id,key,title,parent_id) VALUES($1,$2,'TA-2','x',$3)`, f.tenant, f.kinds["ticket"], f.projectA)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "42501") && !strings.Contains(err.Error(), "parent node does not exist") {
		t.Fatalf("insert without visibility: %v", err)
	}
	var updated int64
	err = db.InTenant(db.OnlyProjects(ctx, f.projectA), d.App, f.tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE nodes SET title='changed' WHERE id=$1`, f.ticketB)
		updated = tag.RowsAffected()
		return err
	})
	if err != nil || updated != 0 {
		t.Fatalf("update of an invisible node: rows=%d err=%v", updated, err)
	}
	err = db.InTenant(db.OnlyProjects(ctx, f.projectA), d.App, f.tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE nodes SET parent_id=$1 WHERE id=$2`, f.projectB, f.ticketA)
		return err
	})
	if err == nil {
		t.Fatal("a caller moved a node into a project it cannot see")
	}
	err = db.InTenant(db.OnlyProjects(ctx, f.projectA), d.App, f.tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO node_relations(tenant_id,source_node_id,target_node_id,type) VALUES($1,$2,$3,'cites')`, f.tenant, f.ticketA, f.ticketB)
		return err
	})
	if err == nil {
		t.Fatal("a caller linked to a node it cannot see")
	}
	// A malformed setting is an error, never a wider view.
	err = db.InTenant(ctx, d.App, f.tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('aeon.visible_projects','garbage',true)`); err != nil {
			return err
		}
		var n int
		return tx.QueryRow(ctx, `SELECT count(*) FROM nodes`).Scan(&n)
	})
	if err == nil {
		t.Fatal("malformed visibility was accepted")
	}
}

// The project root follows every insert, move, re-parent and kind change, down
// the whole subtree, and cannot be written directly.
func TestNodeProjectRootFollowsMoves(t *testing.T) {
	d := dbtest.Open(t)
	f := newVisibilityFixture(t, d, "p2-moves")
	ctx := dbtest.Seed(t.Context())
	projectOf := func(tx pgx.Tx, id string) string {
		t.Helper()
		var project *string
		if err := tx.QueryRow(ctx, `SELECT project_id::text FROM nodes WHERE id=$1`, id).Scan(&project); err != nil {
			t.Fatal(err)
		}
		if project == nil {
			return ""
		}
		return *project
	}
	err := db.InTenant(ctx, d.App, f.tenant, func(tx pgx.Tx) error {
		epic := insertNode(ctx, t, tx, f, "epic", "EA-1", &f.projectA)
		ticket := insertNode(ctx, t, tx, f, "ticket", "TA-9", &epic)
		task := insertNode(ctx, t, tx, f, "task", "KA-9", &ticket)
		deleted := insertNode(ctx, t, tx, f, "task", "KA-10", &ticket)
		if _, err := tx.Exec(ctx, `UPDATE nodes SET deleted_at=now() WHERE id=$1`, deleted); err != nil {
			return err
		}
		for _, id := range []string{f.projectA, epic, ticket, task, deleted} {
			if got := projectOf(tx, id); got != f.projectA {
				t.Fatalf("insert: %s has project %q, want A", id, got)
			}
		}
		if got := projectOf(tx, f.orgNode); got != "" {
			t.Fatalf("workspace node has project %q", got)
		}
		// Move the epic, with its whole subtree, to project B.
		if _, err := tx.Exec(ctx, `UPDATE nodes SET parent_id=$1 WHERE id=$2`, f.projectB, epic); err != nil {
			return err
		}
		for _, id := range []string{epic, ticket, task, deleted} {
			if got := projectOf(tx, id); got != f.projectB {
				t.Fatalf("move to B: %s has project %q", id, got)
			}
		}
		// Re-parent a subtree out of every project.
		if _, err := tx.Exec(ctx, `UPDATE nodes SET parent_id=NULL WHERE id=$1`, ticket); err != nil {
			return err
		}
		for _, id := range []string{ticket, task, deleted} {
			if got := projectOf(tx, id); got != "" {
				t.Fatalf("move to root: %s has project %q", id, got)
			}
		}
		if got := projectOf(tx, epic); got != f.projectB {
			t.Fatalf("epic lost its project: %q", got)
		}
		// Back under the epic, and a direct write of the column is ignored.
		if _, err := tx.Exec(ctx, `UPDATE nodes SET parent_id=$1 WHERE id=$2`, epic, ticket); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE nodes SET project_id=$1 WHERE id=$2`, f.projectA, task); err != nil {
			return err
		}
		if got := projectOf(tx, task); got != f.projectB {
			t.Fatalf("direct write took effect: %q", got)
		}
		// A nested project is its own project; moving its parent leaves it.
		nested := insertNode(ctx, t, tx, f, "project", "PN-1", &epic)
		inner := insertNode(ctx, t, tx, f, "ticket", "TN-1", &nested)
		if projectOf(tx, nested) != nested || projectOf(tx, inner) != nested {
			t.Fatal("nested project does not own its subtree")
		}
		if _, err := tx.Exec(ctx, `UPDATE nodes SET parent_id=$1 WHERE id=$2`, f.projectA, epic); err != nil {
			return err
		}
		if projectOf(tx, epic) != f.projectA || projectOf(tx, task) != f.projectA || projectOf(tx, nested) != nested || projectOf(tx, inner) != nested {
			t.Fatal("move crossed into a nested project")
		}
		// A kind change into a project takes the subtree along; back out, it
		// returns the subtree to the enclosing project.
		if _, err := tx.Exec(ctx, `UPDATE nodes SET kind_id=$1 WHERE id=$2`, f.kinds["project"], ticket); err != nil {
			return err
		}
		if projectOf(tx, ticket) != ticket || projectOf(tx, task) != ticket {
			t.Fatal("kind change to project did not cascade")
		}
		if _, err := tx.Exec(ctx, `UPDATE nodes SET kind_id=$1 WHERE id=$2`, f.kinds["ticket"], ticket); err != nil {
			return err
		}
		if projectOf(tx, ticket) != f.projectA || projectOf(tx, task) != f.projectA {
			t.Fatal("kind change from project did not cascade")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A caller seeing only A loses the moved subtree at once.
	var visible int
	if err := db.InTenant(db.OnlyProjects(t.Context(), f.projectB), d.App, f.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM nodes WHERE key IN ('EA-1','TA-9','KA-9')`).Scan(&visible)
	}); err != nil {
		t.Fatal(err)
	}
	if visible != 0 {
		t.Fatalf("project B still sees %d moved nodes", visible)
	}
}

// Production applies migrations as the table-owning NOSUPERUSER NOBYPASSRLS
// role, so FORCE ROW LEVEL SECURITY applies to every data step (AEON-158).
// Re-run the P2 data steps as that role on data from two tenants.
func TestProjectAccessMigrationsRunUnderForcedRLS(t *testing.T) {
	d := dbtest.Open(t)
	one := newVisibilityFixture(t, d, "p2-mig-one")
	two := newVisibilityFixture(t, d, "p2-mig-two")
	ctx := t.Context()
	// Undo what the superuser migration run did: no project roots, and a
	// classic external person with the P1 workspace Guest placeholder.
	if _, err := d.Admin.Exec(ctx, `ALTER TABLE nodes DISABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `UPDATE nodes SET project_id=NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `ALTER TABLE nodes ENABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	var external string
	if err := d.Admin.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1,'person','External',ARRAY['external']) RETURNING id::text`, two.tenant).Scan(&external); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `ALTER TABLE role_bindings DISABLE TRIGGER role_bindings_guest_project_only`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) SELECT $1,$2,id,'workspace' FROM roles WHERE tenant_id=$1 AND key='guest'`, two.tenant, external); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `ALTER TABLE role_bindings ENABLE TRIGGER role_bindings_guest_project_only`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"0820_node_project_root.sql", "0822_guest_project_only.sql"} {
		body, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		start := strings.Index(text, "DO $$")
		end := strings.Index(text[start:], "\n$$;") + start + len("\n$$;")
		if start < 0 || end <= start {
			t.Fatalf("%s: no data step", name)
		}
		if _, err := d.App.Exec(ctx, text[start:end]); err != nil {
			t.Fatalf("%s data step as the app role: %v", name, err)
		}
	}
	for _, f := range []*visibilityFixture{one, two} {
		var missing, wrong int
		if err := d.Admin.QueryRow(ctx, `SELECT
		   count(*) FILTER (WHERE project_id IS NULL AND id<>$2),
		   count(*) FILTER (WHERE id IN ($3,$4) AND project_id IS DISTINCT FROM (CASE WHEN id=$3 THEN $5 ELSE $6 END)::uuid)
		   FROM nodes WHERE tenant_id=$1`, f.tenant, f.orgNode, f.ticketA, f.ticketB, f.projectA, f.projectB).Scan(&missing, &wrong); err != nil {
			t.Fatal(err)
		}
		if missing != 0 || wrong != 0 {
			t.Fatalf("tenant %s backfill: %d missing, %d wrong", f.tenant, missing, wrong)
		}
	}
	var guests, events int
	if err := d.Admin.QueryRow(ctx, `SELECT
	   (SELECT count(*) FROM role_bindings b JOIN roles r ON r.tenant_id=b.tenant_id AND r.id=b.role_id WHERE b.scope_type='workspace' AND r.key='guest'),
	   (SELECT count(*) FROM events WHERE type='binding.removed' AND before->>'principal_id'=$1)`, external).Scan(&guests, &events); err != nil {
		t.Fatal(err)
	}
	if guests != 0 || events != 1 {
		t.Fatalf("workspace guest bindings %d, removal events %d", guests, events)
	}
	// And the placeholder cannot come back.
	err := db.InTenant(dbtest.Seed(ctx), d.App, two.tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT aeon_bind_legacy_principal($1::uuid,$2::uuid)`, two.tenant, external); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM role_bindings WHERE principal_id=$1`, external).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("external person rebound: %d", n)
		}
		_, err := tx.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) SELECT $1,$2,id,'workspace' FROM roles WHERE tenant_id=$1 AND key='guest'`, two.tenant, external)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "guest is a project role") {
		t.Fatalf("workspace guest binding: %v", err)
	}
}
