// SPDX-License-Identifier: AGPL-3.0-only

package plugins

import (
	"bytes"
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/inspr-at/aeon/internal/authz"
	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/inspr-at/aeon/internal/tenant"
)

func (m *Module) bind(ctx context.Context, p tenant.Principal, pluginID string, need []string) (Call, Plugin, error) {
	if p.ID == "" || p.TenantID == "" {
		return Call{}, Plugin{}, unauthorized()
	}
	plug, ok := m.reg.plugin(pluginID)
	if !ok {
		return Call{}, Plugin{}, notFound("plugin not found")
	}
	if len(need) == 0 {
		return Call{}, Plugin{}, ErrDenied
	}
	var row *installRow
	err := m.inTenant(ctx, p.TenantID, func(tx pgx.Tx) error {
		var loadErr error
		row, loadErr = loadInstall(ctx, tx, p.TenantID, pluginID, false)
		return loadErr
	})
	if err != nil {
		return Call{}, Plugin{}, err
	}
	if row == nil || !row.enabled || row.digest != plug.Manifest.DigestSHA256 {
		return Call{}, Plugin{}, ErrClosed
	}
	ceiling := setOf(plug.Manifest.Permissions)
	for _, perm := range need {
		if _, ok := ceiling[perm]; !ok || !slices.Contains(row.perms, perm) {
			return Call{}, Plugin{}, ErrDenied
		}
	}
	return Call{Principal: p, Grant: grantOf(need)}, plug, nil
}

func needPerms(class, specific string) []string {
	if specific == "" || specific == class {
		return []string{class}
	}
	return []string{class, specific}
}

// Evaluate asks a step plugin whether the operation is currently allowed.
func (m *Module) Evaluate(ctx context.Context, p tenant.Principal, pluginID string, in StepRequest) (StepDecision, error) {
	return m.step(ctx, p, pluginID, in, fence.PermStepsEvaluate, func(call Call, plug Plugin) (StepDecision, error) {
		return plug.Steps.Evaluate(ctx, call, in)
	})
}

// Request asks a step plugin to admit a new attempt.
func (m *Module) Request(ctx context.Context, p tenant.Principal, pluginID string, in StepRequest) (StepDecision, error) {
	return m.step(ctx, p, pluginID, in, fence.PermStepsRequest, func(call Call, plug Plugin) (StepDecision, error) {
		return plug.Steps.Request(ctx, call, in)
	})
}

// ApplyResult asks a step plugin to judge a terminal claim.
func (m *Module) ApplyResult(ctx context.Context, p tenant.Principal, pluginID string, in StepResult) (StepDecision, error) {
	return m.step(ctx, p, pluginID, in.StepRequest, fence.PermStepsApply, func(call Call, plug Plugin) (StepDecision, error) {
		return plug.Steps.ApplyResult(ctx, call, in)
	})
}

func (m *Module) step(ctx context.Context, p tenant.Principal, pluginID string, in StepRequest, class string, fn func(Call, Plugin) (StepDecision, error)) (StepDecision, error) {
	plug, ok := m.reg.plugin(pluginID)
	if !ok {
		return StepDecision{}, notFound("plugin not found")
	}
	perm, ok := plug.StepPermissions[in.Operation]
	if !ok || plug.Steps == nil {
		return StepDecision{}, invalid("unknown workflow step")
	}
	call, plug, err := m.bind(ctx, p, pluginID, needPerms(class, perm))
	if err != nil {
		return StepDecision{}, err
	}
	return fn(call, plug)
}

// NodeKinds returns the declared kinds when the tenant grant allows it.
// A contributor that drifts from the manifest fails closed.
func (m *Module) NodeKinds(ctx context.Context, p tenant.Principal, pluginID string) ([]NodeKind, error) {
	plug, ok := m.reg.plugin(pluginID)
	if !ok || plug.Kinds == nil {
		return nil, notFound("plugin has no node kinds")
	}
	call, plug, err := m.bind(ctx, p, pluginID, []string{fence.PermNodesContribute})
	if err != nil {
		return nil, err
	}
	got, err := plug.Kinds.NodeKinds(ctx, call)
	if err != nil {
		return nil, err
	}
	if !sameKinds(got, plug.Manifest.NodeKinds) {
		return nil, ErrClosed
	}
	return got, nil
}

// Views returns the declared views when the tenant grant allows it.
func (m *Module) Views(ctx context.Context, p tenant.Principal, pluginID string) ([]View, error) {
	plug, ok := m.reg.plugin(pluginID)
	if !ok || plug.Views == nil {
		return nil, notFound("plugin has no views")
	}
	call, plug, err := m.bind(ctx, p, pluginID, []string{fence.PermViewsProvide})
	if err != nil {
		return nil, err
	}
	got, err := plug.Views.Views(ctx, call)
	if err != nil {
		return nil, err
	}
	if !sameViews(got, plug.Manifest.Views) {
		return nil, ErrClosed
	}
	return got, nil
}

// InvokeTool calls one declared tool with a grant narrowed to that tool.
func (m *Module) InvokeTool(ctx context.Context, p tenant.Principal, pluginID, toolID string, input any) (any, error) {
	plug, ok := m.reg.plugin(pluginID)
	if !ok || plug.Tools == nil {
		return nil, notFound("plugin has no agent tools")
	}
	perm, ok := capabilityPerm(plug.Manifest.AgentTools, toolID)
	if !ok {
		return nil, notFound("agent tool not found")
	}
	call, plug, err := m.bind(ctx, p, pluginID, needPerms(fence.PermToolsInvoke, perm))
	if err != nil {
		return nil, err
	}
	return plug.Tools.Invoke(authz.BindPool(tenant.WithPrincipal(ctx, p), m.pool), call, toolID, input)
}

// CallIntegration calls one declared integration with a narrowed grant.
func (m *Module) CallIntegration(ctx context.Context, p tenant.Principal, pluginID, integrationID string, input any) (any, error) {
	plug, ok := m.reg.plugin(pluginID)
	if !ok || plug.Integrations == nil {
		return nil, notFound("plugin has no integrations")
	}
	perm, ok := capabilityPerm(plug.Manifest.Integrations, integrationID)
	if !ok {
		return nil, notFound("integration not found")
	}
	call, plug, err := m.bind(ctx, p, pluginID, needPerms(fence.PermIntegrationsCall, perm))
	if err != nil {
		return nil, err
	}
	return plug.Integrations.Call(ctx, call, integrationID, input)
}

func capabilityPerm(items []Capability, id string) (string, bool) {
	for _, item := range items {
		if item.ID == id {
			return item.Permission, true
		}
	}
	return "", false
}

func sameKinds(got, want []NodeKind) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i].Slug != want[i].Slug || !slices.Equal(got[i].AllowedChildKinds, want[i].AllowedChildKinds) {
			return false
		}
		if !bytes.Equal(bytes.TrimSpace(got[i].FieldSchema), bytes.TrimSpace(want[i].FieldSchema)) {
			return false
		}
	}
	return true
}

func sameViews(got, want []View) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i].ID != want[i].ID || !slices.Equal(got[i].Panels, want[i].Panels) {
			return false
		}
	}
	return true
}
