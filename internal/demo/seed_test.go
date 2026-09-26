// SPDX-License-Identifier: AGPL-3.0-only

package demo

import (
	"errors"
	"slices"
	"testing"

	"github.com/inspr-at/aeon/internal/auth"
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
	id, err := tenantbootstrap.Create(ctx, database.App, "lumen-demo", "Lumen Demo")
	if err != nil {
		t.Fatal(err)
	}
	before := seedRows(t, database, id)
	first, err := Seed(ctx, database.App, "lumen-demo")
	if err != nil {
		t.Fatal(err)
	}
	if first.Already || first.Projects != 3 || first.Tickets < 40 || first.Stage != "build" {
		t.Fatalf("summary %+v", first)
	}
	after := seedRows(t, database, first.TenantID)
	if slices.Equal(before, after) {
		t.Fatal("seed wrote no resources")
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
	if replay := seedRows(t, database, first.TenantID); !slices.Equal(after, replay) {
		t.Fatal("replay changed seeded nodes, keys, bindings, principals, or events")
	}
}

func TestDemoInterruptedRunRollsBackAndRetryConverges(t *testing.T) {
	t.Setenv("AEON_ENV", "dev")
	database := dbtest.Open(t)
	ctx := t.Context()
	tenantID, err := tenantbootstrap.Create(ctx, database.App, "retry-demo", "Retry Demo")
	if err != nil {
		t.Fatal(err)
	}
	baseline := seedRows(t, database, tenantID)
	_, err = seedWithHook(ctx, database.App, "retry-demo", func(step string) error {
		if step == "agents" {
			return errors.New("injected interruption")
		}
		return nil
	})
	if err == nil || err.Error() != "injected interruption" {
		t.Fatalf("expected injected interruption, got %v", err)
	}
	if partial := seedRows(t, database, tenantID); !slices.Equal(baseline, partial) {
		t.Fatal("interruption left seeded nodes, keys, bindings, principals, or events")
	}
	first, err := Seed(ctx, database.App, "retry-demo")
	if err != nil || first.Already {
		t.Fatalf("retry: summary %+v, error %v", first, err)
	}
	complete := seedRows(t, database, tenantID)
	second, err := Seed(ctx, database.App, "retry-demo")
	if err != nil || !second.Already {
		t.Fatalf("replay: summary %+v, error %v", second, err)
	}
	if replay := seedRows(t, database, tenantID); !slices.Equal(complete, replay) {
		t.Fatal("retry replay changed seeded resources")
	}
}

func TestDemoJourneyScopesOnlyExtendNewKey(t *testing.T) {
	t.Setenv("AEON_ENV", "dev")
	database := dbtest.Open(t)
	ctx := t.Context()
	tenantID, err := tenantbootstrap.Create(ctx, database.App, "keys-demo", "Keys Demo")
	if err != nil {
		t.Fatal(err)
	}
	oldKey, principalID, _, err := auth.OperatorCreateAgentKey(ctx, database.App, tenantID, "Lumen Scribe", "", []string{"nodes.read"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Seed(ctx, database.App, "keys-demo"); err != nil {
		t.Fatal(err)
	}
	var oldScopes, newScopes []string
	var newKey, eventKey string
	err = db.InTenant(dbtest.Seed(ctx), database.App, tenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT scopes FROM agent_keys WHERE id=$1::uuid`, oldKey).Scan(&oldScopes); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id::text,scopes FROM agent_keys WHERE principal_id=$1::uuid AND id<>$2::uuid`, principalID, oldKey).Scan(&newKey, &newScopes); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT after->>'key_id' FROM events WHERE type='agent_key.scopes_extended'`).Scan(&eventKey)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(oldScopes, []string{"nodes.read"}) || !slices.Contains(newScopes, "journey.requirements") || !slices.Contains(newScopes, "journey.build") || eventKey != newKey {
		t.Fatalf("old scopes %v, new scopes %v, new key %s, event key %s", oldScopes, newScopes, newKey, eventKey)
	}
}

// seedRows compares row contents, including IDs, keys, scopes, and event
// snapshots. Counts alone would miss changed or duplicated resources.
func seedRows(t *testing.T, database *dbtest.DB, tenantID string) []string {
	t.Helper()
	queries := []string{
		`SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.id),'[]'::jsonb)::text FROM identities t WHERE t.issuer='https://demo.aeon.invalid'`,
		`SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.id),'[]'::jsonb)::text FROM nodes t`,
		`SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.id),'[]'::jsonb)::text FROM agent_keys t`,
		`SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.id),'[]'::jsonb)::text FROM role_bindings t`,
		`SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.id),'[]'::jsonb)::text FROM roles t`,
		`SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.role_id,t.permission),'[]'::jsonb)::text FROM role_permissions t`,
		`SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.id),'[]'::jsonb)::text FROM principals t`,
		`SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY t.id),'[]'::jsonb)::text FROM events t`,
	}
	rows := make([]string, len(queries))
	err := db.InTenant(dbtest.Seed(t.Context()), database.App, tenantID, func(tx pgx.Tx) error {
		for i, query := range queries {
			if err := tx.QueryRow(t.Context(), query).Scan(&rows[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return rows
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
