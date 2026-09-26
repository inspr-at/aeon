// SPDX-License-Identifier: AGPL-3.0-only
package importer

import (
	"context"
	"testing"

	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/dbtest"
)

func TestBackfillRelationsRequiresInput(t *testing.T) {
	if _, err := BackfillRelations(context.Background(), nil, "00000000-0000-0000-0000-000000000001"); err == nil {
		t.Fatal("missing pool")
	}
}

func TestClassicRelationMappingBackfillAndIdempotency(t *testing.T) {
	d := dbtest.Open(t)
	ctx := context.Background()
	if err := db.EnsureTenant(ctx, d.Admin, "map", "Map"); err != nil {
		t.Fatal(err)
	}
	issues := []Record{
		issue(20, 3, "release", "PAI-20", "R1"),
		issue(26, 3, "release", "PAI-26", "R2"),
		issue(21, 3, "ticket", "PAI-21", "Ticket"),
		issue(24, 3, "ticket", "PAI-24", "Other"),
		issue(27, 3, "ticket", "PAI-27", "Later"),
		issue(22, 3, "epic", "PAI-22", "Epic"),
		issue(23, 3, "memory", "PAI-23", "Memory"),
		issue(25, 3, "memory", "PAI-25", "Other memory"),
	}
	relations := []Record{
		{"source_id": 20, "target_id": 21, "type": "release"}, // container → member
		{"source_id": 27, "target_id": 26, "type": "release"}, // member → release, kinds decide
		{"source_id": 20, "target_id": 22, "type": "release"}, // epic is not a journey ticket
		{"source_id": 21, "target_id": 23, "type": "related"},
		{"source_id": 23, "target_id": 21, "type": "related"}, // same undirected pair
		{"source_id": 21, "target_id": 22, "type": "blocks"},
		{"source_id": 21, "target_id": 24, "type": "follows_from"},
		{"source_id": 24, "target_id": 23, "type": "impacts"},
		{"source_id": 24, "target_id": 25, "type": "applies_to_memory"},
		{"source_id": 22, "target_id": 25, "type": "groups"},
		{"source_id": 21, "target_id": 24, "type": "duplicates"},
		{"source_id": 23, "target_id": 22, "type": "cites"},
		{"source_id": 24, "target_id": 21, "type": "depends_on"},
		{"source_id": 22, "target_id": 21, "type": "parent"},
		{"source_id": 21, "target_id": 21, "type": "blocks"},
		{"source_id": 21, "target_id": 23, "type": "mentions"},
	}
	snap := relationSnapshot("classic-map", 3, issues, relations)
	first, err := (PostgresWriter{Pool: d.App}).Write(ctx, snap, "map")
	if err != nil {
		t.Fatal(err)
	}
	if first.Writes != 15 {
		t.Fatalf("writes %d", first.Writes)
	}
	assertClassicProjection(t, d)
	var classicType, recordType string
	if err := d.Admin.QueryRow(ctx, `SELECT after->>'classic_type', after->'record'->>'type' FROM events WHERE type='import.relation' AND after->'record'->>'type'='related' LIMIT 1`).Scan(&classicType, &recordType); err != nil {
		t.Fatal(err)
	}
	if classicType != "related" || recordType != "related" {
		t.Fatalf("classic type lost: %s %s", classicType, recordType)
	}
	var mapped, kept string
	if err := d.Admin.QueryRow(ctx, `SELECT after->>'type', after->>'classic_type' FROM events WHERE type='relation.created' AND after->>'classic_type'='related' LIMIT 1`).Scan(&mapped, &kept); err != nil {
		t.Fatal(err)
	}
	if mapped != "relates" || kept != "related" {
		t.Fatalf("relation event type=%s classic_type=%s", mapped, kept)
	}
	second, err := (PostgresWriter{Pool: d.App}).Write(ctx, snap, "map")
	if err != nil {
		t.Fatal(err)
	}
	if second.Writes != 0 || second.Created != 0 || second.Updated != 0 {
		t.Fatalf("writer rerun changed rows: %+v", second)
	}

	before := projectionCounts(t, d)
	if _, err := d.Admin.Exec(ctx, `DELETE FROM journey_tickets`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `DELETE FROM journey_releases`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `DELETE FROM journey_projects`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `DELETE FROM node_relations`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `UPDATE nodes SET parent_id=(SELECT id FROM nodes WHERE key='PRJ-3') WHERE key='PAI-21'`); err != nil {
		t.Fatal(err)
	}
	// Endpoint and project identity have to come from the import events.
	if _, err := d.Admin.Exec(ctx, `UPDATE nodes SET fields='{}'::jsonb`); err != nil {
		t.Fatal(err)
	}
	tenantID := tenantBySlug(t, d, "map")
	replay, err := BackfillRelations(ctx, d.App, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Writes != 15 || replay.Counts["relations"] != len(relations) {
		t.Fatalf("backfill writes=%d counts=%v", replay.Writes, replay.Counts)
	}
	assertClassicProjection(t, d)
	restored := projectionCounts(t, d)
	if restored.relations != before.relations || restored.releases != before.releases || restored.tickets != before.tickets {
		t.Fatalf("backfill restored %+v, want %+v", restored, before)
	}
	if restored.links <= before.links || restored.memberships <= before.memberships {
		t.Fatalf("backfill did not append link events: before %+v restored %+v", before, restored)
	}
	again, err := BackfillRelations(ctx, d.App, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Writes != 0 {
		t.Fatalf("second backfill writes %d", again.Writes)
	}
	if got := projectionCounts(t, d); got != restored {
		t.Fatalf("second backfill changed %+v -> %+v", restored, got)
	}

	if _, err := d.Admin.Exec(ctx, `UPDATE journey_tickets SET release_node_id=(SELECT id FROM nodes WHERE key='PAI-26') WHERE ticket_node_id=(SELECT id FROM nodes WHERE key='PAI-21')`); err != nil {
		t.Fatal(err)
	}
	held, err := BackfillRelations(ctx, d.App, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if held.Writes != 0 {
		t.Fatalf("replaced an assigned release, writes %d", held.Writes)
	}
	var releaseKey string
	if err := d.Admin.QueryRow(ctx, `SELECT n.key FROM journey_tickets t JOIN nodes n ON n.id=t.release_node_id JOIN nodes ticket ON ticket.id=t.ticket_node_id WHERE ticket.key='PAI-21'`).Scan(&releaseKey); err != nil {
		t.Fatal(err)
	}
	if releaseKey != "PAI-26" {
		t.Fatalf("ticket moved to %s", releaseKey)
	}
	if _, err := d.Admin.Exec(ctx, `UPDATE journey_tickets SET release_node_id=NULL WHERE ticket_node_id=(SELECT id FROM nodes WHERE key='PAI-21')`); err != nil {
		t.Fatal(err)
	}
	refilled, err := BackfillRelations(ctx, d.App, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if refilled.Writes != 1 {
		t.Fatalf("null membership writes %d", refilled.Writes)
	}
	if err := d.Admin.QueryRow(ctx, `SELECT n.key FROM journey_tickets t JOIN nodes n ON n.id=t.release_node_id JOIN nodes ticket ON ticket.id=t.ticket_node_id WHERE ticket.key='PAI-21'`).Scan(&releaseKey); err != nil {
		t.Fatal(err)
	}
	if releaseKey != "PAI-20" {
		t.Fatalf("membership restored to %s", releaseKey)
	}
	quiet, err := BackfillRelations(ctx, d.App, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if quiet.Writes != 0 {
		t.Fatalf("refill rerun writes %d", quiet.Writes)
	}
}

func TestBackfillTenantIsolation(t *testing.T) {
	d := dbtest.Open(t)
	ctx := context.Background()
	for _, slug := range []string{"alpha", "beta"} {
		if err := db.EnsureTenant(ctx, d.Admin, slug, slug); err != nil {
			t.Fatal(err)
		}
		snap := relationSnapshot("classic-"+slug, 9, []Record{
			issue(40, 9, "release", "ISO-40", "Release"),
			issue(41, 9, "ticket", "ISO-41", "Ticket"),
			issue(42, 9, "ticket", "ISO-42", "Blocked"),
		}, []Record{
			{"source_id": 41, "target_id": 42, "type": "blocks"},
			{"source_id": 40, "target_id": 41, "type": "release"},
		})
		if _, err := (PostgresWriter{Pool: d.App}).Write(ctx, snap, slug); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Admin.Exec(ctx, `DELETE FROM journey_tickets`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `DELETE FROM journey_releases`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `DELETE FROM journey_projects`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Admin.Exec(ctx, `DELETE FROM node_relations`); err != nil {
		t.Fatal(err)
	}
	alpha := tenantBySlug(t, d, "alpha")
	beta := tenantBySlug(t, d, "beta")
	report, err := BackfillRelations(ctx, d.App, alpha)
	if err != nil {
		t.Fatal(err)
	}
	if report.Writes == 0 {
		t.Fatal("alpha backfill wrote nothing")
	}
	var betaRelations, betaReleases int
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM node_relations WHERE tenant_id=$1`, beta).Scan(&betaRelations); err != nil {
		t.Fatal(err)
	}
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM journey_releases WHERE tenant_id=$1`, beta).Scan(&betaReleases); err != nil {
		t.Fatal(err)
	}
	if betaRelations != 0 || betaReleases != 0 {
		t.Fatalf("beta changed: relations %d releases %d", betaRelations, betaReleases)
	}
	var alphaBlocks int
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM node_relations r JOIN nodes s ON s.id=r.source_node_id JOIN nodes g ON g.id=r.target_node_id WHERE r.tenant_id=$1 AND r.type='blocks' AND s.key='ISO-41' AND g.key='ISO-42'`, alpha).Scan(&alphaBlocks); err != nil {
		t.Fatal(err)
	}
	if alphaBlocks != 1 {
		t.Fatalf("alpha blocks %d", alphaBlocks)
	}
	betaReport, err := BackfillRelations(ctx, d.App, beta)
	if err != nil {
		t.Fatal(err)
	}
	if betaReport.Writes == 0 {
		t.Fatal("beta backfill wrote nothing")
	}
	var alphaRelations int
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM node_relations WHERE tenant_id=$1`, alpha).Scan(&alphaRelations); err != nil {
		t.Fatal(err)
	}
	if alphaRelations != 1 {
		t.Fatalf("alpha relations after beta backfill: %d", alphaRelations)
	}
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM node_relations WHERE tenant_id=$1`, beta).Scan(&betaRelations); err != nil {
		t.Fatal(err)
	}
	if betaRelations != 1 {
		t.Fatalf("beta relations %d", betaRelations)
	}
}

func issue(id, project int, typ, key, title string) Record {
	return Record{"id": id, "project_id": project, "issue_key": key, "type": typ, "title": title, "status": "open"}
}

func relationSnapshot(source string, project int, issues []Record, relations []Record) Snapshot {
	anchor, _ := intField(issues[0], "id")
	return Snapshot{
		SourceID: source,
		Projects: []Project{{Record: Record{"id": project, "name": "Paimos", "status": "active"}, Issues: issues}},
		Details:  map[int64]Details{anchor: {Relations: relations}},
	}
}

func tenantBySlug(t *testing.T, d *dbtest.DB, slug string) string {
	t.Helper()
	var id string
	if err := d.Admin.QueryRow(context.Background(), `SELECT id::text FROM tenants WHERE slug=$1`, slug).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

type projectionCount struct {
	relations, releases, tickets, links, memberships int
}

func projectionCounts(t *testing.T, d *dbtest.DB) projectionCount {
	t.Helper()
	var c projectionCount
	ctx := context.Background()
	queries := []struct {
		dest *int
		sql  string
	}{
		{&c.relations, `SELECT count(*) FROM node_relations`},
		{&c.releases, `SELECT count(*) FROM journey_releases`},
		{&c.tickets, `SELECT count(*) FROM journey_tickets`},
		{&c.links, `SELECT count(*) FROM events WHERE type='relation.created'`},
		{&c.memberships, `SELECT count(*) FROM events WHERE type='import.release_membership'`},
	}
	for _, q := range queries {
		if err := d.Admin.QueryRow(ctx, q.sql).Scan(q.dest); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func assertClassicProjection(t *testing.T, d *dbtest.DB) {
	t.Helper()
	ctx := context.Background()
	directed := []struct{ typ, source, target string }{
		{"blocks", "PAI-21", "PAI-22"},
		{"blocks", "PAI-21", "PAI-24"},
		{"duplicates", "PAI-21", "PAI-24"},
		{"cites", "PAI-23", "PAI-22"},
	}
	for _, want := range directed {
		var n int
		if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM node_relations r JOIN nodes s ON s.tenant_id=r.tenant_id AND s.id=r.source_node_id JOIN nodes g ON g.tenant_id=r.tenant_id AND g.id=r.target_node_id WHERE r.type=$1 AND s.key=$2 AND g.key=$3`, want.typ, want.source, want.target).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("%s %s→%s count %d", want.typ, want.source, want.target, n)
		}
	}
	undirected := [][2]string{{"PAI-21", "PAI-23"}, {"PAI-21", "PAI-24"}, {"PAI-24", "PAI-23"}, {"PAI-24", "PAI-25"}, {"PAI-22", "PAI-25"}}
	for _, pair := range undirected {
		var n int
		if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM node_relations r JOIN nodes s ON s.tenant_id=r.tenant_id AND s.id=r.source_node_id JOIN nodes g ON g.tenant_id=r.tenant_id AND g.id=r.target_node_id WHERE r.type='relates' AND r.source_node_id < r.target_node_id AND ((s.key=$1 AND g.key=$2) OR (s.key=$2 AND g.key=$1))`, pair[0], pair[1]).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("relates %s–%s count %d", pair[0], pair[1], n)
		}
	}
	var total, reversed int
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM node_relations`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 9 {
		t.Fatalf("node_relations %d", total)
	}
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM node_relations WHERE type='relates' AND source_node_id >= target_node_id`).Scan(&reversed); err != nil {
		t.Fatal(err)
	}
	if reversed != 0 {
		t.Fatalf("unordered relates %d", reversed)
	}
	var parentKey string
	if err := d.Admin.QueryRow(ctx, `SELECT parent.key FROM nodes child JOIN nodes parent ON parent.id=child.parent_id WHERE child.key='PAI-21'`).Scan(&parentKey); err != nil {
		t.Fatal(err)
	}
	if parentKey != "PAI-22" {
		t.Fatalf("parent %s", parentKey)
	}
	var number int
	var state, releaseKey string
	var unreleased, noCurrent bool
	if err := d.Admin.QueryRow(ctx, `SELECT r.number, r.state, r.released_at IS NULL, j.current_release_node_id IS NULL, n.key FROM journey_releases r JOIN nodes n ON n.id=r.release_node_id JOIN journey_projects j ON j.tenant_id=r.tenant_id AND j.project_node_id=r.project_node_id WHERE n.key='PAI-20'`).Scan(&number, &state, &unreleased, &noCurrent, &releaseKey); err != nil {
		t.Fatal(err)
	}
	if number != 1 || state != "planning" || !unreleased || !noCurrent || releaseKey != "PAI-20" {
		t.Fatalf("release 20 number=%d state=%s unreleased=%v noCurrent=%v", number, state, unreleased, noCurrent)
	}
	if err := d.Admin.QueryRow(ctx, `SELECT r.number FROM journey_releases r JOIN nodes n ON n.id=r.release_node_id WHERE n.key='PAI-26'`).Scan(&number); err != nil {
		t.Fatal(err)
	}
	if number != 2 {
		t.Fatalf("release 26 number %d", number)
	}
	membership := map[string]string{"PAI-21": "PAI-20", "PAI-27": "PAI-26"}
	for ticket, release := range membership {
		var got string
		var source string
		if err := d.Admin.QueryRow(ctx, `SELECT rel.key, t.source FROM journey_tickets t JOIN nodes ticket ON ticket.id=t.ticket_node_id JOIN nodes rel ON rel.id=t.release_node_id WHERE ticket.key=$1`, ticket).Scan(&got, &source); err != nil {
			t.Fatalf("%s membership: %v", ticket, err)
		}
		if got != release || source != "manual" {
			t.Fatalf("%s -> %s source %s", ticket, got, source)
		}
	}
	var epicTickets int
	if err := d.Admin.QueryRow(ctx, `SELECT count(*) FROM journey_tickets t JOIN nodes n ON n.id=t.ticket_node_id WHERE n.key='PAI-22'`).Scan(&epicTickets); err != nil {
		t.Fatal(err)
	}
	if epicTickets != 0 {
		t.Fatal("epic stored as a journey ticket")
	}
}
