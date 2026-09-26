// SPDX-License-Identifier: AGPL-3.0-only

package hours

import (
	"context"

	"github.com/inspr-at/paimos/internal/plugins"
	"github.com/inspr-at/paimos/internal/plugins/fence"
)

const PluginID = "business_hours"

// Plugin returns the compiled declaration, with an exact canonical digest.
// Workflow hooks fail closed: only this package's transactional HTTP host can
// observe and apply entry/approval facts. Caller-supplied payloads grant nothing.
func Plugin() (plugins.Plugin, error) {
	p := plugins.Plugin{
		Manifest: plugins.Manifest{ID: PluginID, Version: "1", Owner: "inspr-at",
			Permissions: []string{fence.PermViewsProvide, fence.PermStepsApply},
			Views:       []plugins.View{{ID: "hours", Panels: []string{"hours"}}},
			WorkflowSteps: []plugins.WorkflowStep{
				{Key: "time_entry", Gates: []string{fence.GateObservedState}},
				{Key: "period_approve", Gates: []string{fence.GateObservedState, fence.GatePersonDecision}},
			},
		},
		StepPermissions: map[string]string{"time_entry": fence.PermStepsApply, "period_approve": fence.PermStepsApply},
		Views:           extension{}, Steps: extension{},
	}
	digest, err := plugins.Digest(p)
	p.Manifest.DigestSHA256 = digest
	return p, err
}

type extension struct{}

func (extension) Views(ctx context.Context, call plugins.Call) ([]plugins.View, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !call.Grant.Allows(fence.PermViewsProvide) {
		return nil, plugins.ErrDenied
	}
	return []plugins.View{{ID: "hours", Panels: []string{"hours"}}}, nil
}
func (extension) Evaluate(ctx context.Context, _ plugins.Call, _ plugins.StepRequest) (plugins.StepDecision, error) {
	return hostRequired(ctx)
}
func (extension) Request(ctx context.Context, _ plugins.Call, _ plugins.StepRequest) (plugins.StepDecision, error) {
	return hostRequired(ctx)
}
func (extension) ApplyResult(ctx context.Context, _ plugins.Call, _ plugins.StepResult) (plugins.StepDecision, error) {
	return hostRequired(ctx)
}
func hostRequired(ctx context.Context) (plugins.StepDecision, error) {
	if err := ctx.Err(); err != nil {
		return plugins.StepDecision{}, err
	}
	return plugins.StepDecision{Proceed: false, Outcome: "blocked", Blocker: "hours_transaction_required"}, nil
}
