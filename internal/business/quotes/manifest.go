// SPDX-License-Identifier: AGPL-3.0-only

// Package quotes implements R4 frozen versions and QP1 mutable documents. The
// coordinator registers ManifestPlugin before sealing the registry and mounts
// New on the API server. An existing tenant installation must pin this digest.
package quotes

import (
	"context"

	"github.com/inspr-at/paimos/internal/plugins"
	"github.com/inspr-at/paimos/internal/plugins/fence"
)

const PluginID = "business_quotes"

type declaration struct{}

func (declaration) NodeKinds(context.Context, plugins.Call) ([]plugins.NodeKind, error) {
	return []plugins.NodeKind{{Slug: "quote", FieldSchema: []byte(`{"type":"object","properties":{}}`)}}, nil
}
func (declaration) Views(context.Context, plugins.Call) ([]plugins.View, error) {
	return []plugins.View{{ID: "quotes", Panels: []string{"quotes"}}}, nil
}

// Workflow hooks never grant authority: host handlers verify observed facts,
// person identity and current rows inside the write transaction.
func (declaration) Evaluate(context.Context, plugins.Call, plugins.StepRequest) (plugins.StepDecision, error) {
	return plugins.StepDecision{Blocker: fence.BlockerPolicyRefused}, nil
}
func (declaration) Request(context.Context, plugins.Call, plugins.StepRequest) (plugins.StepDecision, error) {
	return plugins.StepDecision{Blocker: fence.BlockerPolicyRefused}, nil
}
func (declaration) ApplyResult(context.Context, plugins.Call, plugins.StepResult) (plugins.StepDecision, error) {
	return plugins.StepDecision{Blocker: fence.BlockerPolicyRefused}, nil
}

// ManifestPlugin is the compiled business_quotes declaration. Register it
// with the shared registry before Seal; pass the sealed registry to New.
func ManifestPlugin() (plugins.Plugin, error) {
	d := declaration{}
	p := plugins.Plugin{Manifest: plugins.Manifest{
		ID: PluginID, Version: "2", Owner: "aeon",
		Permissions: []string{fence.PermNodesContribute, fence.PermViewsProvide, fence.PermStepsApply},
		NodeKinds:   []plugins.NodeKind{{Slug: "quote", FieldSchema: []byte(`{"type":"object","properties":{}}`)}},
		Views:       []plugins.View{{ID: "quotes", Panels: []string{"quotes"}}},
		WorkflowSteps: []plugins.WorkflowStep{
			{Key: "quote_issue", Gates: []string{fence.GateObservedState, fence.GatePersonDecision}},
			{Key: "quote_accept", Gates: []string{fence.GateObservedState, fence.GatePersonDecision}},
			{Key: "quote_finalize", Gates: []string{fence.GateObservedState, fence.GatePersonDecision}},
			{Key: "quote_branch", Gates: []string{fence.GateObservedState, fence.GatePersonDecision}},
		},
	}, StepPermissions: map[string]string{"quote_issue": fence.PermStepsApply, "quote_accept": fence.PermStepsApply, "quote_finalize": fence.PermStepsApply, "quote_branch": fence.PermStepsApply}, Kinds: d, Views: d, Steps: d}
	sum, err := plugins.Digest(p)
	if err != nil {
		return plugins.Plugin{}, err
	}
	p.Manifest.DigestSHA256 = sum
	return p, nil
}
