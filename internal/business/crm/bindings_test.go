// SPDX-License-Identifier: AGPL-3.0-only

package crm

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/inspr-at/aeon/internal/tenant"
)

func TestListContactBindings(t *testing.T) {
	f := setup(t)
	path := "/api/crm/contacts/" + f.contact + "/principals"
	w := request(f.handler, f.admin, "GET", path, "")
	expect(t, w, 200)
	if w.Body.String() != "[]\n" && w.Body.String() != "[]" {
		t.Fatalf("empty list %q", w.Body.String())
	}
	expect(t, request(f.handler, f.admin, "POST", path, fmt.Sprintf(`{"principal_id":%q}`, f.customer)), 201)

	member := tenant.Principal{ID: f.member, TenantID: f.admin.TenantID, Kind: tenant.Person, Roles: []string{"member"}}
	w = request(f.handler, member, "GET", path, "")
	expect(t, w, 200)
	var out []BoundPrincipal
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].PrincipalID != f.customer || out[0].PrincipalName != "Customer" || out[0].PrincipalKind != "person" || out[0].BoundByPrincipalID != f.admin.ID {
		t.Fatalf("bindings %+v", out)
	}

	// Customers and agents do not read the binding list.
	customer := tenant.Principal{ID: f.customer, TenantID: f.admin.TenantID, Kind: tenant.Person, Roles: []string{"customer"}}
	expect(t, request(f.handler, customer, "GET", path, ""), 403)
	agent := tenant.Principal{ID: f.agent, TenantID: f.admin.TenantID, Kind: tenant.Agent}
	expect(t, request(f.handler, agent, "GET", path, ""), 403)
	// Deleted or non-contact nodes are not found; another tenant cannot see the contact.
	expect(t, request(f.handler, f.admin, "GET", "/api/crm/contacts/"+f.deleted+"/principals", ""), 404)
	expect(t, request(f.handler, f.admin, "GET", "/api/crm/contacts/"+f.task+"/principals", ""), 404)
	expect(t, request(f.handler, f.admin, "GET", "/api/crm/contacts/not-a-uuid/principals", ""), 400)
	f.other.Roles = []string{"admin"}
	f.setInstallFor(t, f.other.TenantID, true, f.digest, []string{fence.PermViewsProvide})
	expect(t, request(f.handler, f.other, "GET", path, ""), 404)

	// A closed or under-granted installation fails closed.
	f.setInstall(t, true, f.digest, []string{fence.PermStepsApply})
	expect(t, request(f.handler, f.admin, "GET", path, ""), 409)
	f.setInstall(t, false, f.digest, []string{fence.PermViewsProvide})
	expect(t, request(f.handler, f.admin, "GET", path, ""), 409)
}
