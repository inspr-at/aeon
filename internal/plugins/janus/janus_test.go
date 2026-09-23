// SPDX-License-Identifier: AGPL-3.0-only

package janus

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/plugins/fence"
)

func TestPrepareDoesNotAdvanceAccess(t *testing.T) {
	facts := observed(OperationPrepare)
	facts.DeploymentSucceeded = false
	decision, err := Evaluate(actor, facts)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Proceed || decision.AdvancesAccess || decision.AdvancesRelease || decision.ConsumePermit {
		t.Fatalf("prepare = %#v", decision)
	}
	if decision.Seal == "" || strings.Contains(marshal(t, decision), actor) {
		t.Fatalf("decision leaked actor or skipped the seal: %s", marshal(t, decision))
	}
	for _, item := range decision.Evidence {
		raw := marshal(t, item)
		for _, forbidden := range []string{"principal", "grant", "secret", "http", "/"} {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("evidence %s contains %s", raw, forbidden)
			}
		}
	}
}

func TestApplyRequiresDeploymentAndBoundedPermit(t *testing.T) {
	facts := observed(OperationApply)
	decision, err := Evaluate(actor, facts)
	if err != nil || decision.Blocker != fence.BlockerDependencyPending {
		t.Fatalf("without deployment = %#v %v", decision, err)
	}
	facts.DeploymentSucceeded = true
	decision, err = Request(actor, facts)
	if err != nil || decision.Blocker != fence.BlockerPolicyRefused || decision.ConsumePermit {
		t.Fatalf("without permit = %#v %v", decision, err)
	}
	facts.PersonApproved = true
	facts.Permit = Permit{Approved: true, Bounded: false}
	decision, err = Evaluate(actor, facts)
	if err != nil || decision.Proceed || decision.Blocker != fence.BlockerPolicyRefused {
		t.Fatalf("unbounded = %#v %v", decision, err)
	}
	facts.Permit = Permit{Approved: true, Bounded: true}
	decision, err = Request(actor, facts)
	if err != nil || !decision.Proceed || !decision.ConsumePermit || decision.AdvancesAccess || decision.AdvancesRelease {
		t.Fatalf("accepted apply = %#v %v", decision, err)
	}

	facts.Permit.Consumed = true
	applied, err := Apply(actor, facts, "succeeded", decision.Evidence)
	if err != nil || !applied.Proceed || applied.Outcome != "succeeded" || applied.AdvancesRelease {
		t.Fatalf("result = %#v %v", applied, err)
	}
	failed, err := Apply(actor, facts, "succeeded", flip(decision.Evidence))
	if err != nil || failed.Proceed || failed.Outcome != "failed" {
		t.Fatalf("promoted = %#v %v", failed, err)
	}
}

func TestForbiddenMaterialIsNotCopied(t *testing.T) {
	facts := observed(OperationPrepare)
	facts.Forbidden.URLs = []string{"https://secret.example/token"}
	facts.Forbidden.Secrets = []string{"super-secret-value"}
	decision, err := Evaluate(actor, facts)
	if err != nil {
		t.Fatal(err)
	}
	raw := marshal(t, decision)
	if decision.Proceed || decision.Blocker != fence.BlockerPolicyRefused || len(decision.Evidence) != 0 {
		t.Fatalf("forbidden = %#v", decision)
	}
	if strings.Contains(raw, "secret.example") || strings.Contains(raw, "super-secret-value") || strings.Contains(raw, actor) {
		t.Fatalf("decision copied forbidden material: %s", raw)
	}
}

func TestMissingObservationDoesNotSeal(t *testing.T) {
	facts := observed(OperationPrepare)
	facts.CredentialReady = nil
	decision, err := Evaluate(actor, facts)
	if err != nil || decision.Blocker != fence.BlockerExternalWaiting || decision.Seal != "" {
		t.Fatalf("partial = %#v %v", decision, err)
	}
}

const actor = "11111111-1111-4111-8111-111111111111"

func observed(operation string) Facts {
	return Facts{
		Operation:       operation,
		Authorized:      boolPtr(true),
		CredentialReady: boolPtr(true),
		ObservedAt:      time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
	}
}

func boolPtr(v bool) *bool { return &v }

func flip(in []Evidence) []Evidence {
	out := append([]Evidence(nil), in...)
	for i := range out {
		if out[i].Authorized != nil {
			value := false
			out[i].Authorized = &value
		}
	}
	return out
}

func marshal(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
