// SPDX-License-Identifier: AGPL-3.0-only
package importer

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/jackc/pgx/v5"
)

func TestImportedProjectKeyBackfill(t *testing.T) {
	d := dbtest.Open(t)
	ctx := t.Context()
	if err := db.EnsureTenant(ctx, d.Admin, "project-keys", "Project keys"); err != nil {
		t.Fatal(err)
	}
	var tenantID string
	if err := d.Admin.QueryRow(ctx, `SELECT id::text FROM tenants WHERE slug='project-keys'`).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../db/migrations/0801_import_project_keys.sql")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func(name string) Snapshot {
		return Snapshot{SourceID: "project-key-source", Details: map[int64]Details{}, Projects: []Project{{
			Record: Record{"id": json.Number("1"), "key": "PK", "name": name, "status": "active"},
		}}}
	}
	writer := PostgresWriter{Pool: d.App}
	if _, err := writer.Write(ctx, snapshot("First"), "project-keys"); err != nil {
		t.Fatal(err)
	}
	var key, beforeTime string
	var baseEvents int
	read := func() {
		t.Helper()
		if err := db.InTenant(ctx, d.App, tenantID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT coalesce(n.fields->>'project_key',''),n.updated_at::text,
				(SELECT count(*) FROM events e WHERE e.tenant_id=n.tenant_id AND e.node_id=n.id AND e.type='import.node_updated')
				FROM nodes n WHERE n.tenant_id=$1 AND n.key='PRJ-1'`, tenantID).Scan(&key, &beforeTime, &baseEvents)
		}); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if key != "PK" {
		t.Fatalf("imported project key = %q, want PK", key)
	}
	// Restore the old importer shape and make it the last import baseline.
	if err := db.InTenant(ctx, d.Admin, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			WITH old AS (SELECT to_jsonb(n) AS snap FROM nodes n WHERE tenant_id=$1 AND key='PRJ-1'),
			     upd AS (UPDATE nodes SET fields=fields-'project_key' WHERE tenant_id=$1 AND key='PRJ-1' RETURNING *)
			INSERT INTO events(tenant_id,actor_principal_id,node_id,type,before,after)
			SELECT $1, (SELECT actor_principal_id FROM events WHERE tenant_id=$1 AND node_id=upd.id AND type='import.node_created'),
			       upd.id,'import.node_updated',old.snap,to_jsonb(upd) FROM upd,old`, tenantID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	read()
	if key != "" {
		t.Fatalf("rewind kept project key %q", key)
	}
	unchangedTime, eventCount := beforeTime, baseEvents
	for range 2 {
		if err := db.InTenant(ctx, d.Admin, tenantID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, string(migration))
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if key != "PK" || beforeTime != unchangedTime || baseEvents != eventCount+1 {
		t.Fatalf("backfill key=%q, timestamp changed=%t, added events=%d", key, beforeTime != unchangedTime, baseEvents-eventCount)
	}
	same, err := writer.Write(ctx, snapshot("First"), "project-keys")
	if err != nil {
		t.Fatal(err)
	}
	if same.Updated != 0 || len(same.Conflicts) != 0 {
		t.Fatalf("unchanged delta: updated %d, conflicts %v", same.Updated, same.Conflicts)
	}
	edited, err := writer.Write(ctx, snapshot("Second"), "project-keys")
	if err != nil {
		t.Fatal(err)
	}
	if edited.Updated != 1 || len(edited.Conflicts) != 0 {
		t.Fatalf("edited delta: updated %d, conflicts %v", edited.Updated, edited.Conflicts)
	}
}
