// SPDX-License-Identifier: AGPL-3.0-only

package plugins

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/inspr-at/aeon/internal/tenant"
)

// Call is the only value an extension receives beside its own arguments.
// It has no pool, environment, or HTTP client.
type Call struct {
	Principal tenant.Principal
	Grant     Grant
}

// Grant is the permission subset for one call. The set cannot be widened.
type Grant struct {
	perms map[string]struct{}
}

// Allows reports whether this call was granted permission.
func (g Grant) Allows(permission string) bool {
	_, ok := g.perms[permission]
	return ok
}

// Names returns the granted permissions in sorted order.
func (g Grant) Names() []string {
	out := make([]string, 0, len(g.perms))
	for name := range g.perms {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func grantOf(names []string) Grant {
	g := Grant{perms: make(map[string]struct{}, len(names))}
	for _, name := range names {
		g.perms[name] = struct{}{}
	}
	return g
}

// NodeKindContributor returns the node kinds declared by the manifest.
type NodeKindContributor interface {
	NodeKinds(ctx context.Context, call Call) ([]NodeKind, error)
}

// ViewProvider returns the views declared by the manifest.
type ViewProvider interface {
	Views(ctx context.Context, call Call) ([]View, error)
}

// StepPlugin is a stage operation. Evaluate inspects current facts, Request
// asks the host to admit a new attempt, and ApplyResult judges a terminal claim.
type StepPlugin interface {
	Evaluate(ctx context.Context, call Call, in StepRequest) (StepDecision, error)
	Request(ctx context.Context, call Call, in StepRequest) (StepDecision, error)
	ApplyResult(ctx context.Context, call Call, in StepResult) (StepDecision, error)
}

// ToolProvider invokes one declared agent tool.
type ToolProvider interface {
	Invoke(ctx context.Context, call Call, toolID string, input any) (any, error)
}

// IntegrationProvider calls one declared integration. Implementations stay in process.
type IntegrationProvider interface {
	Call(ctx context.Context, call Call, integrationID string, input any) (any, error)
}

// JobProvider runs one declared background job under the host's lease.
type JobProvider interface {
	Run(ctx context.Context, call Call, jobID string) (JobResult, error)
}

// JobResult is the durable outcome of a job. Error text is not stored.
type JobResult struct {
	Outcome string
}

// StepRequest is one stage operation. Payload is the plugin's typed facts.
type StepRequest struct {
	Stage     string
	Operation string
	Payload   any
}

// StepResult is a terminal claim against a step request.
type StepResult struct {
	StepRequest
	ClaimedOutcome string
	Evidence       any
}

// StepDecision is a plugin result. AdvancesRelease and AdvancesAccess stay
// false for the built-in plugins: a plugin cannot mint a release or skip a
// person decision. Evidence is safe metadata only.
type StepDecision struct {
	Proceed                bool
	Outcome                string
	Blocker                string
	EvidenceCeiling        []string
	ConsumeLaunchAdmission bool
	ConsumePermit          bool
	AdvancesRelease        bool
	AdvancesAccess         bool
	PrerequisiteSeal       string
	Evidence               json.RawMessage
}
