// SPDX-License-Identifier: AGPL-3.0-only

package crm

import (
	"context"
	"fmt"

	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/inspr-at/aeon/internal/tenant"
)

// BindFacts is observed state a host may show the crm_bind step. Setting these
// fields does not bind a principal and does not accept an offer. The HTTP
// handler checks the live rows and the admin person itself.
type BindFacts struct {
	ContactLive   bool
	ContactKind   string
	PrincipalKind string
	ActorKind     string
	ActorAdmin    bool
}

type bindStep struct{}

func (bindStep) Evaluate(ctx context.Context, call plugins.Call, in plugins.StepRequest) (plugins.StepDecision, error) {
	facts, err := ready(ctx, call, in, fence.PermStepsEvaluate)
	if err != nil {
		return plugins.StepDecision{}, err
	}
	return decide(facts), nil
}

func (bindStep) Request(ctx context.Context, call plugins.Call, in plugins.StepRequest) (plugins.StepDecision, error) {
	facts, err := ready(ctx, call, in, fence.PermStepsRequest)
	if err != nil {
		return plugins.StepDecision{}, err
	}
	decision := decide(facts)
	if decision.Proceed {
		decision.Outcome = "admissible"
	}
	return decision, nil
}

// ApplyResult never grants a binding. A claimed outcome is not authority.
func (bindStep) ApplyResult(ctx context.Context, call plugins.Call, in plugins.StepResult) (plugins.StepDecision, error) {
	if _, err := ready(ctx, call, in.StepRequest, fence.PermStepsApply); err != nil {
		return plugins.StepDecision{}, err
	}
	return plugins.StepDecision{
		Proceed: false,
		Outcome: "not_authority",
		Blocker: fence.BlockerPolicyRefused,
	}, nil
}

func ready(ctx context.Context, call plugins.Call, in plugins.StepRequest, permission string) (BindFacts, error) {
	if err := ctx.Err(); err != nil {
		return BindFacts{}, err
	}
	if !call.Grant.Allows(permission) {
		return BindFacts{}, plugins.ErrDenied
	}
	if in.Operation != OperationBind {
		return BindFacts{}, fmt.Errorf("crm: operation does not match crm_bind")
	}
	facts, ok := in.Payload.(BindFacts)
	if !ok {
		return BindFacts{}, fmt.Errorf("crm: bind facts are required")
	}
	return facts, nil
}

func decide(facts BindFacts) plugins.StepDecision {
	person := string(tenant.Person)
	if !facts.ContactLive || facts.ContactKind != Contact || facts.PrincipalKind != person {
		return plugins.StepDecision{Proceed: false, Outcome: "observed_state", Blocker: fence.BlockerPolicyRefused}
	}
	if facts.ActorKind != person || !facts.ActorAdmin {
		return plugins.StepDecision{Proceed: false, Outcome: "person_decision", Blocker: fence.BlockerPolicyRefused}
	}
	return plugins.StepDecision{Proceed: true, Outcome: "eligible"}
}
