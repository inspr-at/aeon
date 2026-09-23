// SPDX-License-Identifier: AGPL-3.0-only

// Package pharos declares the first-party Deploy stage ceiling.
package pharos

import (
	"context"
	"errors"
	"github.com/inspr-at/aeon/internal/plugins"
)

func Manifest() plugins.Manifest {
	return plugins.Seal(plugins.Manifest{ID: "pharos", Version: "1", Owner: "internal/plugins/pharos", Permissions: []string{"deploy", "verify"}, NodeKinds: []plugins.NodeKind{}, Views: []plugins.View{{ID: "deploy", Panels: []string{"deployment", "verification"}}}, WorkflowSteps: []plugins.WorkflowStep{{Key: "deploy", Gates: []string{"candidate", "deploy"}}, {Key: "verify", Gates: []string{"deploy"}}}, AgentTools: []plugins.Capability{}, Integrations: []plugins.Capability{{ID: "pharos_deployment", Permission: "deploy"}, {ID: "pharos_verification", Permission: "verify"}}, BackgroundJobs: []plugins.Capability{}})
}

type Plugin struct{}

func (Plugin) Evaluate(_ context.Context, c plugins.Call, operation string) error {
	if (operation != "deploy" && operation != "verify") || !c.Capabilities.Has(operation) {
		return errors.New("pharos operation is not installed")
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

// Register installs the compiled Pharos manifest and implementation.
func Register(r *plugins.Registry) error {
	if err := r.Register(Manifest()); err != nil {
		return err
	}
	return r.BindStep("pharos", Plugin{})
}
