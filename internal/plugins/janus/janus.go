// SPDX-License-Identifier: AGPL-3.0-only

// Package janus declares the first-party Access stage ceiling.
package janus

import (
	"context"
	"errors"
	"github.com/inspr-at/aeon/internal/plugins"
)

func Manifest() plugins.Manifest {
	return plugins.Seal(plugins.Manifest{ID: "janus", Version: "1", Owner: "internal/plugins/janus", Permissions: []string{"prepare", "apply"}, NodeKinds: []plugins.NodeKind{}, Views: []plugins.View{{ID: "access", Panels: []string{"preparation", "permit", "application"}}}, WorkflowSteps: []plugins.WorkflowStep{{Key: "prepare", Gates: []string{}}, {Key: "apply", Gates: []string{"access"}}}, AgentTools: []plugins.Capability{}, Integrations: []plugins.Capability{{ID: "janus_preparation", Permission: "prepare"}, {ID: "janus_application", Permission: "apply"}}, BackgroundJobs: []plugins.Capability{}})
}

type Plugin struct{}

func (Plugin) Evaluate(_ context.Context, c plugins.Call, operation string) error {
	if (operation != "prepare" && operation != "apply") || !c.Capabilities.Has(operation) {
		return errors.New("janus operation is not installed")
	}
	return nil
}
func (p Plugin) Request(ctx context.Context, c plugins.Call, operation string) error {
	return p.Evaluate(ctx, c, operation)
}
func (p Plugin) ApplyResult(ctx context.Context, c plugins.Call, operation string) error {
	return p.Evaluate(ctx, c, operation)
}

var _ plugins.StepPlugin = Plugin{}

func (Plugin) Integrations(_ context.Context, _ plugins.Call) []plugins.Capability {
	return Manifest().Integrations
}

var _ plugins.IntegrationProvider = Plugin{}

// Register installs the compiled Janus manifest and implementation.
func Register(r *plugins.Registry) error {
	if err := r.Register(Manifest()); err != nil {
		return err
	}
	return r.BindStep("janus", Plugin{})
}
