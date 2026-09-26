// SPDX-License-Identifier: AGPL-3.0-only
package stagehandoff

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/paimos/internal/db"
	"github.com/inspr-at/paimos/internal/dbtest"
	"github.com/inspr-at/paimos/internal/tenant"
)

func launchHTTP(t *testing.T, w *launchWorld, p tenant.Principal, bearer, path, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	w.m.Mount(mux)
	method := http.MethodPost
	if body == "" {
		method = http.MethodGet
	}
	req := httptest.NewRequest(method, "/api/stage-handoffs/"+w.h.ID+path, bytes.NewBufferString(body)).WithContext(tenant.WithPrincipal(t.Context(), p))
	req.Header.Set("Authorization", bearer)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func launchEventCount(t *testing.T, w *launchWorld, eventType string) int {
	t.Helper()
	var count int
	err := db.InTenant(dbtest.Seed(t.Context()), w.m.pool, w.p.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type=$1`, eventType).Scan(&count)
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestLaunchExactReplayAndAdmissionRead(t *testing.T) {
	w := newLaunchWorld(t)
	// GET's routed-agent path also requires the operation's read scope.
	w.p.Scopes = []string{"stage.deploy"}
	err := db.InTenant(dbtest.Seed(t.Context()), w.m.pool, w.p.TenantID, func(tx pgx.Tx) error {
		var role string
		if err := tx.QueryRow(t.Context(), `INSERT INTO roles(tenant_id,key,name) VALUES($1::uuid,'launch_reader','Launch reader') RETURNING id::text`, w.p.TenantID).Scan(&role); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO role_permissions(tenant_id,role_id,permission) VALUES($1::uuid,$2::uuid,'stage.deploy'),($1::uuid,$2::uuid,'nodes.read')`, w.p.TenantID, role); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `UPDATE role_bindings SET role_id=$3::uuid WHERE tenant_id=$1::uuid AND principal_id=$2::uuid AND scope_type='workspace'`, w.p.TenantID, w.p.ID, role)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	w.storeArtifact(t, w.art.DigestSHA256)
	w.postReadiness(t, w.plan, true, true, true, time.Now(), w.h.AuthorityEpoch)
	base := "/launch/"
	admitKey := "11111111-1111-4111-8111-111111111111"
	consumeKey := "22222222-2222-4222-8222-222222222222"
	otherKey := "33333333-3333-4333-8333-333333333333"
	firstBody, err := json.Marshal(w.art)
	if err != nil {
		t.Fatal(err)
	}
	if rec := launchHTTP(t, w, w.p, w.bearer, base+"admit", "", string(firstBody)); rec.Code != 400 {
		t.Fatalf("missing idempotency key: %d %s", rec.Code, rec.Body.String())
	}
	first := launchHTTP(t, w, w.p, w.bearer, base+"admit", admitKey, string(firstBody))
	if first.Code != 200 {
		t.Fatalf("admit: %d %s", first.Code, first.Body.String())
	}
	var admission LaunchAdmission
	if err := json.Unmarshal(first.Body.Bytes(), &admission); err != nil {
		t.Fatal(err)
	}
	// The client can retry before writing its local journal. Sorted key order and
	// whitespace change the bytes, but not the canonical request digest.
	var fields map[string]any
	if err := json.Unmarshal(firstBody, &fields); err != nil {
		t.Fatal(err)
	}
	reordered, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	replayed := launchHTTP(t, w, w.p, w.bearer, base+"admit", admitKey, string(reordered))
	if replayed.Code != 200 || replayed.Body.String() != first.Body.String() {
		t.Fatalf("admit replay: %d %s", replayed.Code, replayed.Body.String())
	}
	if got := launchEventCount(t, w, "stage_handoff.launch_admitted"); got != 1 {
		t.Fatalf("admit events=%d", got)
	}
	read := launchHTTP(t, w, w.p, w.bearer, "", "", "")
	if read.Code != 200 {
		t.Fatalf("get: %d %s", read.Code, read.Body.String())
	}
	var handoff Handoff
	if err := json.Unmarshal(read.Body.Bytes(), &handoff); err != nil {
		t.Fatal(err)
	}
	if handoff.Admission == nil || handoff.Admission.AdmissionID != admission.ID || handoff.Admission.Epoch != admission.AuthorityEpoch || handoff.Admission.ConsumedAt != nil {
		t.Fatalf("admission state: %+v", handoff.Admission)
	}
	if !handoff.Admission.ExpiresAt.Equal(admission.ExpiresAt) || handoff.Admission.ExpiresAt.Format(time.RFC3339Nano) != admission.ExpiresAt.Format(time.RFC3339Nano) {
		t.Fatalf("admission expiry differs from GET: admitted=%s read=%s", admission.ExpiresAt.Format(time.RFC3339Nano), handoff.Admission.ExpiresAt.Format(time.RFC3339Nano))
	}
	if strings.Contains(read.Body.String(), "binding_digest_sha256") || strings.Contains(read.Body.String(), "artifact_digest_sha256") {
		t.Fatal("GET leaked launch artifact binding")
	}
	changed := w.art
	changed.Version = "different"
	changedBody, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if rec := launchHTTP(t, w, w.p, w.bearer, base+"admit", admitKey, string(changedBody)); rec.Code != 409 || !strings.Contains(rec.Body.String(), "idempotency_conflict") {
		t.Fatalf("changed admit body: %d %s", rec.Code, rec.Body.String())
	}
	if rec := launchHTTP(t, w, w.p, w.bearer, base+"admit", otherKey, string(firstBody)); rec.Code != 409 {
		t.Fatalf("second admit key: %d %s", rec.Code, rec.Body.String())
	}
	foreign, foreignBearer := scopedForeignAgent(t, w.m, w.p, w.release, "stage.deploy")
	routeFixtureAgent(t, w.m, &foreign, "pharos")
	if rec := launchHTTP(t, w, foreign, foreignBearer, base+"admit", admitKey, string(firstBody)); rec.Code != 409 || !strings.Contains(rec.Body.String(), "idempotency_conflict") {
		t.Fatalf("cross-principal replay: %d %s", rec.Code, rec.Body.String())
	}
	consumeBody := `{"admission_id":"` + admission.ID + `"}`
	consumed := launchHTTP(t, w, w.p, w.bearer, base+"consume", consumeKey, consumeBody)
	if consumed.Code != 200 {
		t.Fatalf("consume: %d %s", consumed.Code, consumed.Body.String())
	}
	var receipt LaunchConsumption
	if err := json.Unmarshal(consumed.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.HandoffID != w.h.ID || receipt.AdmissionID != admission.ID || !receipt.Consumed || receipt.ConsumedAt.IsZero() {
		t.Fatalf("receipt: %+v", receipt)
	}
	if rec := launchHTTP(t, w, w.p, w.bearer, base+"consume", consumeKey, consumeBody); rec.Code != 200 || rec.Body.String() != consumed.Body.String() {
		t.Fatalf("consume replay: %d %s", rec.Code, rec.Body.String())
	}
	if rec := launchHTTP(t, w, w.p, w.bearer, base+"consume", consumeKey, `{"admission_id":"00000000-0000-4000-8000-000000000000"}`); rec.Code != 409 || !strings.Contains(rec.Body.String(), "idempotency_conflict") {
		t.Fatalf("changed consume body: %d %s", rec.Code, rec.Body.String())
	}
	if rec := launchHTTP(t, w, foreign, foreignBearer, base+"consume", consumeKey, consumeBody); rec.Code != 409 || !strings.Contains(rec.Body.String(), "idempotency_conflict") {
		t.Fatalf("cross-principal consume replay: %d %s", rec.Code, rec.Body.String())
	}
	if rec := launchHTTP(t, w, w.p, w.bearer, base+"consume", otherKey, consumeBody); rec.Code != 409 {
		t.Fatalf("second consume key: %d %s", rec.Code, rec.Body.String())
	}
	if got := launchEventCount(t, w, "stage_handoff.launch_consumed"); got != 1 {
		t.Fatalf("consume events=%d", got)
	}
	read = launchHTTP(t, w, w.p, w.bearer, "", "", "")
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &handoff) != nil || handoff.Admission == nil || handoff.Admission.ConsumedAt == nil || handoff.Admission.ConsumedByPrincipalID == nil || *handoff.Admission.ConsumedByPrincipalID != w.p.ID {
		t.Fatalf("consumed admission state: %d %s", read.Code, read.Body.String())
	}
	if !handoff.Admission.ConsumedAt.Equal(receipt.ConsumedAt) || handoff.Admission.ConsumedAt.Format(time.RFC3339Nano) != receipt.ConsumedAt.Format(time.RFC3339Nano) {
		t.Fatalf("admission consumption differs from GET: consumed=%s read=%s", receipt.ConsumedAt.Format(time.RFC3339Nano), handoff.Admission.ConsumedAt.Format(time.RFC3339Nano))
	}
	// AEON-169 still fences exact replays after this principal loses routing.
	routeFixtureAgent(t, w.m, &w.p, "other")
	if rec := launchHTTP(t, w, w.p, w.bearer, base+"consume", consumeKey, consumeBody); rec.Code != 403 {
		t.Fatalf("unrouted replay: %d %s", rec.Code, rec.Body.String())
	}
	routeFixtureAgent(t, w.m, &w.p, "pharos")
	// A later terminal result does not erase the receipt. The injected clock
	// makes the 24-hour boundary deterministic without waiting on wall time.
	w.seq++
	evidence := EvidenceWrite{Sequence: w.seq, Kind: "deployment", Outcome: "succeeded", ObservedAt: time.Now(), AuthorityEpoch: w.h.AuthorityEpoch, Workflow: stringPtr("deploy"), Environment: stringPtr("production"), Artifact: &w.art}
	var result Result
	err = db.InTenant(dbtest.Seed(t.Context()), w.m.pool, w.p.TenantID, func(tx pgx.Tx) error {
		if _, err := w.m.appendEvidence(t.Context(), tx, w.p, w.bearer, w.h.ID, evidence); err != nil {
			return err
		}
		var err error
		result, err = w.m.close(t.Context(), tx, w.p, w.bearer, w.h.ID, ResultWrite{Outcome: "succeeded", TerminalSequence: w.seq, AuthorityEpoch: w.h.AuthorityEpoch, PrerequisiteSealSHA256: w.h.PrerequisiteSealSHA256})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec := launchHTTP(t, w, w.p, w.bearer, base+"consume", consumeKey, consumeBody); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"authority_open":false`) || !strings.Contains(rec.Body.String(), admission.ID) {
		t.Fatalf("terminal consume replay: %d %s", rec.Code, rec.Body.String())
	}
	if rec := launchHTTP(t, w, w.p, w.bearer, base+"admit", admitKey, string(firstBody)); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"authority_open":false`) || !strings.Contains(rec.Body.String(), admission.ID) {
		t.Fatalf("terminal admit replay: %d %s", rec.Code, rec.Body.String())
	}
	w.m.now = func() time.Time { return result.CompletedAt.Add(24*time.Hour + time.Second) }
	if rec := launchHTTP(t, w, w.p, w.bearer, base+"consume", consumeKey, consumeBody); rec.Code != 409 {
		t.Fatalf("expired consume replay: %d %s", rec.Code, rec.Body.String())
	}
	if rec := launchHTTP(t, w, w.p, w.bearer, base+"admit", admitKey, string(firstBody)); rec.Code != 409 {
		t.Fatalf("expired admit replay: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSupersededHandoffReadAndLaunchReplay(t *testing.T) {
	w := newLaunchWorld(t)
	w.p.Scopes = []string{"stage.deploy"}
	ctx := t.Context()
	if err := db.InTenant(dbtest.Seed(ctx), w.m.pool, w.p.TenantID, func(tx pgx.Tx) error {
		var role string
		if err := tx.QueryRow(ctx, `INSERT INTO roles(tenant_id,key,name) VALUES($1::uuid,'supersession_reader','Supersession reader') RETURNING id::text`, w.p.TenantID).Scan(&role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions(tenant_id,role_id,permission) VALUES($1::uuid,$2::uuid,'stage.deploy'),($1::uuid,$2::uuid,'nodes.read')`, w.p.TenantID, role); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE role_bindings SET role_id=$3::uuid WHERE tenant_id=$1::uuid AND principal_id=$2::uuid AND scope_type='workspace'`, w.p.TenantID, w.p.ID, role)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	w.storeArtifact(t, w.art.DigestSHA256)
	w.postReadiness(t, w.plan, true, true, true, time.Now(), w.h.AuthorityEpoch)
	admitKey := "44444444-4444-4444-8444-444444444444"
	consumeKey := "55555555-5555-4555-8555-555555555555"
	artifactBody, err := json.Marshal(w.art)
	if err != nil {
		t.Fatal(err)
	}
	admitted := launchHTTP(t, w, w.p, w.bearer, "/launch/admit", admitKey, string(artifactBody))
	if admitted.Code != 200 {
		t.Fatalf("admit: %d %s", admitted.Code, admitted.Body.String())
	}
	var admission LaunchAdmission
	if err := json.Unmarshal(admitted.Body.Bytes(), &admission); err != nil || !admission.AuthorityOpen {
		t.Fatalf("admission: %+v %v", admission, err)
	}
	consumeBody := `{"admission_id":"` + admission.ID + `"}`
	consumed := launchHTTP(t, w, w.p, w.bearer, "/launch/consume", consumeKey, consumeBody)
	if consumed.Code != 200 {
		t.Fatalf("consume: %d %s", consumed.Code, consumed.Body.String())
	}
	var successor Handoff
	if err := db.InTenant(dbtest.Seed(ctx), w.m.pool, w.p.TenantID, func(tx pgx.Tx) error {
		var err error
		successor, err = w.m.create(ctx, tx, w.p, RequestWrite{ProjectNodeID: w.project, ReleaseNodeID: w.release, Stage: "deploy", Operation: "deploy", ExpectedJourneyRevision: 1, IdempotencyKey: "superseding-attempt"}, "pharos", []string{"deployment", "launch_readiness"}, "deploy")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if successor.Attempt != w.h.Attempt+1 || successor.AuthorityEpoch != w.h.AuthorityEpoch+1 {
		t.Fatalf("successor: %+v", successor)
	}
	read := launchHTTP(t, w, w.p, w.bearer, "", "", "")
	var stale Handoff
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &stale) != nil || stale.Attempt != w.h.Attempt || stale.SupersededBy == nil || *stale.SupersededBy != successor.ID || stale.AuthorityOpen {
		t.Fatalf("stale read: %d %s", read.Code, read.Body.String())
	}
	for _, tc := range []struct{ path, key, body string }{
		{"/launch/admit", admitKey, string(artifactBody)},
		{"/launch/consume", consumeKey, consumeBody},
	} {
		replay := launchHTTP(t, w, w.p, w.bearer, tc.path, tc.key, tc.body)
		if replay.Code != 200 || !strings.Contains(replay.Body.String(), `"authority_open":false`) || !strings.Contains(replay.Body.String(), admission.ID) {
			t.Fatalf("stale replay %s: %d %s", tc.path, replay.Code, replay.Body.String())
		}
		if rec := launchHTTP(t, w, w.p, w.bearer, tc.path, "66666666-6666-4666-8666-666666666666", tc.body); rec.Code != 409 {
			t.Fatalf("new stale write %s: %d %s", tc.path, rec.Code, rec.Body.String())
		}
	}
	if err := db.InTenant(dbtest.Seed(ctx), w.m.pool, w.p.TenantID, func(tx pgx.Tx) error {
		_, err := w.m.appendEvidence(ctx, tx, w.p, w.bearer, w.h.ID, w.readiness(w.seq+1, w.plan, true, true, true, time.Now(), w.h.AuthorityEpoch))
		return err
	}); err == nil || !strings.Contains(err.Error(), "handoff is stale") {
		t.Fatalf("new evidence on stale handoff: %v", err)
	}
	if got := launchEventCount(t, w, "stage_handoff.launch_consumed"); got != 1 {
		t.Fatalf("consume events after replay=%d", got)
	}
}
