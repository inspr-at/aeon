// SPDX-License-Identifier: AGPL-3.0-only

package stagehandoff

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestClassicBatchAliasBuiltEvidence(t *testing.T) {
	m, p, project, release, _ := fixture(t)
	var handoffID string
	err := db.InTenant(t.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `UPDATE journey_releases SET access_required=false WHERE release_node_id=$1::uuid`, release); err != nil {
			return err
		}
		plan, err := planDigest(t.Context(), tx, release)
		if err != nil {
			return err
		}
		contextDigest := digest(project, release, "deploy", "deploy", "1", "1", plan, emptyDigest)
		return tx.QueryRow(t.Context(), `INSERT INTO stage_handoffs
		 (tenant_id,project_node_id,release_node_id,stage,operation,plugin_id,requested_by_principal_id,idempotency_key,attempt,authority_epoch,journey_revision,plan_digest,predecessor_digest,context_digest,prerequisite_seal_sha256,evidence_ceiling,expires_at)
		 VALUES($1::uuid,$2::uuid,$3::uuid,'deploy','deploy','pharos',$4::uuid,'classic-fixture',1,1,1,$5,$6,$7,$6,ARRAY['deployment'],now()+interval '30 minutes') RETURNING id::text`,
			p.TenantID, project, release, p.ID, plan, emptyDigest, contextDigest).Scan(&handoffID)
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	m.Mount(mux)
	call := func(path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw)).WithContext(tenant.WithPrincipal(t.Context(), p))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	aliasPath := "/api/stage-handoffs/" + handoffID + "/classic-batch-alias"
	w := call(aliasPath, map[string]any{"classic_batch_id": 42, "classic_project_id": 17,
		"classic_batch": map[string]any{"id": 42, "project_id": 17, "progress": map[string]any{"next_action": "implementation_evidence"}}})
	if w.Code != http.StatusCreated {
		t.Fatalf("bind alias: %d %s", w.Code, w.Body.String())
	}
	receipt := classicBuiltReceipt{IdempotencyKey: "build:one", ExpectedAttemptID: 1, ExpectedPlanRevision: 1,
		Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OCIConfigDigest: "sha256:" + emptyDigest,
		ReleaseManifestDigest: emptyDigest, ReleaseManifestCoordinate: "oci:release-1",
		VersionScheme: "legacy", ReleaseChannel: "stable", ReleaseSequence: 1, Version: "1.0.0", QADigest: emptyDigest}
	path := "/api/projects/" + project + "/baseline-batches/batches/42/built-receipt"
	for i := 0; i < 2; i++ {
		w = call(path, receipt)
		if w.Code != http.StatusOK {
			t.Fatalf("built receipt try %d: %d %s", i, w.Code, w.Body.String())
		}
		var response struct {
			ID       int64 `json:"id"`
			Progress struct {
				NextAction string `json:"next_action"`
			} `json:"progress"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.ID != 42 || response.Progress.NextAction != "operator_external_stage_cli" {
			t.Fatalf("batch output: %+v %v", response, err)
		}
	}
	receipt.QADigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	w = call(path, receipt)
	if w.Code != http.StatusConflict {
		t.Fatalf("divergent replay: %d %s", w.Code, w.Body.String())
	}
	w = call("/api/projects/"+project+"/baseline-batches/batches/43/built-receipt", receipt)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown alias: %d %s", w.Code, w.Body.String())
	}
	var evidence, events int
	err = db.InTenant(t.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM stage_handoff_build_evidence WHERE handoff_id=$1::uuid`, handoffID).Scan(&evidence); err != nil {
			return err
		}
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type='stage_handoff.built_reported'`).Scan(&events)
	})
	if err != nil || evidence != 1 || events != 1 {
		t.Fatalf("evidence %d events %d err %v at %s", evidence, events, err, time.Now().UTC())
	}
}
