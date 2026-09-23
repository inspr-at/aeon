// SPDX-License-Identifier: AGPL-3.0-only
package stagehandoff

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/plugins/janus"
	"github.com/inspr-at/aeon/internal/plugins/pharos"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

const emptyDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func fixture(t *testing.T) (*Module, tenant.Principal, string, string, string) {
	t.Helper()
	ctx := context.Background()
	fresh := dbtest.Open(t)
	var tenantID string
	if err := fresh.Admin.QueryRow(ctx, `INSERT INTO tenants(slug,name) VALUES('p34','P34') RETURNING id::text`).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	registry := plugins.NewRegistry()
	if err := janus.Register(registry); err != nil {
		t.Fatal(err)
	}
	if err := pharos.Register(registry); err != nil {
		t.Fatal(err)
	}
	m := &Module{pool: fresh.App, registry: registry, launchChecks: testLaunchChecks{}}
	var p tenant.Principal
	var project, release string
	err := db.InTenant(ctx, fresh.App, tenantID, func(tx pgx.Tx) error {
		p.TenantID = tenantID
		p.Kind = tenant.Agent
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'agent','Worker') RETURNING id::text`, tenantID).Scan(&p.ID); err != nil {
			return err
		}
		var human string
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,'person','Human',ARRAY['admin']) RETURNING id::text`, tenantID).Scan(&human); err != nil {
			return err
		}
		var projectKind, releaseKind string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM node_kinds WHERE slug='project'`).Scan(&projectKind); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id::text FROM node_kinds WHERE slug='release'`).Scan(&releaseKind); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title) VALUES($1::uuid,'P34-1',$2::uuid,'Project') RETURNING id::text`, tenantID, projectKind).Scan(&project); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO journey_projects(tenant_id,project_node_id) VALUES($1::uuid,$2::uuid)`, tenantID, project); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title,parent_id) VALUES($1::uuid,'P34-2',$2::uuid,'Release',$3::uuid) RETURNING id::text`, tenantID, releaseKind, project).Scan(&release); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO journey_releases(tenant_id,release_node_id,project_node_id,number,state,access_required) VALUES($1::uuid,$2::uuid,$3::uuid,1,'candidate',true)`, tenantID, release, project); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE journey_projects SET current_release_node_id=$1::uuid WHERE project_node_id=$2::uuid`, release, project); err != nil {
			return err
		}
		man := janus.Manifest()
		if _, err := tx.Exec(ctx, `INSERT INTO plugin_installations(tenant_id,plugin_id,version,manifest_digest_sha256,owner,enabled,permissions,updated_by_principal_id) VALUES($1::uuid,$2,$3,$4,$5,true,$6,$7::uuid)`, tenantID, man.ID, man.Version, man.DigestSHA256, man.Owner, man.Permissions, human); err != nil {
			return err
		}
		secret := "fixture-secret"
		sum := sha256.Sum256([]byte(secret))
		prefix := "fixture"
		if _, err := tx.Exec(ctx, `INSERT INTO agent_keys(tenant_id,principal_id,name,prefix,hash,scopes) VALUES($1::uuid,$2::uuid,'test',$3,$4,ARRAY['stage.prepare','stage.deploy','stage.verify','stage.apply'])`, tenantID, p.ID, prefix, hex.EncodeToString(sum[:])); err != nil {
			return err
		}
		var approval string
		if err := tx.QueryRow(ctx, `INSERT INTO approval_requests(tenant_id,proposed_by_principal_id,agent_principal_id,scope,resource_kind,resource_id,rationale,expires_at) VALUES($1::uuid,$2::uuid,$2::uuid,'stage.prepare','node',$3::uuid,'Prepare access',now()+interval '1 hour') RETURNING id::text`, tenantID, p.ID, release).Scan(&approval); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO approval_decisions(tenant_id,request_id,decided_by_principal_id,decision) VALUES($1::uuid,$2::uuid,$3::uuid,'approved')`, tenantID, approval, human); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO agent_permission_grants(tenant_id,approval_request_id,agent_principal_id,scope,resource_kind,resource_id,valid_until) SELECT tenant_id,id,agent_principal_id,scope,resource_kind,resource_id,expires_at FROM approval_requests WHERE id=$1::uuid`, approval)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m, p, project, release, "Bearer aeon_fixture_fixture-secret"
}
func TestPrepareHandoffFencingAndEvidence(t *testing.T) {
	m, p, project, release, bearer := fixture(t)
	ctx := context.Background()
	in := RequestWrite{ProjectNodeID: project, ReleaseNodeID: release, Stage: "access", Operation: "prepare", ExpectedJourneyRevision: 1, IdempotencyKey: "prepare-1"}
	var h Handoff
	err := db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		h, err = m.create(ctx, tx, p, in, "janus", []string{"authorization", "credential_handoff"}, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if h.Attempt != 1 || h.AuthorityEpoch != 1 || h.PrerequisiteSealSHA256 == "" {
		t.Fatalf("bad handoff: %+v", h)
	}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		same, err := m.create(ctx, tx, p, in, "janus", []string{"authorization", "credential_handoff"}, "")
		if err != nil {
			return err
		}
		if same.ID != h.ID {
			t.Fatal("idempotency replay made a new request")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	e1 := EvidenceWrite{Sequence: 1, Kind: "authorization", Outcome: "satisfied", ObservedAt: time.Now().UTC().Truncate(time.Microsecond), AuthorityEpoch: h.AuthorityEpoch, Authorized: &yes}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error { _, err := m.appendEvidence(ctx, tx, p, bearer, h.ID, e1); return err })
	if err != nil {
		t.Fatal(err)
	}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error { _, err := m.appendEvidence(ctx, tx, p, bearer, h.ID, e1); return err })
	if err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	divergent := e1
	no := false
	divergent.Authorized = &no
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error { _, err := m.appendEvidence(ctx, tx, p, bearer, h.ID, divergent); return err })
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.code != 409 {
		t.Fatalf("divergent replay: %v", err)
	}
	e2 := EvidenceWrite{Sequence: 2, Kind: "credential_handoff", Outcome: "satisfied", ObservedAt: time.Now().UTC().Truncate(time.Microsecond), AuthorityEpoch: h.AuthorityEpoch, CredentialReady: &yes}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error { _, err := m.appendEvidence(ctx, tx, p, bearer, h.ID, e2); return err })
	if err != nil {
		t.Fatal(err)
	}
	r := ResultWrite{Outcome: "succeeded", TerminalSequence: 2, AuthorityEpoch: h.AuthorityEpoch, PrerequisiteSealSHA256: h.PrerequisiteSealSHA256}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error { _, err := m.close(ctx, tx, p, bearer, h.ID, r); return err })
	if err != nil {
		t.Fatal(err)
	}
	var eventCount int
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM events WHERE type LIKE 'stage_handoff.%'`).Scan(&eventCount)
	})
	if err != nil {
		t.Fatal(err)
	}
	if eventCount != 4 {
		t.Fatalf("events=%d, want 4", eventCount)
	}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM stage_handoffs WHERE id=$1::uuid`, h.ID).Scan(&state); err != nil {
			return err
		}
		if state != "succeeded" {
			t.Fatalf("handoff state=%s", state)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestRegistryRejectsInvalidManifests(t *testing.T) {
	r := plugins.NewRegistry()
	m := janus.Manifest()
	if err := r.Register(m); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(m); err == nil {
		t.Fatal("duplicate accepted")
	}
	m.ID = "tampered"
	if err := plugins.NewRegistry().Register(m); err == nil {
		t.Fatal("digest mismatch accepted")
	}
}
func TestPharosAdmissionAndTenantFence(t *testing.T) {
	m, p, project, release, bearer := fixture(t)
	ctx := context.Background()
	err := db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var human string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM principals WHERE kind='person' LIMIT 1`).Scan(&human); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE journey_releases SET version_scheme='inspr-calendar-v2',version='260923000000.0.0' WHERE release_node_id=$1::uuid`, release); err != nil {
			return err
		}
		man := pharos.Manifest()
		if _, err := tx.Exec(ctx, `INSERT INTO plugin_installations(tenant_id,plugin_id,version,manifest_digest_sha256,owner,enabled,permissions,updated_by_principal_id) VALUES($1::uuid,$2,$3,$4,$5,true,$6,$7::uuid)`, p.TenantID, man.ID, man.Version, man.DigestSHA256, man.Owner, man.Permissions, human); err != nil {
			return err
		}
		for _, gate := range []string{"candidate", "deploy", "access"} {
			var approval string
			scope := "stage." + gate
			if gate == "access" {
				scope = "stage.apply"
			}
			if err := tx.QueryRow(ctx, `INSERT INTO approval_requests(tenant_id,proposed_by_principal_id,agent_principal_id,scope,resource_kind,resource_id,rationale,expires_at) VALUES($1::uuid,$2::uuid,$2::uuid,$3,'node',$4::uuid,'Reviewed',now()+interval '1 hour') RETURNING id::text`, p.TenantID, p.ID, scope, release).Scan(&approval); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO approval_decisions(tenant_id,request_id,decided_by_principal_id,decision) VALUES($1::uuid,$2::uuid,$3::uuid,'approved')`, p.TenantID, approval, human); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO agent_permission_grants(tenant_id,approval_request_id,agent_principal_id,scope,resource_kind,resource_id,valid_until) SELECT tenant_id,id,agent_principal_id,scope,resource_kind,resource_id,expires_at FROM approval_requests WHERE id=$1::uuid`, approval); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO journey_gates(tenant_id,project_node_id,release_node_id,gate,approval_request_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5::uuid)`, p.TenantID, project, release, gate, approval); err != nil {
				return err
			}
		}
		var verifyApproval string
		if err := tx.QueryRow(ctx, `INSERT INTO approval_requests(tenant_id,proposed_by_principal_id,agent_principal_id,scope,resource_kind,resource_id,rationale,expires_at) VALUES($1::uuid,$2::uuid,$2::uuid,'stage.verify','node',$3::uuid,'Verify',now()+interval '1 hour') RETURNING id::text`, p.TenantID, p.ID, release).Scan(&verifyApproval); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO approval_decisions(tenant_id,request_id,decided_by_principal_id,decision) VALUES($1::uuid,$2::uuid,$3::uuid,'approved')`, p.TenantID, verifyApproval, human); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO agent_permission_grants(tenant_id,approval_request_id,agent_principal_id,scope,resource_kind,resource_id,valid_until) SELECT tenant_id,id,agent_principal_id,scope,resource_kind,resource_id,expires_at FROM approval_requests WHERE id=$1::uuid`, verifyApproval); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	prepareRequest := RequestWrite{ProjectNodeID: project, ReleaseNodeID: release, Stage: "access", Operation: "prepare", ExpectedJourneyRevision: 1, IdempotencyKey: "prepare-before-deploy"}
	var prepare Handoff
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		prepare, err = m.create(ctx, tx, p, prepareRequest, "janus", []string{"authorization", "credential_handoff"}, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	for _, e := range []EvidenceWrite{{Sequence: 1, Kind: "authorization", Outcome: "satisfied", ObservedAt: time.Now().UTC(), AuthorityEpoch: prepare.AuthorityEpoch, Authorized: &yes}, {Sequence: 2, Kind: "credential_handoff", Outcome: "satisfied", ObservedAt: time.Now().UTC(), AuthorityEpoch: prepare.AuthorityEpoch, CredentialReady: &yes}} {
		err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error { _, err := m.appendEvidence(ctx, tx, p, bearer, prepare.ID, e); return err })
		if err != nil {
			t.Fatal(err)
		}
	}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		_, err := m.close(ctx, tx, p, bearer, prepare.ID, ResultWrite{Outcome: "succeeded", TerminalSequence: 2, AuthorityEpoch: prepare.AuthorityEpoch, PrerequisiteSealSHA256: prepare.PrerequisiteSealSHA256})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	in := RequestWrite{ProjectNodeID: project, ReleaseNodeID: release, Stage: "deploy", Operation: "deploy", ExpectedJourneyRevision: 1, IdempotencyKey: "deploy-1"}
	var h Handoff
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		h, err = m.create(ctx, tx, p, in, "pharos", []string{"deployment"}, "deploy")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	a := Artifact{VersionScheme: "inspr-calendar-v2", Version: "260923000000.0.0", ReleaseChannel: "stable", ReleaseSequence: 1, DigestSHA256: emptyDigest, CommitDigest: "commit", ManifestCoordinate: "manifest", ManifestDigestSHA256: emptyDigest}
	e := EvidenceWrite{Sequence: 1, Kind: "deployment", Outcome: "succeeded", ObservedAt: time.Now().UTC(), AuthorityEpoch: h.AuthorityEpoch, Workflow: stringPtr("deploy"), Environment: stringPtr("production"), Artifact: &a}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error { _, err := m.appendEvidence(ctx, tx, p, bearer, h.ID, e); return err })
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.code != 409 {
		t.Fatalf("launch without admission: %v", err)
	}
	m.launchChecks = nil
	if _, err := m.AdmitLaunch(ctx, p, bearer, h.ID, a); !errors.As(err, &apiErr) || apiErr.code != 409 {
		t.Fatalf("missing readiness provider: %v", err)
	}
	m.launchChecks = testLaunchChecks{}
	admission, err := m.AdmitLaunch(ctx, p, bearer, h.ID, a)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ConsumeLaunch(ctx, p, bearer, admission.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.ConsumeLaunch(ctx, p, bearer, admission.ID); !errors.As(err, &apiErr) || apiErr.code != 409 {
		t.Fatalf("double consume: %v", err)
	}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error { _, err := m.appendEvidence(ctx, tx, p, bearer, h.ID, e); return err })
	if err != nil {
		t.Fatal(err)
	}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		_, err := m.close(ctx, tx, p, bearer, h.ID, ResultWrite{Outcome: "succeeded", TerminalSequence: 1, AuthorityEpoch: h.AuthorityEpoch, PrerequisiteSealSHA256: h.PrerequisiteSealSHA256})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	verifyRequest := RequestWrite{ProjectNodeID: project, ReleaseNodeID: release, Stage: "deploy", Operation: "verify", ExpectedJourneyRevision: 2, IdempotencyKey: "verify-1"}
	var verification Handoff
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		verification, err = m.create(ctx, tx, p, verifyRequest, "pharos", []string{"verification"}, "deploy")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	verificationEvidence := EvidenceWrite{Sequence: 1, Kind: "verification", Outcome: "succeeded", ObservedAt: time.Now().UTC(), AuthorityEpoch: verification.AuthorityEpoch, Workflow: stringPtr("verify"), Environment: stringPtr("production"), Artifact: &a}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		_, err := m.appendEvidence(ctx, tx, p, bearer, verification.ID, verificationEvidence)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		_, err := m.close(ctx, tx, p, bearer, verification.ID, ResultWrite{Outcome: "succeeded", TerminalSequence: 1, AuthorityEpoch: verification.AuthorityEpoch, PrerequisiteSealSHA256: verification.PrerequisiteSealSHA256})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var state string
		var revision int64
		if err := tx.QueryRow(ctx, `SELECT state FROM journey_releases WHERE release_node_id=$1::uuid`, release).Scan(&state); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT revision FROM journey_projects WHERE project_node_id=$1::uuid`, project).Scan(&revision); err != nil {
			return err
		}
		if state != "access" || revision != 3 {
			t.Fatalf("release=%s journey revision=%d", state, revision)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	applyRequest := RequestWrite{ProjectNodeID: project, ReleaseNodeID: release, Stage: "access", Operation: "apply", ExpectedJourneyRevision: 3, IdempotencyKey: "apply-1"}
	var apply Handoff
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		apply, err = m.create(ctx, tx, p, applyRequest, "janus", []string{"authorization", "credential_handoff"}, "access")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []EvidenceWrite{{Sequence: 1, Kind: "authorization", Outcome: "satisfied", ObservedAt: time.Now().UTC(), AuthorityEpoch: apply.AuthorityEpoch, Authorized: &yes}, {Sequence: 2, Kind: "credential_handoff", Outcome: "satisfied", ObservedAt: time.Now().UTC(), AuthorityEpoch: apply.AuthorityEpoch, CredentialReady: &yes}} {
		err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error { _, err := m.appendEvidence(ctx, tx, p, bearer, apply.ID, e); return err })
		if err != nil {
			t.Fatal(err)
		}
	}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		_, err := m.close(ctx, tx, p, bearer, apply.ID, ResultWrite{Outcome: "succeeded", TerminalSequence: 2, AuthorityEpoch: apply.AuthorityEpoch, PrerequisiteSealSHA256: apply.PrerequisiteSealSHA256})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM journey_releases WHERE release_node_id=$1::uuid`, release).Scan(&state); err != nil {
			return err
		}
		if state != "released" {
			t.Fatalf("final release state=%s", state)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		replayed, err := m.create(ctx, tx, p, in, "pharos", []string{"deployment"}, "deploy")
		if err != nil {
			return err
		}
		if replayed.ID != h.ID {
			t.Fatal("completed request replay created another handoff")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	otherTenant := "00000000-0000-0000-0000-000000000001"
	err = db.InTenant(ctx, m.pool, otherTenant, func(tx pgx.Tx) error { _, err := loadHandoff(ctx, tx, h.ID, false); return err })
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant handoff visible: %v", err)
	}
}
func stringPtr(s string) *string { return &s }

