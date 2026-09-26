// SPDX-License-Identifier: AGPL-3.0-only

package stagehandoff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestLaunchBindingCanonical(t *testing.T) {
	h := Handoff{
		ID:             "11111111-1111-4111-8111-111111111111",
		ReleaseNodeID:  "22222222-2222-4222-8222-222222222222",
		AuthorityEpoch: 7,
	}
	artifact := strings.Repeat("ab", 32)
	plan := strings.Repeat("cd", 32)
	canonical := `{"artifact_digest_sha256":"` + artifact + `","authority_epoch":7,"handoff_id":"` + h.ID + `","release_node_id":"` + h.ReleaseNodeID + `","reviewed_plan_digest":"` + plan + `"}`
	sum := sha256.Sum256(append(append([]byte(launchBindingDomain), 0), []byte(canonical)...))
	if got := launchBinding(h, artifact, plan); got != hex.EncodeToString(sum[:]) {
		t.Fatalf("binding %s", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(canonical), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["authority_epoch"] != float64(7) {
		t.Fatalf("authority_epoch %#v", decoded["authority_epoch"])
	}
}

func TestLaunchReadinessFieldValidation(t *testing.T) {
	yes := true
	no := false
	good := EvidenceWrite{
		Sequence: 1, Kind: "launch_readiness", Outcome: "satisfied",
		ObservedAt: time.Now(), AuthorityEpoch: 1,
		ReviewedPlanDigest: strings.Repeat("ab", 32), Host: "edge-1",
		AllHostEvalPassed: &yes, TargetBuildPassed: &yes, BackupReady: &yes,
		BackupObservedAt: time.Now(), RestartRequired: &no,
		RunningKernel: "6.8.0", ExpectedKernel: "6.8.0",
	}
	cases := []struct {
		name string
		edit func(*EvidenceWrite)
	}{
		{"uppercase digest", func(e *EvidenceWrite) { e.ReviewedPlanDigest = strings.ToUpper(e.ReviewedPlanDigest) }},
		{"short digest", func(e *EvidenceWrite) { e.ReviewedPlanDigest = "abcd" }},
		{"missing host flag", func(e *EvidenceWrite) { e.AllHostEvalPassed = nil }},
		{"missing build flag", func(e *EvidenceWrite) { e.TargetBuildPassed = nil }},
		{"missing backup flag", func(e *EvidenceWrite) { e.BackupReady = nil }},
		{"missing restart", func(e *EvidenceWrite) { e.RestartRequired = nil }},
		{"empty host", func(e *EvidenceWrite) { e.Host = " " }},
		{"empty kernel", func(e *EvidenceWrite) { e.RunningKernel = "" }},
		{"artifact mixed in", func(e *EvidenceWrite) {
			e.Artifact = &Artifact{DigestSHA256: strings.Repeat("ab", 32)}
		}},
	}
	if err := validateEvidence(good); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := good
			tc.edit(&e)
			if err := validateEvidence(e); err == nil {
				t.Fatal("accepted invalid launch readiness")
			}
		})
	}
	deploy := EvidenceWrite{
		Sequence: 1, Kind: "deployment", Outcome: "succeeded", ObservedAt: time.Now(), AuthorityEpoch: 1,
		Workflow: stringPtr("deploy"), Environment: stringPtr("production"),
		Artifact: &Artifact{VersionScheme: "inspr-calendar-v2", Version: "1", ReleaseChannel: "stable", ReleaseSequence: 1, DigestSHA256: emptyDigest, CommitDigest: "commit", ManifestCoordinate: "aeon:stable", ManifestDigestSHA256: emptyDigest},
		Host:     "edge-1",
	}
	if err := validateEvidence(deploy); err == nil {
		t.Fatal("deployment evidence accepted launch fields")
	}
}

