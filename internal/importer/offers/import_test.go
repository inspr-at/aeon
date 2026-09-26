// SPDX-License-Identifier: AGPL-3.0-only
package offers

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/dbtest"
)

func syntheticBundle() Bundle {
	doc := json.RawMessage(`{"title":"Synthetic steel work","subtitle":"","project_ref":"TEST-1","offer_date":"2026-01-01","valid_until":"2026-12-31","sender":{"company":"Beispiel Stahl GmbH"},"customer":{"name":"Synthetic Buyer","address":"Testweg 1","contact":"Sample Person","country":"AT","customer_no":"K26011","email":"sample@example.invalid"},"intro":"Synthetic introduction","accept_text":"Synthetic acceptance","vat_note":"VAT is additional","footer":{"logo_width_mm":33,"logo_offset_mm":0},"blocks":[{"heading":"Payment terms","body":"Pay within 14 days","nodes":[{"kind":"item","text":"Pay within 14 days","marker":"decimal","numbering":"outline","section_bound":true}]}],"positions":[{"short_text":"Synthetic item","long_text":"Sample scope","quantity":1.25,"unit":"item","unit_price_cents":101,"total_cents":126}],"net_total_cents":126}`)
	return Bundle{Customers: []Customer{{ID: 17, Name: "Synthetic Buyer", CustomerNo: "K26011", UpdatedAt: "2026-01-01 09:00:00"}}, Contacts: []Contact{{ID: 29, CustomerID: 17, Name: "Sample Person", Email: "sample@example.invalid", IsPrimary: true, UpdatedAt: "2026-01-01 09:00:00"}}, Offers: []Offer{{ID: 41, CustomerID: 17, OfferNo: "A260101-01", Revision: 3, Status: "sent", SentAt: "2026-01-02T10:00:00Z", Document: doc}}, Settings: json.RawMessage(`{"sender":{},"defaults":{}}`), Branding: json.RawMessage(`{"name":"Synthetic"}`)}
}