type testLaunchChecks struct{}

func (testLaunchChecks) CheckLaunch(_ context.Context, _, _, _ string, a Artifact) (LaunchReadiness, error) {
	return LaunchReadiness{ReviewedArtifactDigestSHA256: a.DigestSHA256, BackupReady: true, HostReady: true, ObservedAt: time.Now()}, nil
}
func TestNewAttemptRevokesOldAuthority(t *testing.T) {
	m, p, project, release, bearer := fixture(t)
	ctx := context.Background()
	request := RequestWrite{ProjectNodeID: project, ReleaseNodeID: release, Stage: "access", Operation: "prepare", ExpectedJourneyRevision: 1, IdempotencyKey: "first"}
	var first, second Handoff
	err := db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		first, err = m.create(ctx, tx, p, request, "janus", []string{"authorization", "credential_handoff"}, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	request.IdempotencyKey = "second"
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		second, err = m.create(ctx, tx, p, request, "janus", []string{"authorization", "credential_handoff"}, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.AuthorityEpoch <= first.AuthorityEpoch {
		t.Fatal("authority did not advance")
	}
	yes := true
	e := EvidenceWrite{Sequence: 1, Kind: "authorization", Outcome: "satisfied", ObservedAt: time.Now().UTC(), AuthorityEpoch: first.AuthorityEpoch, Authorized: &yes}
	err = db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error { _, err := m.appendEvidence(ctx, tx, p, bearer, first.ID, e); return err })
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.code != 409 {
		t.Fatalf("old reporter was not fenced: %v", err)
	}
}
