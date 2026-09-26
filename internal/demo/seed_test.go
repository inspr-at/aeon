// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/tenantbootstrap"
	"github.com/jackc/pgx/v5"
)

func TestDemoSeedRefusesOutsideDev(t *testing.T) {
	t.Setenv("AEON_ENV", "prod")
	if _, err := Seed(t.Context(), nil, "lumen-demo"); err == nil {
		t.Fatal("prod seed was allowed")
	}
	t.Setenv("AEON_ENV", "")
	if err := Run(t.Context(), nil, []string{"seed", "--tenant", "lumen-demo"}, nil); err == nil {
		t.Fatal("unset AEON_ENV was allowed")
	}
}

func TestDemoSeedTwice(t *testing.T) {
	t.Setenv("AEON_ENV", "dev")
	database := dbtest.Open(t)
	ctx := t.Context()
	if _, err := tenantbootstrap.Create(ctx, database.App, "lumen-demo", "Lumen Demo"); err != nil {
		t.Fatal(err)
	}
	first, err := Seed(ctx, database.App, "lumen-demo")
	if err != nil {
		t.Fatal(err)
	}
	if first.Already || first.Projects != 3 || first.Tickets < 40 || first.Stage != "build" {
		t.Fatalf("summary %+v", first)
	}
	events := scalar(t, database, first.TenantID, `SELECT count(*) FROM events`)
	tickets := scalar(t, database, first.TenantID, `SELECT count(*) FROM nodes n JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id WHERE k.slug='ticket' AND n.deleted_at IS NULL`)
	kinds := scalar(t, database, first.TenantID, `SELECT count(DISTINCT k.slug) FROM nodes n JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id WHERE n.deleted_at IS NULL AND k.slug IN ('runbook','guideline','memory','external_system','related_project')`)
	knowledge := scalar(t, database, first.TenantID, `SELECT count(*) FROM nodes n JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id WHERE n.deleted_at IS NULL AND k.slug IN ('runbook','guideline','memory','external_system','related_project')`)
	agents := scalar(t, database, first.TenantID, `SELECT count(*) FROM principals WHERE kind='agent' AND name IN ('Lumen Scribe','Harbor Clerk')`)
	sessions := scalar(t, database, first.TenantID, `SELECT count(*) FROM harness_sessions`)
	finished := scalar(t, database, first.TenantID, `SELECT count(*) FROM agent_runs WHERE status='completed' AND started_at IS NOT NULL AND ended_at IS NOT NULL`)
	pending := scalar(t, database, first.TenantID, `SELECT count(*) FROM approval_requests a WHERE NOT EXISTS (SELECT 1 FROM approval_decisions d WHERE d.tenant_id=a.tenant_id AND d.request_id=a.id)`)
	hours := scalar(t, database, first.TenantID, `SELECT count(*) FROM time_entries`)
	rates := scalar(t, database, first.TenantID, `SELECT count(*) FROM cost_unit_rates WHERE internal_amount=80.00 AND bill_amount=140.00 AND currency='EUR'`)
	if kinds != 5 || knowledge < 8 || agents != 2 || sessions < 1 || finished < 1 || pending < 1 || hours < 3 || rates != 1 || tickets < 40 {
		t.Fatalf("kinds %d knowledge %d agents %d sessions %d finished %d pending %d hours %d rates %d tickets %d", kinds, knowledge, agents, sessions, finished, pending, hours, rates, tickets)
	}
	second, err := Seed(ctx, database.App, "lumen-demo")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Already || second.Projects != first.Projects || second.Tickets != first.Tickets || second.Stage != "build" {
		t.Fatalf("second summary %+v", second)
	}
	if again := scalar(t, database, first.TenantID, `SELECT count(*) FROM events`); again != events {
		t.Fatalf("second seed wrote events: %d to %d", events, again)
	}
	if again := scalar(t, database, first.TenantID, `SELECT count(*) FROM nodes n JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id WHERE k.slug='ticket' AND n.deleted_at IS NULL`); again != tickets {
		t.Fatalf("tickets changed %d to %d", tickets, again)
	}
}

func scalar(t *testing.T, database *dbtest.DB, tenantID, query string) int {
	t.Helper()
	var n int
	err := db.InTenant(dbtest.Seed(t.Context()), database.App, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), query).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