func TestBundleDirectoryTarAndDryRun(t *testing.T) {
	b := syntheticBundle()
	dir := t.TempDir()
	files := map[string]any{"customer-17.json": b.Customers[0], "customer-17-contacts.json": b.Contacts, "customer-17-offers.json": []map[string]any{{"id": 41}}, "offer-41.json": b.Offers[0], "offer-41-document.json": b.Offers[0].Document, "offer-settings.json": json.RawMessage(b.Settings), "branding.json": json.RawMessage(b.Branding)}
	var tarBytes bytes.Buffer
	tw := tar.NewWriter(&tarBytes)
	for name, value := range files {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := tw.WriteHeader(&tar.Header{Name: "raw/" + name, Mode: 0600, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	fromDir, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fromTar, err := ReadTar(&tarBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(fromDir.Offers) != 1 || len(fromTar.Contacts) != 1 {
		t.Fatalf("lost bundle records: %+v %+v", fromDir, fromTar)
	}
	var out bytes.Buffer
	if err := RunCommand(context.Background(), []string{"--source-instance", "synthetic", "--bundle", dir}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"action":"plan"`) || strings.Contains(out.String(), "Sample Person") {
		t.Fatal("dry-run report missing mapping or leaked document body")
	}
	if _, err := ReadTar(strings.NewReader("not tar")); err == nil {
		t.Fatal("invalid tar accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "offer-41-document.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDir(dir); err == nil {
		t.Fatal("mismatched exported document accepted")
	}
}

func TestImportVersionAndIsolation(t *testing.T) {
	d := dbtest.Open(t)
	ctx := context.Background()
	tenantID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	otherID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	var adminID string
	for _, tid := range []string{tenantID, otherID} {
		err := db.InTenant(dbtest.Seed(ctx), d.App, tid, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO tenants(id,slug,name) VALUES($1::uuid,$2,$2)`, tid, tid); err != nil {
				return err
			}
			if tid == tenantID {
				if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,'person','Synthetic Admin',ARRAY['admin']) RETURNING id::text`, tid).Scan(&adminID); err != nil {
					return err
				}
			}
			for slug, prefix := range map[string]string{"organisation": "ORG", "contact": "CON", "quote": "QUO"} {
				if _, err := tx.Exec(ctx, `INSERT INTO node_kinds(tenant_id,slug,label,short_prefix,icon) VALUES($1::uuid,$2,$2,$3,$2)`, tid, slug, prefix); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	dbtest.BindLegacy(t, d, tenantID, adminID)
	var profileID string
	if err := db.InTenant(dbtest.Seed(ctx), d.App, tenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO quote_document_profiles(tenant_id,name,current_revision) VALUES($1::uuid,'Synthetic print',1) RETURNING id::text`, tenantID).Scan(&profileID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO quote_document_profile_revisions(tenant_id,profile_id,revision,name,definition,created_by_principal_id) VALUES($1::uuid,$2::uuid,1,'Synthetic print','{"schema":"inspr.document-profile.v1","layout_variant":"classic-v1"}'::jsonb,$3::uuid)`, tenantID, profileID, adminID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO quote_settings(tenant_id,revision,numbering_time_zone,default_currency,sender,defaults,layout,updated_by_principal_id,default_profile_id) VALUES($1::uuid,1,'Europe/Vienna','EUR','{}'::jsonb,'{}'::jsonb,'{}'::jsonb,$2::uuid,$3::uuid)`, tenantID, adminID, profileID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	b := syntheticBundle()
	report, err := Import(ctx, d.App, tenantID, adminID, "synthetic", b, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Mappings) != 3 || report.Mappings[2].Action != "create" {
		t.Fatalf("initial mapping: %+v", report)
	}
	quoteID := report.Mappings[2].NodeID
	if err := db.InTenant(dbtest.Seed(ctx), d.App, tenantID, func(tx pgx.Tx) error {
		var state, number, body, importedProfileID string
		var version int
		var total int64
		if err := tx.QueryRow(ctx, `SELECT q.state,q.offer_no,q.current_version,s.document->'sections'->0->'nodes'->0->>'text', (s.document->>'net_total_cents')::bigint,s.document->'profile'->>'id' FROM business_quotes q JOIN quote_version_snapshots s ON s.tenant_id=q.tenant_id AND s.quote_node_id=q.quote_node_id AND s.version=q.current_version WHERE q.quote_node_id=$1::uuid`, quoteID).Scan(&state, &number, &version, &body, &total, &importedProfileID); err != nil {
			return err
		}
		if state != "issued" || number != "A260101-01" || version != 1 || body != "Pay within 14 days" || total != 126 || importedProfileID != profileID {
			t.Fatalf("incorrect issued snapshot: %s %s %d %s %d", state, number, version, body, total)
		}
		var sequence int64
		if err := tx.QueryRow(ctx, `SELECT value FROM quote_number_sequences WHERE kind='offer' AND period='260101'`).Scan(&sequence); err != nil {
			return err
		}
		if sequence != 1 {
			t.Fatalf("number sequence %d", sequence)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	replay, err := Import(ctx, d.App, tenantID, adminID, "synthetic", b, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range replay.Mappings {
		if m.Action != "skip" {
			t.Fatalf("replay changed %s", m.SourceKind)
		}
	}
	b.Offers[0].Revision = 4
	newer, err := Import(ctx, d.App, tenantID, adminID, "synthetic", b, true)
	if err != nil {
		t.Fatal(err)
	}
	if newer.Mappings[2].Action != "update" || newer.Mappings[2].NodeID != quoteID {
		t.Fatalf("newer revision mapping: %+v", newer.Mappings[2])
	}
	if err := db.InTenant(dbtest.Seed(ctx), d.App, tenantID, func(tx pgx.Tx) error {
		var n, snapshots, issues, eventCount int
		if err := tx.QueryRow(ctx, `SELECT current_version FROM business_quotes WHERE quote_node_id=$1::uuid`, quoteID).Scan(&n); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM quote_version_snapshots WHERE quote_node_id=$1::uuid`, quoteID).Scan(&snapshots); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM quote_issues WHERE quote_node_id=$1::uuid`, quoteID).Scan(&issues); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM events WHERE node_id=$1::uuid`, quoteID).Scan(&eventCount); err != nil {
			return err
		}
		if n != 2 || snapshots != 2 || issues != 2 || eventCount < 4 {
			t.Fatalf("newer revision lost issued evidence: %d %d %d %d", n, snapshots, issues, eventCount)
		}
		var sequence int64
		if err := tx.QueryRow(ctx, `SELECT value FROM quote_number_sequences WHERE kind='customer' AND period='2601'`).Scan(&sequence); err != nil {
			return err
		}
		if sequence != 1 {
			t.Fatalf("customer sequence high water %d", sequence)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.InTenant(dbtest.Seed(ctx), d.App, otherID, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM paimos_offer_imports`).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatalf("cross-tenant mappings visible: %d", n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	b.Offers[0].Revision = 3
	stale, err := Import(ctx, d.App, tenantID, adminID, "synthetic", b, true)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Mappings[2].Action != "skip" {
		t.Fatal("stale revision wrote")
	}
	b.Offers[0].Revision = 4
	b.Offers[0].Document = json.RawMessage(strings.Replace(string(b.Offers[0].Document), "Synthetic steel work", "Changed without revision", 1))
	if _, err := Import(ctx, d.App, tenantID, adminID, "synthetic", b, true); err == nil {
		t.Fatal("same revision with changed source was accepted")
	}
	b = syntheticBundle()
	b.Offers = append(b.Offers, Offer{ID: 42, CustomerID: 17, OfferNo: "A260101-02", Revision: 1, Status: "draft", Document: b.Offers[0].Document})
	drafts, err := Import(ctx, d.App, tenantID, adminID, "synthetic", b, true)
	if err != nil {
		t.Fatal(err)
	}
	if drafts.Mappings[3].Action != "create" {
		t.Fatalf("draft mapping: %+v", drafts.Mappings[3])
	}
	if err := db.InTenant(dbtest.Seed(ctx), d.App, tenantID, func(tx pgx.Tx) error {
		var state string
		var version int
		if err := tx.QueryRow(ctx, `SELECT state,current_version FROM business_quotes WHERE quote_node_id=$1::uuid`, drafts.Mappings[3].NodeID).Scan(&state, &version); err != nil {
			return err
		}
		if state != "draft" || version != 0 {
			t.Fatalf("draft state/version: %s %d", state, version)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(ctx, d.App, tenantID, adminID, "second-instance", b, true); err == nil {
		t.Fatal("colliding imported customer number accepted")
	}
	if err := db.InTenant(dbtest.Seed(ctx), d.App, tenantID, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM paimos_offer_imports WHERE source_instance='second-instance'`).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatalf("failed import left %d mappings", n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
