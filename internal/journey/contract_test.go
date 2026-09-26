// SPDX-License-Identifier: AGPL-3.0-only

package journey_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/dbtest"
)

func TestJourneyKindIsLazyAndProjectionIsTenantScoped(t *testing.T) {
	ctx := context.Background()
	fresh := dbtest.Open(t)
	var tenantA, tenantB string
	if err := fresh.Admin.QueryRow(ctx, `INSERT INTO tenants(slug, name) VALUES ('journey-a', 'A') RETURNING id::text`).Scan(&tenantA); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Admin.QueryRow(ctx, `INSERT INTO tenants(slug, name) VALUES ('journey-b', 'B') RETURNING id::text`).Scan(&tenantB); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := fresh.Admin.QueryRow(ctx, `SELECT count(*) FROM node_kinds WHERE slug = 'requirement'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 0 {
		t.Fatalf("requirement kind seeded before journey initialization: %d", before)
	}

	err := db.InTenant(dbtest.Seed(ctx), fresh.App, tenantA, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT aeon_seed_requirement_kind($1::uuid)`, tenantA); err != nil {
			return err
		}
		var projectKind string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM node_kinds WHERE tenant_id = $1::uuid AND slug = 'project'`, tenantA).Scan(&projectKind); err != nil {
			return err
		}
		var projectID string
		if err := tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id, key, kind_id, title)
			VALUES ($1::uuid, 'JRN-1', $2::uuid, 'Journey') RETURNING id::text`, tenantA, projectKind).Scan(&projectID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO journey_projects(tenant_id, project_node_id)
			VALUES ($1::uuid, $2::uuid)`, tenantA, projectID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	var kindB int
	if err := fresh.Admin.QueryRow(ctx, `SELECT count(*) FROM node_kinds WHERE tenant_id = $1::uuid AND slug = 'requirement'`, tenantB).Scan(&kindB); err != nil {
		t.Fatal(err)
	}
	if kindB != 0 {
		t.Fatal("journey kind leaked into another tenant")
	}
	for _, tc := range []struct {
		tenant string
		want   int
	}{{tenantA, 1}, {tenantB, 0}} {
		err := db.InTenant(dbtest.Seed(ctx), fresh.App, tc.tenant, func(tx pgx.Tx) error {
			var got int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM journey_projects`).Scan(&got); err != nil {
				return err
			}
			if got != tc.want {
				t.Fatalf("journey rows for tenant %s: got %d, want %d", tc.tenant, got, tc.want)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
