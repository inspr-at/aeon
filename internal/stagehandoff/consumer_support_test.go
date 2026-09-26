// SPDX-License-Identifier: AGPL-3.0-only
package stagehandoff

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestClosedLaunchChecksRefuse(t *testing.T) {
	_, err := (ClosedLaunchChecks{}).CheckLaunch(context.Background(), "", "", "", Artifact{})
	if err == nil {
		t.Fatal("closed launch checks allowed an admission")
	}
}

func TestHandoffReadScopeAndSeal(t *testing.T) {
	m, owner, project, release, bearer := fixture(t)
	ctx := context.Background()
	var handoffID, seal string
	pharos := tenant.Principal{TenantID: owner.TenantID, Kind: tenant.Agent, Name: "pharos", Scopes: []string{"stage.deploy"}}
	other := tenant.Principal{TenantID: owner.TenantID, Kind: tenant.Agent, Name: "janus", Scopes: []string{"stage.deploy"}}
	unscoped := tenant.Principal{TenantID: owner.TenantID, Kind: tenant.Agent, Name: "pharos"}
	reader := tenant.Principal{TenantID: owner.TenantID, Kind: tenant.Person, Name: "Reader"}
	err := db.InTenant(dbtest.Seed(ctx), m.pool, owner.TenantID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'agent','pharos') RETURNING id::text`, owner.TenantID).Scan(&pharos.ID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'agent','janus') RETURNING id::text`, owner.TenantID).Scan(&other.ID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'agent','pharos') RETURNING id::text`, owner.TenantID).Scan(&unscoped.ID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'person','Reader') RETURNING id::text`, owner.TenantID).Scan(&reader.ID); err != nil {
			return err
		}
		var role, see string
		if err := tx.QueryRow(ctx, `INSERT INTO roles(tenant_id,key,name) VALUES($1::uuid,'deploy_only','Deploy only') RETURNING id::text`, owner.TenantID).Scan(&role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions(tenant_id,role_id,permission) VALUES($1::uuid,$2::uuid,'stage.deploy'),($1::uuid,$2::uuid,'nodes.read')`, owner.TenantID, role); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO roles(tenant_id,key,name) VALUES($1::uuid,'see_only','See only') RETURNING id::text`, owner.TenantID).Scan(&see); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions(tenant_id,role_id,permission) VALUES($1::uuid,$2::uuid,'nodes.read')`, owner.TenantID, see); err != nil {
			return err
		}
		for _, id := range []string{pharos.ID, other.ID} {
			if _, err := tx.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) VALUES($1::uuid,$2::uuid,$3::uuid,'workspace')`, owner.TenantID, id, role); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) VALUES($1::uuid,$2::uuid,$3::uuid,'workspace')`, owner.TenantID, unscoped.ID, see); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) SELECT $1::uuid,$2::uuid,id,'workspace' FROM roles WHERE tenant_id=$1::uuid AND key='member'`, owner.TenantID, reader.ID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO stage_handoffs
			(tenant_id,project_node_id,release_node_id,stage,operation,plugin_id,requested_by_principal_id,idempotency_key,attempt,authority_epoch,journey_revision,plan_digest,predecessor_digest,context_digest,prerequisite_seal_sha256,evidence_ceiling,expires_at)
			VALUES($1::uuid,$2::uuid,$3::uuid,'deploy','deploy','pharos',$4::uuid,'read-scope',1,1,1,$5,$5,$5,$5,ARRAY['deployment'],now()+interval '30 minutes')
			RETURNING id::text, prerequisite_seal_sha256`, owner.TenantID, project, release, owner.ID, emptyDigest).Scan(&handoffID, &seal)
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	m.Mount(mux)
	get := func(p tenant.Principal) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/api/stage-handoffs/"+handoffID, nil).WithContext(tenant.WithPrincipal(ctx, p))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	ok := get(pharos)
	if ok.Code != http.StatusOK {
		t.Fatalf("routed principal: %d %s", ok.Code, ok.Body.String())
	}
	var body Handoff
	if err := json.Unmarshal(ok.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.PrerequisiteSealSHA256 != seal || body.PrerequisiteSealSHA256 == "" {
		t.Fatalf("seal %q want %q", body.PrerequisiteSealSHA256, seal)
	}
	if got := get(other); got.Code != http.StatusForbidden {
		t.Fatalf("other principal with op scope: %d %s", got.Code, got.Body.String())
	}
	if got := get(unscoped); got.Code != http.StatusForbidden {
		t.Fatalf("routed principal without scope: %d %s", got.Code, got.Body.String())
	}
	if got := get(reader); got.Code != http.StatusOK {
		t.Fatalf("stage_handoffs.read: %d %s", got.Code, got.Body.String())
	}

	prepare, err := func() (Handoff, error) {
		var h Handoff
		err := db.InTenant(dbtest.Seed(ctx), m.pool, owner.TenantID, func(tx pgx.Tx) error {
			var err error
			h, err = m.create(ctx, tx, owner, RequestWrite{ProjectNodeID: project, ReleaseNodeID: release, Stage: "access", Operation: "prepare", ExpectedJourneyRevision: 1, IdempotencyKey: "seal-check"}, "janus", []string{"authorization", "credential_handoff"}, "")
			return err
		})
		return h, err
	}()
	if err != nil {
		t.Fatal(err)
	}
	wrong := strings.Repeat("ab", 32)
	if wrong == prepare.PrerequisiteSealSHA256 {
		wrong = strings.Repeat("cd", 32)
	}
	raw, _ := json.Marshal(ResultWrite{Outcome: "failed", TerminalSequence: 1, AuthorityEpoch: prepare.AuthorityEpoch, PrerequisiteSealSHA256: wrong, BlockerCode: stringPtr("policy_refused")})
	req := httptest.NewRequest(http.MethodPost, "/api/stage-handoffs/"+prepare.ID+"/result", bytes.NewReader(raw)).WithContext(tenant.WithPrincipal(ctx, owner))
	req.Header.Set("Authorization", bearer)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "seal") {
		t.Fatalf("wrong seal: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLaunchHTTPRefusesMissingAndForeignOperation(t *testing.T) {
	m, owner, project, release, _ := fixture(t)
	ctx := context.Background()
	var handoffID string
	err := db.InTenant(dbtest.Seed(ctx), m.pool, owner.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO stage_handoffs
			(tenant_id,project_node_id,release_node_id,stage,operation,plugin_id,requested_by_principal_id,idempotency_key,attempt,authority_epoch,journey_revision,plan_digest,predecessor_digest,context_digest,prerequisite_seal_sha256,evidence_ceiling,expires_at)
			VALUES($1::uuid,$2::uuid,$3::uuid,'access','prepare','janus',$4::uuid,'launch-http',1,1,1,$5,$5,$5,$5,ARRAY['authorization'],now()+interval '30 minutes')
			RETURNING id::text`, owner.TenantID, project, release, owner.ID, emptyDigest).Scan(&handoffID)
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	m.Mount(mux)
	call := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(tenant.WithPrincipal(ctx, owner))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	missing := call("/api/stage-handoffs/"+handoffID+"/launch/consume", `{"admission_id":"00000000-0000-4000-8000-000000000099"}`)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("consume without admit: %d %s", missing.Code, missing.Body.String())
	}
	artifact := `{"version_scheme":"inspr-calendar-v2","version":"1","release_channel":"stable","release_sequence":1,"digest_sha256":"` + emptyDigest + `","commit_digest":"commit","manifest_coordinate":"manifest","manifest_digest_sha256":"` + emptyDigest + `"}`
	foreign := call("/api/stage-handoffs/"+handoffID+"/launch/admit", artifact)
	if foreign.Code != http.StatusConflict {
		t.Fatalf("admit on non-deploy: %d %s", foreign.Code, foreign.Body.String())
	}
}
