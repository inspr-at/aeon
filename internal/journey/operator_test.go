// SPDX-License-Identifier: AGPL-3.0-only

package journey_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/journey"
)

func TestDisposableOperatorSeedUsesJourneyActions(t *testing.T) {
	t.Setenv("AEON_ENV", "dev")
	f := newFixture(t)
	project := f.node(t, "project", "PRJ-361", "Disposable project")
	marked, err := journey.MarkDisposable(t.Context(), f.db.App, "journey-a", "PRJ-361")
	if err != nil || !marked.Disposable || marked.Already {
		t.Fatalf("mark: %+v %v", marked, err)
	}
	if again, err := journey.MarkDisposable(t.Context(), f.db.App, "journey-a", "PRJ-361"); err != nil || !again.Already {
		t.Fatalf("mark replay: %+v %v", again, err)
	}
	release := f.node(t, "release", "REL-361", "Release")
	ticket := f.node(t, "ticket", "TKT-361", "Completed ticket")
	ctx := t.Context()
	if err := db.InTenant(dbtest.Seed(ctx), f.db.App, f.tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO journey_projects(tenant_id,project_node_id,brief_confirmed_at,requirements_revision,agreed_requirements_revision,agreed_requirements_digest_sha256,current_release_node_id)
			VALUES($1::uuid,$2::uuid,now(),1,1,$4,$3::uuid)`, f.tenant, project, release, digest); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO journey_releases(tenant_id,release_node_id,project_node_id,number,state)
			VALUES($1::uuid,$2::uuid,$3::uuid,1,'planning')`, f.tenant, release, project); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO journey_tickets(tenant_id,ticket_node_id,project_node_id,release_node_id,walker_position,source)
			VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,0,'manual')`, f.tenant, ticket, project, release); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	view := f.journey(t, f.person, "GET", "/api/projects/"+project+"/journey", "")
	if !view.Disposable {
		t.Fatal("journey document omitted disposable marker")
	}
	if _, err := journey.SeedDisposable(ctx, f.db.App, "journey-a", "PRJ-361", "candidate"); err == nil || !strings.Contains(err.Error(), "wait_for_build") {
		t.Fatalf("open ticket allowed candidate seed: %v", err)
	}
	if err := db.InTenant(dbtest.Seed(ctx), f.db.App, f.tenant, func(tx pgx.Tx) error {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM journey_releases WHERE release_node_id=$1::uuid`, release).Scan(&state); err != nil {
			return err
		}
		if state != "planning" {
			t.Fatalf("blocked seed left partial state %s", state)
		}
		_, err := tx.Exec(ctx, `UPDATE nodes SET state='done' WHERE id=$1::uuid`, ticket)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := journey.SeedDisposable(ctx, f.db.App, "journey-a", "PRJ-361", "candidate")
	if err != nil || got.Already {
		t.Fatalf("seed candidate: %+v %v", got, err)
	}
	view = f.journey(t, f.person, "GET", "/api/projects/"+project+"/journey", "")
	if view.NextAction.Key != "approve_candidate" || view.NextAction.Available {
		t.Fatalf("candidate approval was bypassed: %+v", view)
	}
	buildStage := func() journey.JourneyStage {
		t.Helper()
		for _, stage := range view.Stages {
			if stage.Key == "build" {
				return stage
			}
		}
		t.Fatal("build stage missing")
		return journey.JourneyStage{}
	}
	if gate := buildStage(); gate.GateScope != journey.ScopeCandidate || gate.GateLive || gate.GateApprovalID != nil {
		t.Fatalf("candidate gate absent in journey document: %+v", gate)
	}
	var state, actor string
	var events int
	if err := db.InTenant(dbtest.Seed(ctx), f.db.App, f.tenant, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT state FROM journey_releases WHERE release_node_id=$1::uuid`, release).Scan(&state); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*),min(p.name) FROM events e JOIN principals p ON p.tenant_id=e.tenant_id AND p.id=e.actor_principal_id
			WHERE e.node_id=$1::uuid AND e.type LIKE 'journey.seed_%'`, project).Scan(&events, &actor)
	}); err != nil {
		t.Fatal(err)
	}
	if state != "candidate" || events != 2 || actor != "Access operator" {
		t.Fatalf("state=%s events=%d actor=%s", state, events, actor)
	}
	pending, err := journey.SeedDisposable(ctx, f.db.App, "journey-a", "PRJ-361", "deploy")
	if err != nil || pending.TargetReached || pending.PendingAction != "approve_candidate" {
		t.Fatalf("deploy crossed candidate gate: %+v %v", pending, err)
	}
	candidate := f.grant(t, f.agent.ID, f.person.ID, journey.ScopeCandidate, release)
	view = f.journey(t, f.person, "POST", "/api/projects/"+project+"/journey/actions", actionJSON("approve_candidate", view.Revision, "ds1-candidate", candidate, release, ""))
	if gate := buildStage(); gate.GateScope != journey.ScopeCandidate || !gate.GateLive || gate.GateApprovalID == nil || *gate.GateApprovalID != candidate {
		t.Fatalf("candidate gate not reported in journey document: %+v", gate)
	}
	pending, err = journey.SeedDisposable(ctx, f.db.App, "journey-a", "PRJ-361", "deploy")
	if err != nil || pending.TargetReached || pending.PendingAction != "approve_deploy" {
		t.Fatalf("deploy crossed deployment gate: %+v %v", pending, err)
	}
	deployment := f.grant(t, f.agent.ID, f.person.ID, journey.ScopeDeploy, release)
	view = f.journey(t, f.person, "POST", "/api/projects/"+project+"/journey/actions", actionJSON("approve_deploy", view.Revision, "ds1-deploy", deployment, release, ""))
	reached, err := journey.SeedDisposable(ctx, f.db.App, "journey-a", "PRJ-361", "deploy")
	if err != nil || !reached.TargetReached || reached.PendingAction != "" {
		t.Fatalf("live gate target: %+v %v", reached, err)
	}
	if again, err := journey.SeedDisposable(ctx, f.db.App, "journey-a", "PRJ-361", "candidate"); err != nil || !again.Already {
		t.Fatalf("seed replay: %+v %v", again, err)
	}
	var stdout bytes.Buffer
	if err := journey.RunOperator(ctx, f.db.App, []string{"seed", "--tenant", "journey-a", "--project", "PRJ-361", "--to-stage", "candidate"}, &stdout); err != nil || !strings.Contains(stdout.String(), `"disposable":true`) {
		t.Fatalf("operator command: %v %s", err, stdout.String())
	}
}

func TestDisposableOperatorGuards(t *testing.T) {
	t.Setenv("AEON_ENV", "dev")
	f := newFixture(t)
	project := f.node(t, "project", "PRJ-362", "Unmarked project")
	if _, err := journey.SeedDisposable(t.Context(), f.db.App, "journey-a", "PRJ-362", "build"); err == nil || !strings.Contains(err.Error(), "not disposable") {
		t.Fatalf("unmarked project seeded: %v", err)
	}
	var marks int
	if err := db.InTenant(dbtest.Seed(context.Background()), f.db.App, f.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM journey_disposable_projects WHERE project_node_id=$1::uuid`, project).Scan(&marks)
	}); err != nil || marks != 0 {
		t.Fatalf("unexpected marker: %d %v", marks, err)
	}
	t.Setenv("AEON_ENV", "prod")
	if _, err := journey.MarkDisposable(t.Context(), f.db.App, "journey-a", "PRJ-362"); err == nil {
		t.Fatal("production marker allowed")
	}
}
