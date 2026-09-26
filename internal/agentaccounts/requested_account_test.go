// SPDX-License-Identifier: AGPL-3.0-only

package agentaccounts

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/jackc/pgx/v5"
)

func TestRequestedAccountNarrowsEnrollmentWithoutFallback(t *testing.T) {
	reset(t)
	admin := makePrincipal(t, "alpha", "person", "Ada", []string{"admin"})
	runner := addPrincipal(t, admin.TenantID, "agent", "runner", nil)
	profile := codexProfile(t, admin)
	token := issueKey(t, runner, []string{"account.manage"})
	mod := accountsMod()
	register := func(key string) Account {
		t.Helper()
		var a Account
		callStatus(t, mod, &runner, token, "POST", "/api/agent-accounts", fmt.Sprintf(`{"account_key":%q,"harness":"codex","daemon_id":"daemon-a","label":"Local account"}`, key), 201, &a)
		callStatus(t, mod, &runner, token, "POST", "/api/agent-accounts/"+a.ID+"/probe", `{"daemon_id":"daemon-a","daemon_generation":"g1","available":true}`, 200, nil)
		callStatus(t, mod, &admin, "", "POST", "/api/agent-accounts/"+a.ID+"/windows", windowBody(time.Now().Add(-time.Minute), time.Now().Add(time.Hour), "requests", 100, "unrestricted"), 201, nil)
		return a
	}
	other, chosen := register("other"), register("chosen")
	seed := func(fn func(pgx.Tx) error) {
		t.Helper()
		if err := db.InTenant(dbtest.Seed(t.Context()), appPool, admin.TenantID, fn); err != nil {
			t.Fatal(err)
		}
	}
	seed(func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE account_allowance_windows SET used=50 WHERE account_id=$1`, chosen.ID)
		return err
	})
	pinnedRun := func() string {
		id := insertRun(t, admin, runner, profile)
		seed(func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), `UPDATE agent_runs SET requested_account_id=$2 WHERE id=$1`, id, chosen.ID)
			return err
		})
		return id
	}
	run := pinnedRun()
	callStatus(t, mod, &runner, token, "POST", "/api/agent-accounts/route", routeBody(t, run, "daemon-a", []Account{other}, map[string]int64{"requests": 1}), http.StatusConflict, nil)
	if scalar(t, admin, `SELECT count(*) FROM account_reservations WHERE run_id=$1`, run) != 0 {
		t.Fatal("missing enrollment reserved allowance")
	}
	got := mustRoute(t, mod, runner, token, run, "daemon-a", []Account{other, chosen}, map[string]int64{"requests": 1})
	if got.AccountID != chosen.ID {
		t.Fatal("routing ignored the requested account in favor of a lower-usage account")
	}
	// Exact replay is still idempotent; the choice cannot be widened on replay.
	mustRoute(t, mod, runner, token, run, "daemon-a", []Account{other, chosen}, map[string]int64{"requests": 1})
	if scalar(t, admin, `SELECT count(*) FROM events WHERE type='account.reserved'`) != 1 {
		t.Fatal("replay wrote another event")
	}
	// The chosen account is occupied. The other account remains available but
	// must not be an implicit fallback, and no partial reservation may commit.
	waiting := pinnedRun()
	callStatus(t, mod, &runner, token, "POST", "/api/agent-accounts/route", routeBody(t, waiting, "daemon-a", []Account{other, chosen}, map[string]int64{"requests": 1}), http.StatusConflict, nil)
	if scalar(t, admin, `SELECT count(*) FROM account_reservations WHERE run_id=$1`, waiting) != 0 || scalar(t, admin, `SELECT count(*) FROM agent_runs WHERE id=$1 AND account_id IS NOT NULL`, waiting) != 0 {
		t.Fatal("unavailable choice silently fell back or reserved allowance")
	}
}
