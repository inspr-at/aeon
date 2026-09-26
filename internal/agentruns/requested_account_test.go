// SPDX-License-Identifier: AGPL-3.0-only

package agentruns_test

import (
	"testing"

	"github.com/inspr-at/aeon/internal/agentruns"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestRequestedAccountIsValidatedAndAuditedWithoutReservation(t *testing.T) {
	f := setup(t)
	o := f.order(t, nil)
	account := func(p tenant.Principal, harness string) string {
		t.Helper()
		id := uuid()
		f.tx(t, p, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), `INSERT INTO agent_accounts(tenant_id,id,account_key,harness,daemon_id,registered_by_principal_id,label)
			 VALUES($1,$2::uuid,$2::text,$3,'test-daemon',$4,'Test account')`, p.TenantID, id, harness, p.ID)
			return err
		})
		return id
	}
	chosen := account(f.agent, "codex")
	wrongOwner := account(f.other, "codex")
	wrongHarness := account(f.agent, "claude")
	foreignAgent := tenant.Principal{ID: uuid(), TenantID: f.foreign.TenantID, Kind: tenant.Agent}
	f.tx(t, foreignAgent, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO principals(id,tenant_id,kind,name) VALUES($1,$2,'agent','Foreign agent')`, foreignAgent.ID, foreignAgent.TenantID)
		return err
	})
	foreignAccount := account(foreignAgent, "codex")
	path := "/api/work-orders/" + o.NodeID + "/runs"
	for _, invalid := range []struct {
		id     string
		status int
	}{{"invalid", 400}, {wrongOwner, 409}, {wrongHarness, 409}, {foreignAccount, 409}, {uuid(), 409}} {
		f.call(t, f.person, "POST", path, map[string]any{"agent_principal_id": f.agent.ID, "model_profile_id": f.profile, "requested_account_id": invalid.id}, invalid.status, nil)
	}
	if f.count(t, f.person, `SELECT count(*) FROM agent_runs`) != 0 || f.count(t, f.person, `SELECT count(*) FROM events WHERE type='run.created'`) != 0 {
		t.Fatal("rejected choices created a run or event")
	}
	var run agentruns.Run
	f.call(t, f.person, "POST", path, map[string]any{"agent_principal_id": f.agent.ID, "model_profile_id": f.profile, "requested_account_id": chosen}, 201, &run)
	if run.Status != "queued" || run.AccountID != nil || run.RequestedAccountID == nil || *run.RequestedAccountID != chosen {
		t.Fatal("account choice was not retained separately from a reservation")
	}
	if f.count(t, f.person, `SELECT count(*) FROM account_reservations WHERE run_id=$1`, run.ID) != 0 {
		t.Fatal("creating a run reserved account allowance")
	}
	if f.count(t, f.person, `SELECT count(*) FROM events WHERE type='run.created' AND after->>'requested_account_id'=$1`, chosen) != 1 {
		t.Fatal("account choice missing from run.created event")
	}
	var read agentruns.Run
	f.call(t, f.person, "GET", "/api/runs/"+run.ID, nil, 200, &read)
	if read.RequestedAccountID == nil || *read.RequestedAccountID != chosen {
		t.Fatal("read lost the requested account")
	}
	f.call(t, f.foreign, "GET", "/api/runs/"+run.ID, nil, 404, nil)
}
