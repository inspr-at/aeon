// SPDX-License-Identifier: AGPL-3.0-only

package approvals

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
)

func TestApprovalRiskProjection(t *testing.T) {
	f := newFixture(t)
	// Expand only this fixture key; requests still pass the real scope ceiling.
	err := db.InTenant(t.Context(), f.db.App, f.tenantA, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE agent_keys SET scopes=ARRAY['nodes','harness','release','run'] WHERE principal_id=$1::uuid AND scopes=ARRAY['run','nodes.read']::text[]`, f.agentA.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ scope, kind, risk string }{{"nodes.read", "node", "low"}, {"nodes.read.fields", "node", "low"}, {"nodes.read", "tenant", "high"}, {"harness.control", "node", "high"}, {"release.deploy", "node", "high"}, {"nodes.delete", "node", "high"}, {"nodes.delete.read", "node", "high"}, {"nodes.write", "node", "medium"}, {"run.claim", "run", "medium"}, {"nodes.readiness", "node", "medium"}} {
		t.Run(tc.scope+"-"+tc.kind, func(t *testing.T) {
			resource := &f.nodeA
			if tc.kind == "tenant" {
				resource = nil
			} else if tc.kind == "run" {
				resource = &f.runA
			}
			response := f.do(f.agentA, f.wide, "POST", "/api/approvals", proposalJSON(tc.scope, tc.kind, resource, nil))
			if response.Code != 201 {
				t.Fatalf("propose %d %s", response.Code, response.Body.String())
			}
			a := decodeApproval(t, response)
			if a.Risk != tc.risk {
				t.Fatalf("risk %s want %s", a.Risk, tc.risk)
			}
			response = f.do(f.personA, "", "POST", "/api/approvals/"+a.ID+"/decision", `{"decision":"approved"}`)
			if response.Code != 200 || decodeApproval(t, response).Risk != tc.risk {
				t.Fatal("decision risk missing")
			}
		})
	}
	response := f.do(f.personA, "", "GET", "/api/approvals", "")
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var items []Approval
	if err = json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 10 {
		t.Fatal("missing approvals")
	}
	for _, item := range items {
		if item.Risk == "" {
			t.Fatal("list risk missing")
		}
	}
	for _, p := range []struct {
		foreign bool
		want    int
	}{{true, 0}, {false, 0}} {
		principal := f.personB
		if !p.foreign {
			principal = f.agentB
		}
		response = f.do(principal, "", "GET", "/api/approvals", "")
		if response.Code != 200 {
			t.Fatal(response.Body.String())
		}
		if err = json.Unmarshal(response.Body.Bytes(), &items); err != nil {
			t.Fatal(err)
		}
		if len(items) != p.want {
			t.Fatal("risk list leaked approvals")
		}
	}
	if response = f.anon("GET", "/api/approvals", ""); response.Code != 401 {
		t.Fatal("anonymous approvals")
	}
	if response = f.do(f.agentA, f.empty, "POST", "/api/approvals", proposalJSON("nodes.delete", "node", &f.nodeA, nil)); response.Code != 403 {
		t.Fatal("risk bypassed scope ceiling")
	}
}

func TestDecisionRiskRequiresPersonRole(t *testing.T) {
	f := newFixture(t)
	person := func(name, role string) tenant.Principal {
		p := tenant.Principal{TenantID: f.tenantA, Kind: tenant.Person, Name: name, Roles: []string{role}}
		err := db.InTenant(t.Context(), f.db.App, f.tenantA, func(tx pgx.Tx) error {
			return tx.QueryRow(t.Context(), `INSERT INTO principals(tenant_id,kind,name,roles) VALUES($1::uuid,'person',$2,$3) RETURNING id::text`, f.tenantA, name, p.Roles).Scan(&p.ID)
		})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	member := person("member", "member")
	customer := person("customer", "customer")
	super := person("global-admin", "super_admin")
	for _, tc := range []struct {
		scope, kind string
		resource    *string
		risk        string
		decider     tenant.Principal
		want        int
	}{
		{"nodes.read", "node", &f.nodeA, "low", member, http.StatusOK},
		{"run.claim", "run", &f.runA, "medium", member, http.StatusOK},
		{"nodes.read", "tenant", nil, "high", member, http.StatusForbidden},
		{"nodes.read", "node", &f.nodeA, "low", customer, http.StatusForbidden},
		{"nodes.read", "tenant", nil, "high", super, http.StatusOK},
	} {
		proposal := f.do(f.agentA, f.wide, http.MethodPost, "/api/approvals", proposalJSON(tc.scope, tc.kind, tc.resource, nil))
		if proposal.Code != http.StatusCreated {
			t.Fatalf("proposal %s: %d %s", tc.risk, proposal.Code, proposal.Body.String())
		}
		a := decodeApproval(t, proposal)
		if a.Risk != tc.risk {
			t.Fatalf("risk %s: %s", tc.risk, a.Risk)
		}
		decision := f.do(tc.decider, "", http.MethodPost, "/api/approvals/"+a.ID+"/decision", `{"decision":"approved"}`)
		if decision.Code != tc.want {
			t.Errorf("%s by %s: %d %s", tc.risk, tc.decider.Name, decision.Code, decision.Body.String())
		}
		if tc.risk == "high" && tc.decider.Name == "member" {
			adminDecision := f.do(f.personA, "", http.MethodPost, "/api/approvals/"+a.ID+"/decision", `{"decision":"approved"}`)
			if adminDecision.Code != http.StatusOK {
				t.Fatalf("admin high: %d %s", adminDecision.Code, adminDecision.Body.String())
			}
		}
	}
}
