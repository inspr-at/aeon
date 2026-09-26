// SPDX-License-Identifier: AGPL-3.0-only

package activity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/dbtest"
)

// Authors say whether they have a picture, in the timeline and on a fresh
// comment, so the web client requests one only when it exists (U27).
func TestAuthorsSayWhetherTheyHaveAPicture(t *testing.T) {
	f := setup(t)
	pictured := f.p
	f.tx(func(tx pgx.Tx) error {
		if err := tx.QueryRow(t.Context(), `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1,'person','Pictured',ARRAY['member']) RETURNING id::text`, f.p.TenantID).Scan(&pictured.ID); err != nil {
			return err
		}
		if err := dbtest.BindLegacyTx(t.Context(), tx, f.p.TenantID, pictured.ID); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO personal_profiles(tenant_id,principal_id,avatar_original_hash,avatar_hashes) VALUES($1,$2,$3,$4::jsonb)`,
			f.p.TenantID, pictured.ID, strings.Repeat("a", 64), `{"32":"`+strings.Repeat("b", 64)+`"}`)
		return err
	})
	path := "/api/nodes/" + f.node + "/comments"
	var mine, theirs Item
	if err := json.Unmarshal(f.call(&f.p, "POST", path, `{"body_markdown":"No picture"}`, 201).Body.Bytes(), &mine); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(f.call(&pictured, "POST", path, `{"body_markdown":"With picture"}`, 201).Body.Bytes(), &theirs); err != nil {
		t.Fatal(err)
	}
	if mine.Author.HasAvatar || !theirs.Author.HasAvatar {
		t.Fatalf("fresh comments: %+v %+v", mine.Author, theirs.Author)
	}
	got := map[string]bool{}
	for _, item := range f.page("").Items {
		if item.Type == "comment" {
			got[item.Author.Name] = item.Author.HasAvatar
		}
	}
	if len(got) != 2 || got["Writer"] || !got["Pictured"] {
		t.Fatalf("timeline authors: %v", got)
	}
}
