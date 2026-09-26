// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/dbtest"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestOperatorProjectBindingsUseRLSStoreAndEvents(t *testing.T) {
	d := dbtest.Open(t)
	ctx := t.Context()
	var tid, agentID, duplicateID, projectID, ownerID string
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
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'person','Owner') RETURNING id::text`, tid).Scan(&ownerID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO nodes(tenant_id,key,kind_id,title,state)
			SELECT $1::uuid,'PHAROS-1',id,'Pharos','active' FROM node_kinds WHERE tenant_id=$1::uuid AND slug='project' RETURNING id::text`, tid).Scan(&projectID)
	}); err != nil {
		t.Fatal(err)
	}
	dbtest.BindRole(t, d, tid, ownerID, "owner")
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
	var wg sync.WaitGroup
	bindErrors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bindings, err := OperatorBindProjects(ctx, d.App, tid, agentID, []ProjectRolePair{{Project: "PHAROS-1", Role: "guest"}})
			if err == nil && (len(bindings) != 1 || bindings[0].ProjectID != projectID) {
				err = errors.New("wrong binding result")
			}
			bindErrors <- err
		}()
	}
	wg.Wait()
	close(bindErrors)
	for err := range bindErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	operatorID := assertOperator(t, d, tid)
	assertBindingEvent(t, d, tid, operatorID, "binding.set", 1)
	if err := OperatorUnbindProject(ctx, d.App, tid, agentID, "PHAROS-1"); err != nil {
		t.Fatal(err)
	}
	if err := OperatorUnbindProject(ctx, d.App, tid, agentID, "PHAROS-1"); err != nil {
		t.Fatal(err)
	}
	assertBindingEvent(t, d, tid, operatorID, "binding.removed", 1)

	actor := tenant.Principal{ID: ownerID, TenantID: tid, Kind: tenant.Person}
	req := httptest.NewRequest("GET", "/api/audit?category=access", nil)
	req = req.WithContext(tenant.WithPrincipal(req.Context(), actor))
	rec := httptest.NewRecorder()
	(&Module{pool: d.App}).audit(rec, req)
	if rec.Code != 200 {
		t.Fatalf("audit %d: %s", rec.Code, rec.Body.String())
	}
	var audit struct {
		Items []auditItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &audit); err != nil {
		t.Fatal(err)
	}
	for _, item := range audit.Items {
		if item.Type == "binding.set" || item.Type == "binding.removed" {
			if item.Actor.PrincipalID != operatorID || item.Actor.Name != "Access operator" || item.Subject == nil || item.Subject.PrincipalID != agentID {
				t.Fatalf("wrong access audit attribution: %+v", item)
			}
		}
	}
	rec = httptest.NewRecorder()
	(&Module{pool: d.App}).members(rec, req)
	if rec.Code != 200 {
		t.Fatalf("members %d: %s", rec.Code, rec.Body.String())
	}
	var members MemberDirectory
	if err := json.Unmarshal(rec.Body.Bytes(), &members); err != nil {
		t.Fatal(err)
	}
	for _, person := range members.People {
		if person.PrincipalID == operatorID {
			t.Fatal("operator appeared as a person")
		}
	}
}

