// SPDX-License-Identifier: AGPL-3.0-only

package crm

import (
	"context"
	"testing"

	"github.com/inspr-at/paimos/internal/plugins"
	"github.com/inspr-at/paimos/internal/plugins/fence"
	"github.com/inspr-at/paimos/internal/tenant"
)

func TestPluginDeclaration(t *testing.T) {
	first, err := Plugin()
	if err != nil {
		t.Fatal(err)
	}
	second, err := Plugin()
	if err != nil {
		t.Fatal(err)
	}
	if first.Manifest.DigestSHA256 == "" || first.Manifest.DigestSHA256 != second.Manifest.DigestSHA256 {
		t.Fatalf("digest %q", first.Manifest.DigestSHA256)
	}
	reg := plugins.NewRegistry()
	if err := reg.Register(first); err != nil {
		t.Fatal(err)
	}
	reg.Seal()
	stored, ok := reg.Lookup(ID)
	if !ok {
		t.Fatal("missing plugin")
	}
	if stored.Manifest.ID != ID || stored.Manifest.Version != Version || stored.Manifest.Owner != Owner {
		t.Fatalf("manifest %+v", stored.Manifest)
	}
	wantPerms := []string{fence.PermIntegrationsCall, fence.PermNodesContribute, fence.PermStepsApply, fence.PermToolsInvoke, fence.PermViewsProvide}
	if len(stored.Manifest.Permissions) != len(wantPerms) {
		t.Fatalf("permissions %#v", stored.Manifest.Permissions)
	}
	for i, perm := range wantPerms {
		if stored.Manifest.Permissions[i] != perm {
			t.Fatalf("permissions %#v", stored.Manifest.Permissions)
		}
	}
	if len(stored.Manifest.NodeKinds) != 2 || stored.Manifest.NodeKinds[0].Slug != Contact || stored.Manifest.NodeKinds[1].Slug != Organisation {
		t.Fatalf("kinds %#v", stored.Manifest.NodeKinds)
	}
	if len(stored.Manifest.Views) != 1 || stored.Manifest.Views[0].ID != viewID {
		t.Fatalf("views %#v", stored.Manifest.Views)
	}
	if panels := stored.Manifest.Views[0].Panels; len(panels) != 3 || panels[0] != "organisations" || panels[1] != "contacts" || panels[2] != "links" {
		t.Fatalf("panels %#v", stored.Manifest.Views[0].Panels)
	}
	if len(stored.Manifest.WorkflowSteps) != 1 || stored.Manifest.WorkflowSteps[0].Key != OperationBind {
		t.Fatalf("steps %#v", stored.Manifest.WorkflowSteps)
	}
	gates := stored.Manifest.WorkflowSteps[0].Gates
	if len(gates) != 2 || gates[0] != fence.GateObservedState || gates[1] != fence.GatePersonDecision {
		t.Fatalf("gates %#v", gates)
	}
	if stored.StepPermissions[OperationBind] != fence.PermStepsApply {
		t.Fatalf("binding %#v", stored.StepPermissions)
	}
	if len(stored.Manifest.AgentTools) != 1 || stored.Manifest.AgentTools[0].ID != NoteToolID || len(stored.Manifest.Integrations) != 1 || len(stored.Manifest.BackgroundJobs) != 0 {
		t.Fatal("crm tool and provider integration declaration")
	}
	if _, err := stored.Kinds.NodeKinds(context.Background(), plugins.Call{}); err != plugins.ErrDenied {
		t.Fatalf("kinds without grant: %v", err)
	}
	if _, err := stored.Views.Views(context.Background(), plugins.Call{}); err != plugins.ErrDenied {
		t.Fatalf("views without grant: %v", err)
	}
}

func TestBindStepDoesNotGrantAuthority(t *testing.T) {
	person := string(tenant.Person)
	ok := BindFacts{ContactLive: true, ContactKind: Contact, PrincipalKind: person, ActorKind: person, ActorAdmin: true}
	if decision := decide(ok); !decision.Proceed || decision.Outcome != "eligible" || decision.AdvancesRelease || decision.AdvancesAccess {
		t.Fatalf("eligible %+v", decision)
	}
	for _, facts := range []BindFacts{
		{ContactLive: false, ContactKind: Contact, PrincipalKind: person, ActorKind: person, ActorAdmin: true},
		{ContactLive: true, ContactKind: Organisation, PrincipalKind: person, ActorKind: person, ActorAdmin: true},
		{ContactLive: true, ContactKind: Contact, PrincipalKind: string(tenant.Agent), ActorKind: person, ActorAdmin: true},
		{ContactLive: true, ContactKind: Contact, PrincipalKind: person, ActorKind: person, ActorAdmin: false},
		{ContactLive: true, ContactKind: Contact, PrincipalKind: person, ActorKind: string(tenant.Agent), ActorAdmin: true},
	} {
		decision := decide(facts)
		if decision.Proceed || decision.Blocker != fence.BlockerPolicyRefused {
			t.Fatalf("refused %+v for %+v", decision, facts)
		}
	}
	step := bindStep{}
	if _, err := step.ApplyResult(context.Background(), plugins.Call{}, plugins.StepResult{}); err != plugins.ErrDenied {
		t.Fatalf("apply without grant: %v", err)
	}
}

func TestGraphRules(t *testing.T) {
	if !GraphType(CustomerOf) || !GraphType(ContactFor) || GraphType("relates") || GraphType("blocks") {
		t.Fatal("graph type set")
	}
	allowed := []struct{ source, target, rel string }{
		{Organisation, Project, CustomerOf},
		{Organisation, Quote, CustomerOf},
		{Contact, Organisation, ContactFor},
		{Contact, Project, ContactFor},
		{Contact, Quote, ContactFor},
	}
	for _, tc := range allowed {
		if !GraphAllowed(tc.source, tc.target, tc.rel) {
			t.Fatalf("want allow %s %s -> %s", tc.rel, tc.source, tc.target)
		}
	}
	refused := []struct{ source, target, rel string }{
		{Project, Organisation, CustomerOf},
		{Organisation, Contact, CustomerOf},
		{Organisation, "task", CustomerOf},
		{Contact, "task", ContactFor},
		{Organisation, Contact, ContactFor},
		{Contact, Organisation, CustomerOf},
		{Organisation, Project, "relates"},
	}
	for _, tc := range refused {
		if GraphAllowed(tc.source, tc.target, tc.rel) {
			t.Fatalf("want refuse %s %s -> %s", tc.rel, tc.source, tc.target)
		}
	}
}
