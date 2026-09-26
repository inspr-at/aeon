// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/tenantbootstrap"
	"github.com/jackc/pgx/v5"
)

func TestQuoteProfileOperatorCommand(t *testing.T) {
	database := dbtest.Open(t)
	ctx := context.Background()
	tenantID, err := tenantbootstrap.Create(ctx, database.App, "synthetic", "Synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InTenant(dbtest.Seed(ctx), database.App, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO quote_settings(tenant_id,revision,numbering_time_zone,default_currency,sender,defaults,layout,updated_by_principal_id) SELECT $1::uuid,1,'Europe/Vienna','EUR','{}'::jsonb,'{}'::jsonb,'{}'::jsonb,id FROM principals WHERE kind='agent' AND name='Tenant bootstrap'`, tenantID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AEON_DATABASE_URL", database.URL)
	t.Setenv("AEON_FILES_DIR", t.TempDir())
	dir := t.TempDir()
	profile := []byte(`{"name":"Synthetic print","definition":{"schema":"inspr.document-profile.v1","layout_variant":"classic-v1","locale":"de-AT","fonts":[],"colors":{"ink":"#253335","muted":"#637477","soft":"#91a1a3","accent":"#287f78","rule":"#d5dfdf","paper":"#ffffff"},"typography":{"body_pt":"10"},"page":{"width_mm":"210","height_mm":"297","top_mm":"18","right_mm":"20","bottom_mm":"16","left_mm":"22"},"cover":{"top_mm":"11"},"sections":{"numbering":"upper-roman"},"positions_table":{"columns":[{"key":"position","width_mm":"9"},{"key":"description","width_mm":"71"},{"key":"quantity","width_mm":"15"},{"key":"unit","width_mm":"22"},{"key":"unit_price","width_mm":"24"},{"key":"total","width_mm":"27"}],"separator":"rule","repeat_header":true},"totals":{"vat":"note","discount":"hidden","net_label":"Net"},"payment_terms":{"position":"sections","heading":"Payment"},"acceptance":{"signature_columns":2,"gap_mm":"14","lead_mm":"28"},"footer":{"width_mm":"33","offset_mm":"0","page_number_format":"PAGE {page} OF {total}"},"labels":{"quote":"QUOTE"}}}`)
	if err := os.WriteFile(filepath.Join(dir, "profile.json"), profile, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := quoteProfileApply(ctx, []string{"--tenant", "synthetic", "--bundle", dir, "--default"}, bytes.NewReader(nil), &out); err != nil {
		t.Fatal(err)
	}
	var planned map[string]any
	if err := json.Unmarshal(out.Bytes(), &planned); err != nil || planned["action"] != "create" || planned["applied"] != false {
		t.Fatalf("dry run: %s: %v", out.String(), err)
	}
	var stream bytes.Buffer
	w := tar.NewWriter(&stream)
	if err := w.WriteHeader(&tar.Header{Name: "profile.json", Mode: 0600, Size: int64(len(profile))}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(profile); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := quoteProfileApply(ctx, []string{"--tenant", "synthetic", "--bundle", "-", "--default", "--apply"}, &stream, &out); err != nil {
		t.Fatal(err)
	}
	var applied map[string]any
	if err := json.Unmarshal(out.Bytes(), &applied); err != nil || applied["action"] != "create" || applied["applied"] != true || applied["profile_id"] == "" {
		t.Fatalf("apply: %s: %v", out.String(), err)
	}
}