func TestOperatorWorkspaceBindingsUseHTTPStoreAndProtectOwner(t *testing.T) {
	d := dbtest.Open(t)
	ctx := t.Context()
	var tid, other, agentID, ownerID, foreignID string
	if err := db.InTenant(dbtest.Seed(ctx), d.App, "00000000-0000-0000-0000-000000000000", func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO tenants(slug,name) VALUES('ab1-workspace','AB1 Workspace') RETURNING id::text`).Scan(&tid); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO tenants(slug,name) VALUES('ab1-workspace-other','Other') RETURNING id::text`).Scan(&other)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.InTenant(dbtest.Seed(ctx), d.App, other, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'agent','foreign') RETURNING id::text`, other).Scan(&foreignID)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.InTenant(dbtest.Seed(ctx), d.App, tid, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'agent','worker') RETURNING id::text`, tid).Scan(&agentID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,name) VALUES($1::uuid,'person','Owner') RETURNING id::text`, tid).Scan(&ownerID)
	}); err != nil {
		t.Fatal(err)
	}
	dbtest.BindRole(t, d, tid, ownerID, "owner")
	for _, key := range []string{"owner", "guest"} {
		if err := OperatorValidateWorkspaceRole(ctx, d.App, tid, key); err == nil {
			t.Fatalf("%s passed preflight", key)
		}
		if err := OperatorBindWorkspaceRole(ctx, d.App, tid, agentID, key); err == nil {
			t.Fatalf("%s bound to agent", key)
		}
	}
	if err := OperatorBindWorkspaceRole(ctx, d.App, tid, foreignID, "member"); err == nil {
		t.Fatal("foreign principal bound")
	}
	if err := OperatorUnbindWorkspaceRole(ctx, d.App, tid, ownerID); !lastOwnerViolation(err) {
		t.Fatalf("last owner changed: %v", err)
	}
	if err := OperatorBindWorkspaceRole(ctx, d.App, tid, agentID, "member"); err != nil {
		t.Fatal(err)
	}
	if err := OperatorBindWorkspaceRole(ctx, d.App, tid, agentID, "member"); err != nil {
		t.Fatal(err)
	}
	operatorID := assertOperator(t, d, tid)
	check := func(want int, role string) {
		t.Helper()
		var count, attributed int
		var boundRole *string
		if err := db.InTenant(dbtest.Seed(ctx), d.App, tid, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE actor_principal_id=$2::uuid),
				(SELECT r.key FROM role_bindings b JOIN roles r ON r.tenant_id=b.tenant_id AND r.id=b.role_id
				 WHERE b.tenant_id=$1::uuid AND b.principal_id=$3::uuid AND b.scope_type='workspace')
				FROM events WHERE tenant_id=$1::uuid AND type='authz.workspace_role_changed'`, tid, operatorID, agentID).Scan(&count, &attributed, &boundRole)
		}); err != nil || count != want || attributed != want || role == "" && boundRole != nil || role != "" && (boundRole == nil || *boundRole != role) {
			t.Fatalf("workspace state role=%v events=%d attributed=%d want=%s/%d err=%v", boundRole, count, attributed, role, want, err)
		}
	}
	check(1, "member")
	if err := OperatorUnbindWorkspaceRole(ctx, d.App, tid, agentID); err != nil {
		t.Fatal(err)
	}
	if err := OperatorUnbindWorkspaceRole(ctx, d.App, tid, agentID); err != nil {
		t.Fatal(err)
	}
	check(2, "")
	// Customer is a workspace role under the same rule as the HTTP path.
	if err := OperatorBindWorkspaceRole(ctx, d.App, tid, agentID, "customer"); err != nil {
		t.Fatal(err)
	}
	check(3, "customer")
}

func assertOperator(t *testing.T, d *dbtest.DB, tid string) string {
	t.Helper()
	var id string
	var principals, created, bindings, keys, identities int
	if err := db.InTenant(dbtest.Seed(t.Context()), d.App, tid, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT p.id::text,
			(SELECT count(*) FROM principals WHERE tenant_id=$1::uuid AND name='Access operator'),
			(SELECT count(*) FROM events WHERE tenant_id=$1::uuid AND type='principal.created' AND actor_principal_id=p.id),
			(SELECT count(*) FROM role_bindings WHERE tenant_id=$1::uuid AND principal_id=p.id),
			(SELECT count(*) FROM agent_keys WHERE tenant_id=$1::uuid AND principal_id=p.id),
			(SELECT count(*) FROM identities WHERE id=p.identity_id)
			FROM principals p WHERE p.tenant_id=$1::uuid AND p.kind='agent' AND p.name='Access operator' AND p.roles=ARRAY['operator']::text[]`, tid).Scan(&id, &principals, &created, &bindings, &keys, &identities)
	}); err != nil {
		t.Fatal(err)
	}
	if principals != 1 || created != 1 || bindings != 0 || keys != 0 || identities != 0 {
		t.Fatalf("operator safety: principals=%d created=%d bindings=%d keys=%d identities=%d", principals, created, bindings, keys, identities)
	}
	return id
}

func assertBindingEvent(t *testing.T, d *dbtest.DB, tid, actor, typ string, want int) {
	t.Helper()
	var total, attributed int
	if err := db.InTenant(dbtest.Seed(t.Context()), d.App, tid, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*),count(*) FILTER (WHERE actor_principal_id=$2::uuid)
			FROM events WHERE tenant_id=$1::uuid AND type=$3`, tid, actor, typ).Scan(&total, &attributed)
	}); err != nil || total != want || attributed != want {
		t.Fatalf("%s actor %s: total=%d attributed=%d want=%d err=%v", typ, actor, total, attributed, want, err)
	}
}