func TestEvidenceLaunchAdmission(t *testing.T) {
	otherPlan := strings.Repeat("ef", 32)
	otherDigest := strings.Repeat("11", 32)
	cases := []struct {
		name    string
		prepare func(t *testing.T, w *launchWorld)
		want    string
	}{
		{name: "missing readiness", want: "missing"},
		{name: "stale readiness", want: "stale", prepare: func(t *testing.T, w *launchWorld) {
			w.insertReadiness(t, w.plan, true, true, true, time.Now().Add(-LaunchFreshness-time.Second), w.h.AuthorityEpoch)
		}},
		{name: "all_host_eval_passed false", want: "all_host_eval_passed", prepare: func(t *testing.T, w *launchWorld) {
			w.postReadiness(t, w.plan, false, true, true, time.Now(), w.h.AuthorityEpoch)
		}},
		{name: "target_build_passed false", want: "target_build_passed", prepare: func(t *testing.T, w *launchWorld) {
			w.postReadiness(t, w.plan, true, false, true, time.Now(), w.h.AuthorityEpoch)
		}},
		{name: "backup_ready false", want: "backup_ready", prepare: func(t *testing.T, w *launchWorld) {
			w.postReadiness(t, w.plan, true, true, false, time.Now(), w.h.AuthorityEpoch)
		}},
		{name: "contradicting later record", want: "contradict", prepare: func(t *testing.T, w *launchWorld) {
			w.postReadiness(t, w.plan, true, true, true, time.Now(), w.h.AuthorityEpoch)
			w.postReadiness(t, otherPlan, true, true, true, time.Now(), w.h.AuthorityEpoch)
		}},
		{name: "wrong artifact", want: "does not match the approved candidate", prepare: func(t *testing.T, w *launchWorld) {
			w.postReadiness(t, w.plan, true, true, true, time.Now(), w.h.AuthorityEpoch)
		}},
		{name: "wrong epoch", want: "authority_epoch", prepare: func(t *testing.T, w *launchWorld) {
			w.insertReadiness(t, w.plan, true, true, true, time.Now(), w.h.AuthorityEpoch+1)
		}},
		{name: "happy path", prepare: func(t *testing.T, w *launchWorld) {
			w.postReadinessHTTP(t, w.plan, true, true, true, time.Now(), w.h.AuthorityEpoch)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newLaunchWorld(t)
			digest := w.art.DigestSHA256
			if tc.name == "wrong artifact" {
				digest = otherDigest
			}
			w.storeArtifact(t, digest)
			if tc.prepare != nil {
				tc.prepare(t, w)
			}
			admission, err := w.m.AdmitLaunch(context.Background(), w.p, w.bearer, w.h.ID, w.art)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				want := launchBinding(w.h, w.art.DigestSHA256, w.plan)
				if admission.BindingDigestSHA256 != want || admission.ArtifactDigestSHA256 != w.art.DigestSHA256 || admission.AuthorityEpoch != w.h.AuthorityEpoch {
					t.Fatalf("admission %+v", admission)
				}
				return
			}
			var apiErr *apiError
			if !errors.As(err, &apiErr) || apiErr.code != http.StatusConflict || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestLaunchBindingDriftAndConcurrentConsume(t *testing.T) {
	w := newLaunchWorld(t)
	w.storeArtifact(t, w.art.DigestSHA256)
	w.postReadiness(t, w.plan, true, true, true, time.Now(), w.h.AuthorityEpoch)
	ctx := context.Background()
	admission, err := w.m.AdmitLaunch(ctx, w.p, w.bearer, w.h.ID, w.art)
	if err != nil {
		t.Fatal(err)
	}
	w.postReadiness(t, strings.Repeat("ef", 32), true, true, true, time.Now(), w.h.AuthorityEpoch)
	err = w.m.ConsumeLaunch(ctx, w.p, w.bearer, w.h.ID, admission.ID)
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.code != http.StatusConflict || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("drift: %v", err)
	}

	fresh := newLaunchWorld(t)
	fresh.storeArtifact(t, fresh.art.DigestSHA256)
	fresh.postReadiness(t, fresh.plan, true, true, true, time.Now(), fresh.h.AuthorityEpoch)
	admission, err = fresh.m.AdmitLaunch(ctx, fresh.p, fresh.bearer, fresh.h.ID, fresh.art)
	if err != nil {
		t.Fatal(err)
	}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = fresh.m.ConsumeLaunch(ctx, fresh.p, fresh.bearer, fresh.h.ID, admission.ID)
		}(i)
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, consumeErr := range errs {
		if consumeErr == nil {
			wins++
			continue
		}
		if !errors.As(consumeErr, &apiErr) || apiErr.code != http.StatusConflict {
			t.Fatalf("concurrent consume: %v", consumeErr)
		}
	}
	if wins != 1 {
		t.Fatalf("wins=%d", wins)
	}
}

type launchWorld struct {
	m       *Module
	p       tenant.Principal
	project string
	release string
	bearer  string
	h       Handoff
	art     Artifact
	plan    string
	seq     int64
}

func newLaunchWorld(t *testing.T) *launchWorld {
	t.Helper()
	m, p, project, release, bearer := fixture(t)
	m.launchChecks = EvidenceLaunchChecks{Pool: m.pool}
	w := &launchWorld{
		m: m, p: p, project: project, release: release, bearer: bearer,
		art: Artifact{
			VersionScheme: "inspr-calendar-v2", Version: "260926000000.0.0", ReleaseChannel: "stable", ReleaseSequence: 1,
			DigestSHA256: emptyDigest, CommitDigest: "0123456789abcdef0123456789abcdef01234567",
			ManifestCoordinate: "aeon:stable", ManifestDigestSHA256: strings.Repeat("ab", 32),
		},
		plan: strings.Repeat("cd", 32),
	}
	ctx := context.Background()
	err := db.InTenant(dbtest.Seed(ctx), m.pool, p.TenantID, func(tx pgx.Tx) error {
		var human string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM principals WHERE kind='person' LIMIT 1`).Scan(&human); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE journey_releases SET access_required=false, version_scheme='inspr-calendar-v2', version='260926000000.0.0' WHERE release_node_id=$1::uuid`, release); err != nil {
			return err
		}
		plug, _ := m.registry.Lookup("pharos")
		man := plug.Manifest
		if _, err := tx.Exec(ctx, `INSERT INTO plugin_installations(tenant_id,plugin_id,version,manifest_digest_sha256,owner,enabled,permissions,updated_by_principal_id) VALUES($1::uuid,$2,$3,$4,$5,true,$6,$7::uuid)`, p.TenantID, man.ID, man.Version, man.DigestSHA256, man.Owner, man.Permissions, human); err != nil {
			return err
		}
		for _, gate := range []string{"candidate", "deploy"} {
			var approval string
			if err := tx.QueryRow(ctx, `INSERT INTO approval_requests(tenant_id,proposed_by_principal_id,agent_principal_id,scope,resource_kind,resource_id,rationale,expires_at) VALUES($1::uuid,$2::uuid,$2::uuid,$3,'node',$4::uuid,'Reviewed',now()+interval '1 hour') RETURNING id::text`, p.TenantID, p.ID, "stage."+gate, release).Scan(&approval); err != nil {
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
		var deployGrant string
		if err := tx.QueryRow(ctx, `INSERT INTO approval_requests(tenant_id,proposed_by_principal_id,agent_principal_id,scope,resource_kind,resource_id,rationale,expires_at) VALUES($1::uuid,$2::uuid,$2::uuid,'stage.deploy','node',$3::uuid,'Deploy',now()+interval '1 hour') RETURNING id::text`, p.TenantID, p.ID, release).Scan(&deployGrant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO approval_decisions(tenant_id,request_id,decided_by_principal_id,decision) VALUES($1::uuid,$2::uuid,$3::uuid,'approved')`, p.TenantID, deployGrant, human); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO agent_permission_grants(tenant_id,approval_request_id,agent_principal_id,scope,resource_kind,resource_id,valid_until) SELECT tenant_id,id,agent_principal_id,scope,resource_kind,resource_id,expires_at FROM approval_requests WHERE id=$1::uuid`, deployGrant); err != nil {
			return err
		}
		h, err := m.create(ctx, tx, p, RequestWrite{ProjectNodeID: project, ReleaseNodeID: release, Stage: "deploy", Operation: "deploy", ExpectedJourneyRevision: 1, IdempotencyKey: "launch-admit"}, "pharos", []string{"deployment", "launch_readiness"}, "deploy")
		if err != nil {
			return err
		}
		w.h = h
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	routeFixtureAgent(t, m, &w.p, "pharos")
	return w
}

func (w *launchWorld) storeArtifact(t *testing.T, digest string) {
	t.Helper()
	ctx := context.Background()
	err := db.InTenant(dbtest.Seed(ctx), w.m.pool, w.p.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO stage_handoff_build_evidence(tenant_id,handoff_id,idempotency_key,receipt,commit_digest,oci_config_digest,release_manifest_digest,release_manifest_coordinate,oci_index_digest,version_scheme,release_channel,release_sequence,version,qa_digest) VALUES($1::uuid,$2::uuid,'built-receipt','{}'::jsonb,$3,$4,$5,$6,'',$7,$8,$9,$10,$11)`, w.p.TenantID, w.h.ID, w.art.CommitDigest, digest, w.art.ManifestDigestSHA256, w.art.ManifestCoordinate, w.art.VersionScheme, w.art.ReleaseChannel, w.art.ReleaseSequence, w.art.Version, emptyDigest)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (w *launchWorld) postReadiness(t *testing.T, plan string, hostEval, target, backup bool, observed time.Time, epoch int64) {
	t.Helper()
	w.seq++
	e := w.readiness(w.seq, plan, hostEval, target, backup, observed, epoch)
	ctx := context.Background()
	err := db.InTenant(dbtest.Seed(ctx), w.m.pool, w.p.TenantID, func(tx pgx.Tx) error {
		_, err := w.m.appendEvidence(ctx, tx, w.p, w.bearer, w.h.ID, e)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (w *launchWorld) postReadinessHTTP(t *testing.T, plan string, hostEval, target, backup bool, observed time.Time, epoch int64) {
	t.Helper()
	w.seq++
	raw, err := json.Marshal(w.readiness(w.seq, plan, hostEval, target, backup, observed, epoch))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	w.m.Mount(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/stage-handoffs/"+w.h.ID+"/evidence", bytes.NewReader(raw)).WithContext(tenant.WithPrincipal(context.Background(), w.p))
	req.Header.Set("Authorization", w.bearer)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("post readiness: %d %s", rec.Code, rec.Body.String())
	}
}

func (w *launchWorld) insertReadiness(t *testing.T, plan string, hostEval, target, backup bool, observed time.Time, epoch int64) {
	t.Helper()
	w.seq++
	ctx := context.Background()
	err := db.InTenant(dbtest.Seed(ctx), w.m.pool, w.p.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO stage_handoff_evidence(tenant_id,handoff_id,sequence,authority_epoch,kind,outcome,observed_at,reviewed_plan_digest,host,all_host_eval_passed,target_build_passed,backup_ready,backup_observed_at,restart_required,running_kernel,expected_kernel) VALUES($1::uuid,$2::uuid,$3,$4,'launch_readiness','satisfied',$5,$6,'edge-1',$7,$8,$9,$5,false,'6.8.0','6.8.0')`, w.p.TenantID, w.h.ID, w.seq, epoch, observed, plan, hostEval, target, backup)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (w *launchWorld) readiness(seq int64, plan string, hostEval, target, backup bool, observed time.Time, epoch int64) EvidenceWrite {
	restart := false
	return EvidenceWrite{
		Sequence: seq, Kind: "launch_readiness", Outcome: "satisfied", ObservedAt: observed, AuthorityEpoch: epoch,
		ReviewedPlanDigest: plan, Host: "edge-1",
		AllHostEvalPassed: boolPtr(hostEval), TargetBuildPassed: boolPtr(target), BackupReady: boolPtr(backup),
		BackupObservedAt: observed, RestartRequired: &restart, RunningKernel: "6.8.0", ExpectedKernel: "6.8.0",
	}
}

func boolPtr(v bool) *bool { return &v }
