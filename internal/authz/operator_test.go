// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"errors"
	"strings"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/jackc/pgx/v5"
)

func TestOperatorProjectBindingsUseRLSStoreAndEvents(t *testing.T) {
	d := dbtest.Open(t)
	ctx := t.Context()
	var tid, agentID, duplicateID, projectID string
	if err := db.InTenant(dbtest.Seed(ctx), d.App, "00000000-0000-0000-0000-000000000000", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO tenants(slug,name) VALUES('ab1-operator','AB1') RETURNING id::text`).Scan(&tid)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.InTenant(dbtest.Seed(ctx), d.App, tid, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'agent','worker') RETURNING id::text`, tid).Scan(&agentID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'agent','worker') RETURNING id::text`, tid).Scan(&duplicateID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title,state)
			SELECT $1::uuid,'PHAROS-1',id,'Pharos','active' FROM node_kinds WHERE tenant_id=$1::uuid AND slug='project' RETURNING id::text`, tid).Scan(&projectID)
	}); err != nil {
		t.Fatal(err)
	}
	// The same pool used by the operator is the production-like schema owner.
	var bypass bool
	if err := db.InTenant(dbtest.Seed(ctx), d.App, tid, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&bypass)
	}); err != nil || bypass {
		t.Fatalf("app role must be NOBYPASSRLS: bypass=%v err=%v", bypass, err)
	}
	if _, err := OperatorBindProjects(ctx, d.App, tid, "worker", []ProjectRolePair{{Project: "PHAROS-1", Role: "guest"}}); err == nil || !strings.Contains(err.Error(), agentID) || !strings.Contains(err.Error(), duplicateID) {
		t.Fatalf("ambiguous name must list both IDs: %v", err)
	}
	if _, err := OperatorBindProjects(ctx, d.App, tid, agentID, []ProjectRolePair{{Project: "PHAROS-1", Role: "owner"}}); !errors.Is(err, errProjectRole) {
		t.Fatalf("owner should be refused: %v", err)
	}
	if _, err := OperatorBindProjects(ctx, d.App, tid, agentID, []ProjectRolePair{{Project: "PHAROS-1", Role: "customer"}}); !errors.Is(err, errProjectRole) {
		t.Fatalf("customer should be refused: %v", err)
	}
	if _, err := OperatorBindProjects(ctx, d.App, tid, agentID, []ProjectRolePair{{Project: "PHAROS-1", Role: "guest"}, {Project: "MISSING-1", Role: "guest"}}); err == nil {
		t.Fatal("invalid second project accepted")
	}
	var n int
	if err := db.InTenant(dbtest.Seed(ctx), d.App, tid, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM role_bindings WHERE principal_id=$1::uuid AND scope_type='project'`, agentID).Scan(&n)
	}); err != nil || n != 0 {
		t.Fatalf("multi-bind must roll back: count=%d err=%v", n, err)
	}
	for i := 0; i < 2; i++ {
		bindings, err := OperatorBindProjects(ctx, d.App, tid, agentID, []ProjectRolePair{{Project: "PHAROS-1", Role: "guest"}})
		if err != nil || len(bindings) != 1 || bindings[0].ProjectID != projectID {
			t.Fatalf("bind %d: %+v %v", i, bindings, err)
		}
	}
	assertBindingEvent(t, d, tid, agentID, "binding.set", 1)
	if err := OperatorUnbindProject(ctx, d.App, tid, agentID, "PHAROS-1"); err != nil {
		t.Fatal(err)
	}
	if err := OperatorUnbindProject(ctx, d.App, tid, agentID, "PHAROS-1"); err != nil {
		t.Fatal(err)
	}
	assertBindingEvent(t, d, tid, agentID, "binding.removed", 1)
}

func assertBindingEvent(t *testing.T, d *dbtest.DB, tid, actor, typ string, want int) {
	t.Helper()
	var n int
	if err := db.InTenant(dbtest.Seed(t.Context()), d.App, tid, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type=$1 AND actor_principal_id=$2::uuid`, typ, actor).Scan(&n)
	}); err != nil || n != want {
		t.Fatalf("%s actor %s: got %d want %d err=%v", typ, actor, n, want, err)
	}
}
