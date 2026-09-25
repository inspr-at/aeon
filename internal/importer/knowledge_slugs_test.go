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

// AEON-138: imported knowledge entries need fields.slug to open. The importer
// now writes it; migration 0800 repairs entries written before that, and a
// later delta import must treat the repaired entries as unchanged.
func TestKnowledgeClassicSlugBackfill(t *testing.T) {
	d := dbtest.Open(t)
	ctx := t.Context()
	if err := db.EnsureTenant(ctx, d.Admin, "ks", "Knowledge slugs"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../db/migrations/0800_knowledge_classic_slugs.sql")
	if err != nil {
		t.Fatal(err)
	}
	guideline := Record{"id": json.Number("201"), "issue_key": "KS-201", "type": "guideline", "title": "ADR-001", "status": "open",
		"slug": "adr-001-foundation", "metadata": map[string]any{"source_repo": "aeon"}}
	ticket := Record{"id": json.Number("202"), "issue_key": "KS-202", "type": "ticket", "title": "A ticket", "status": "open", "slug": nil}
	snapshot := func(title string) Snapshot {
		g := Record{}
		for k, v := range guideline {
			g[k] = v
		}
		g["title"] = title
		return Snapshot{SourceID: "ks-source", Details: map[int64]Details{}, Projects: []Project{{
			Record: Record{"id": json.Number("1"), "key": "KS", "name": "Knowledge", "status": "active"},
			Issues: []Record{g, ticket},
		}}}
	}
	writer := PostgresWriter{Pool: d.App}
	if _, err := writer.Write(ctx, snapshot("ADR-001"), "ks"); err != nil {
		t.Fatal(err)
	}
	var tid string
	if err := d.Admin.QueryRow(ctx, `SELECT id::text FROM tenants WHERE slug='ks'`).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	fieldsOf := func(key string) map[string]any {
		t.Helper()
		var raw []byte
		if err := db.InTenant(ctx, d.App, tid, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT fields FROM nodes WHERE tenant_id=$1 AND key=$2`, tid, key).Scan(&raw)
		}); err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if f := fieldsOf("KS-201"); f["slug"] != "adr-001-foundation" || f["metadata"].(map[string]any)["source_repo"] != "aeon" {
		t.Fatalf("importer did not map slug and metadata: %v", f)
	}
	if _, ok := fieldsOf("KS-202")["slug"]; ok {
		t.Fatal("a ticket got a knowledge slug")
	}

	// Rewind the guideline to what the importer wrote before AEON-138: no slug or
	// metadata outside fields.classic, and that shape as the last import baseline.
	if err := db.InTenant(ctx, d.Admin, tid, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			WITH old AS (SELECT to_jsonb(n) AS snap FROM nodes n WHERE tenant_id=$1 AND key='KS-201'),
			     upd AS (UPDATE nodes SET fields=fields-'slug'-'metadata' WHERE tenant_id=$1 AND key='KS-201' RETURNING *)
			INSERT INTO events(tenant_id,actor_principal_id,node_id,type,before,after)
			SELECT $1, (SELECT actor_principal_id FROM events WHERE tenant_id=$1 AND node_id=upd.id AND type='import.node_created'),
			       upd.id, 'import.node_updated', old.snap, to_jsonb(upd) FROM upd, old`, tid)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := fieldsOf("KS-201")["slug"]; ok {
		t.Fatal("rewind kept the slug")
	}
	var updatedBefore string
	if err := d.Admin.QueryRow(ctx, `SELECT updated_at::text FROM nodes WHERE tenant_id=$1 AND key='KS-201'`, tid).Scan(&updatedBefore); err != nil {
		t.Fatal(err)
	}
	importEvents := func() int {
		t.Helper()
		var n int
		if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM events e JOIN nodes n ON n.id=e.node_id WHERE e.tenant_id=$1 AND n.key='KS-201' AND e.type='import.node_updated'`, tid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	base := importEvents()
	for range 2 {
		if err := db.InTenant(ctx, d.Admin, tid, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, string(migration)); return err }); err != nil {
			t.Fatal(err)
		}
	}
	if got := importEvents(); got != base+1 {
		t.Fatalf("migration events: %d, want %d (one repair, idempotent rerun)", got, base+1)
	}
	if f := fieldsOf("KS-201"); f["slug"] != "adr-001-foundation" || f["metadata"].(map[string]any)["source_repo"] != "aeon" {
		t.Fatalf("migration did not backfill: %v", f)
	}
	if _, ok := fieldsOf("KS-202")["slug"]; ok {
		t.Fatal("migration touched a ticket")
	}
	var updatedAfter string
	if err := d.Admin.QueryRow(ctx, `SELECT updated_at::text FROM nodes WHERE tenant_id=$1 AND key='KS-201'`, tid).Scan(&updatedAfter); err != nil {
		t.Fatal(err)
	}
	if updatedAfter != updatedBefore {
		t.Fatalf("repair moved updated_at: %s -> %s", updatedBefore, updatedAfter)
	}

	// The delta import sees the repaired entry as unchanged, and a real classic
	// edit still applies without a false "changed in Aeon" conflict.
	same, err := writer.Write(ctx, snapshot("ADR-001"), "ks")
	if err != nil {
		t.Fatal(err)
	}
	if same.Updated != 0 || len(same.Conflicts) != 0 {
		t.Fatalf("unchanged delta: updated %d, conflicts %v", same.Updated, same.Conflicts)
	}
	edited, err := writer.Write(ctx, snapshot("ADR-001 (accepted)"), "ks")
	if err != nil {
		t.Fatal(err)
	}
	if edited.Updated != 1 || len(edited.Conflicts) != 0 {
		t.Fatalf("edited delta: updated %d, conflicts %v", edited.Updated, edited.Conflicts)
	}
}
