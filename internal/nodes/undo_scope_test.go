// SPDX-License-Identifier: AGPL-3.0-only

package nodes

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/tenant"
)

// Undo needs what the reversing change needs (review finding 1): a
// principal who may undo other people's changes but not edit nodes cannot
// reverse a batch edit by undoing it.
func TestBulkUndoNeedsTheEditPermission(t *testing.T) {
	owner := newPrincipal(t, "bulk-undo-scope")
	project := kindBySlug(t, owner, "project")
	ticket := kindBySlug(t, owner, "ticket")
	root := mustNode(t, owner, `{"kind_id":"`+project.ID+`","title":"Undo project"}`)
	item := mustNode(t, owner, `{"kind_id":"`+ticket.ID+`","parent_id":"`+root.ID+`","title":"Item","state":"new"}`)
	status, raw := call(t, &owner, http.MethodPost, "/api/nodes/bulk", `{"ids":["`+item.ID+`"],"state":"done"}`)
	batch := decode[bulkResult](t, status, raw, http.StatusOK)

	undoer := tenant.Principal{TenantID: owner.TenantID, Kind: tenant.Person, Name: "Undoer"}
	if err := testDB.Admin.QueryRow(t.Context(), `INSERT INTO principals(tenant_id,kind,name) VALUES($1,'person','Undoer') RETURNING id::text`, owner.TenantID).Scan(&undoer.ID); err != nil {
		t.Fatal(err)
	}
	var role string
	if err := testDB.Admin.QueryRow(t.Context(), `INSERT INTO roles(tenant_id,key,name) VALUES($1,'undo_only','Undo only') RETURNING id::text`, owner.TenantID).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.Admin.Exec(t.Context(), `INSERT INTO role_permissions(tenant_id,role_id,permission) SELECT $1,$2,unnest(ARRAY['nodes.read','events.read','events.undo','events.undo_other'])`, owner.TenantID, role); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.Admin.Exec(t.Context(), `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) VALUES($1,$2,$3,'workspace')`, owner.TenantID, undoer.ID, role); err != nil {
		t.Fatal(err)
	}
	undo := func(p tenant.Principal) int {
		status, _ := callAs(t, events.New(appPool, events.WithUndoHandlers(UndoHandlers())), &p, http.MethodPost, "/api/events/"+strconv.FormatInt(*batch.EventID, 10)+"/undo", "")
		return status
	}
	if status := undo(undoer); status != http.StatusForbidden {
		t.Fatalf("undo without nodes.write: %d", status)
	}
	if status := undo(owner); status != http.StatusCreated {
		t.Fatalf("owner undo: %d", status)
	}
}
