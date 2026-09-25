// SPDX-License-Identifier: AGPL-3.0-only
package quotes

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/importer/offers"
	"github.com/jackc/pgx/v5"
)

func TestProfileBundleApplyAndDraftAssignment(t *testing.T) {
	database := dbtest.Open(t)
	ctx := context.Background()
	tenantID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	var adminID string
	if err := db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO tenants(id,slug,name) VALUES($1::uuid,'synthetic','Synthetic')`, tenantID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,'person','Synthetic Admin',ARRAY['admin']) RETURNING id::text`, tenantID).Scan(&adminID); err != nil {
			return err
		}
		for slug, prefix := range map[string]string{"organisation": "ORG", "contact": "CON", "quote": "QUO"} {
			if _, err := tx.Exec(ctx, `INSERT INTO node_kinds(tenant_id,slug,label,short_prefix,icon) VALUES($1::uuid,$2,$2,$3,$2)`, tenantID, slug, prefix); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sourceDoc := json.RawMessage(`{"title":"Synthetic service","subtitle":"","project_ref":"","offer_date":"2026-01-01","valid_until":"2026-12-31","sender":{"company":"Synthetic"},"customer":{"name":"Synthetic Buyer","country":"AT","customer_no":"K26011"},"intro":"Synthetic","accept_text":"Synthetic","vat_note":"VAT","footer":{"logo_width_mm":33,"logo_offset_mm":0},"blocks":[],"positions":[{"short_text":"Service","long_text":"","quantity":1,"unit":"item","unit_price_cents":100,"total_cents":100}],"net_total_cents":100}`)
	importBundle := offers.Bundle{
		Customers: []offers.Customer{{ID: 17, Name: "Synthetic Buyer", CustomerNo: "K26011", UpdatedAt: "2026-01-01 09:00:00"}},
		Contacts:  []offers.Contact{{ID: 29, CustomerID: 17, Name: "Synthetic Person", Email: "synthetic@example.invalid", UpdatedAt: "2026-01-01 09:00:00"}},
		Offers: []offers.Offer{
			{ID: 41, CustomerID: 17, OfferNo: "A260101-01", Revision: 1, Status: "draft", Document: sourceDoc},
			{ID: 42, CustomerID: 17, OfferNo: "A260101-02", Revision: 1, Status: "sent", SentAt: "2026-01-02T10:00:00Z", Document: sourceDoc},
		},
		Settings: json.RawMessage(`{"sender":{},"defaults":{}}`), Branding: json.RawMessage(`{"name":"Synthetic"}`),
	}
	imported, err := offers.Import(ctx, database.App, tenantID, adminID, "classic-test", importBundle, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO quote_settings(tenant_id,revision,numbering_time_zone,default_currency,sender,defaults,layout,updated_by_principal_id) VALUES($1::uuid,1,'Europe/Vienna','EUR','{}'::jsonb,'{}'::jsonb,'{}'::jsonb,$2::uuid)`, tenantID, adminID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var draftID, issuedID string
	for _, mapping := range imported.Mappings {
		if mapping.SourceKind != "offer" {
			continue
		}
		if mapping.SourceID == "41" {
			draftID = mapping.NodeID
		} else {
			issuedID = mapping.NodeID
		}
	}
	if draftID == "" || issuedID == "" {
		t.Fatal("imported offer mapping missing")
	}

	definition := syntheticProfile()
	definition.Footer.AssetID = "assets/mark.svg"
	profile, err := json.Marshal(profileWrite{Name: "Synthetic print", Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0L10 10" fill="#123456"/></svg>`)
	bundle := ProfileBundle{Profile: profile, Files: map[string][]byte{"assets/mark.svg": svg}}
	filesDir := t.TempDir()
	plan, err := ApplyProfileBundle(ctx, database.App, tenantID, adminID, filesDir, "classic-test", bundle, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != "create" || plan.AssetsCreated != 1 || plan.DraftsChanged != 1 || !plan.DefaultChange || plan.Applied {
		t.Fatalf("dry-run report: %+v", plan)
	}
	if entries, err := os.ReadDir(filesDir); err != nil || len(entries) != 0 {
		t.Fatal("dry-run wrote files")
	}
	apply, err := ApplyProfileBundle(ctx, database.App, tenantID, adminID, filesDir, "classic-test", bundle, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if apply.Action != "create" || apply.ProfileID == "" || apply.DraftsChanged != 1 {
		t.Fatalf("apply report: %+v", apply)
	}
	replay, err := ApplyProfileBundle(ctx, database.App, tenantID, adminID, filesDir, "classic-test", bundle, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Action != "unchanged" || replay.AssetsCreated != 0 || replay.DraftsChanged != 0 || replay.DefaultChange || replay.Revision != 1 {
		t.Fatalf("replay changed state: %+v", replay)
	}
	if err := db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,'agent','Renamed bootstrap',ARRAY['operator']::text[])`, tenantID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,'agent','Tenant bootstrap','{}'::text[])`, tenantID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	roleReplay, err := ApplyProfileBundle(ctx, database.App, tenantID, "", filesDir, "classic-test", bundle, true, true)
	if err != nil || roleReplay.Action != "unchanged" {
		t.Fatalf("operator role lookup: %+v: %v", roleReplay, err)
	}
	if err := db.InTenant(ctx, database.App, tenantID, func(tx pgx.Tx) error {
		var defaultID, selectedID string
		var draftRevision int64
		if err := tx.QueryRow(ctx, `SELECT default_profile_id::text FROM quote_settings`).Scan(&defaultID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT draft_revision,document->'profile'->>'id' FROM quote_drafts WHERE quote_node_id=$1::uuid`, draftID).Scan(&draftRevision, &selectedID); err != nil {
			return err
		}
		var issuedProfile any
		if err := tx.QueryRow(ctx, `SELECT document->'profile' FROM quote_version_snapshots WHERE quote_node_id=$1::uuid`, issuedID).Scan(&issuedProfile); err != nil {
			return err
		}
		var saves, uploads, selects int
		if err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE type='quote.profile_saved'), count(*) FILTER (WHERE type='quote.profile_asset_uploaded'), count(*) FILTER (WHERE type='quote.profile_selected') FROM events`).Scan(&saves, &uploads, &selects); err != nil {
			return err
		}
		if defaultID != apply.ProfileID || selectedID != apply.ProfileID || draftRevision != 2 || issuedProfile != nil || saves != 1 || uploads != 1 || selects != 1 {
			t.Fatalf("profile state: default=%s selected=%s draft=%d issued=%v events=%d/%d/%d", defaultID, selectedID, draftRevision, issuedProfile, saves, uploads, selects)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	definition.Colors["accent"] = "#558877"
	bundle.Profile, _ = json.Marshal(profileWrite{Name: "Synthetic print", Definition: definition})
	updated, err := ApplyProfileBundle(ctx, database.App, tenantID, adminID, filesDir, "classic-test", bundle, true, true)
	if err != nil || updated.Action != "update" || updated.Revision != 2 || updated.DraftsChanged != 1 {
		t.Fatalf("update report: %+v: %v", updated, err)
	}
}

func TestProfileBundleReadersRejectUnsafeEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(`{"name":"Synthetic"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("profile.json", filepath.Join(dir, "linked.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadProfileBundleDir(dir); err == nil {
		t.Fatal("directory symlink accepted")
	}
	var stream bytes.Buffer
	w := tar.NewWriter(&stream)
	if err := w.WriteHeader(&tar.Header{Name: "../escape", Mode: 0600, Size: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadProfileBundleTar(strings.NewReader(stream.String())); err == nil {
		t.Fatal("tar traversal accepted")
	}
}
