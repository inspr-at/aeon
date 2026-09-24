// SPDX-License-Identifier: AGPL-3.0-only

package plugins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/inspr-at/aeon/internal/plugins/janus"
	"github.com/inspr-at/aeon/internal/plugins/pharos"
)

// Builtin returns a sealed registry containing Pharos, Janus and any extra
// first-party plugins. Extra plugins live in packages that import this one, so
// cmd/aeon passes their constructors in.
func Builtin(extra ...func() (Plugin, error)) (*Registry, error) {
	return BuiltinWithRegistration(nil, extra...)
}

// BuiltinWithRegistration runs after the first-party manifests are registered
// and before sealing. It lets a dependent job provider bind the same registry.
func BuiltinWithRegistration(register func(*Registry) error, extra ...func() (Plugin, error)) (*Registry, error) {
	reg := NewRegistry()
	for _, build := range append([]func() (Plugin, error){pharosPlugin, janusPlugin}, extra...) {
		plug, err := build()
		if err != nil {
			return nil, err
		}
		if err := reg.Register(plug); err != nil {
			return nil, err
		}
	}
	if register != nil {
		if err := register(reg); err != nil {
			return nil, err
		}
	}
	reg.Seal()
	return reg, nil
}

func pharosPlugin() (Plugin, error) {
	p := Plugin{
		Manifest: Manifest{
			ID:      pharos.ID,
			Version: pharos.Version,
			Owner:   pharos.Owner,
			Permissions: []string{
				fence.PermStepsEvaluate, fence.PermStepsRequest, fence.PermStepsApply,
				fence.PermIntegrationsCall, fence.PermStageDeploy, fence.PermStageVerify,
			},
			WorkflowSteps: []WorkflowStep{
				{Key: pharos.OperationDeploy, Gates: []string{
					fence.GateArtifactIdentity, fence.GateBackupReady, fence.GateReadiness,
					fence.GateLaunchAdmission, fence.GatePersonDecision, fence.GatePrerequisiteSeal,
				}},
				{Key: pharos.OperationVerify, Gates: []string{fence.GateArtifactIdentity, fence.GateReadiness}},
			},
			Integrations: []Capability{
				{ID: "pharos_deploy", Permission: fence.PermStageDeploy},
				{ID: "pharos_verify", Permission: fence.PermStageVerify},
			},
		},
		StepPermissions: map[string]string{
			pharos.OperationDeploy: fence.PermStageDeploy,
			pharos.OperationVerify: fence.PermStageVerify,
		},
		Steps:        pharosSteps{},
		Integrations: pharosIntegration{},
	}
	return sealDigest(p)
}

func janusPlugin() (Plugin, error) {
	p := Plugin{
		Manifest: Manifest{
			ID:      janus.ID,
			Version: janus.Version,
			Owner:   janus.Owner,
			Permissions: []string{
				fence.PermStepsEvaluate, fence.PermStepsRequest, fence.PermStepsApply,
				fence.PermIntegrationsCall, fence.PermStageAccessPrepare, fence.PermStageAccessApply,
			},
			WorkflowSteps: []WorkflowStep{
				{Key: janus.OperationPrepare, Gates: []string{fence.GateObservedState}},
				{Key: janus.OperationApply, Gates: []string{
					fence.GateDeploymentSucceeded, fence.GateBoundedPermit, fence.GatePersonDecision,
				}},
			},
			Integrations: []Capability{
				{ID: "janus_prepare", Permission: fence.PermStageAccessPrepare},
				{ID: "janus_apply", Permission: fence.PermStageAccessApply},
			},
		},
		StepPermissions: map[string]string{
			janus.OperationPrepare: fence.PermStageAccessPrepare,
			janus.OperationApply:   fence.PermStageAccessApply,
		},
		Steps:        janusSteps{},
		Integrations: janusIntegration{},
	}
	return sealDigest(p)
}

func sealDigest(p Plugin) (Plugin, error) {
	sum, err := Digest(p)
	if err != nil {
		return Plugin{}, err
	}
	p.Manifest.DigestSHA256 = sum
	return p, nil
}

type pharosSteps struct{}

func (pharosSteps) Evaluate(ctx context.Context, call Call, in StepRequest) (StepDecision, error) {
	return pharosCall(ctx, call, in, fence.PermStepsEvaluate, func(facts pharos.Facts) (pharos.Decision, error) {
		return pharos.Evaluate(facts)
	})
}

func (pharosSteps) Request(ctx context.Context, call Call, in StepRequest) (StepDecision, error) {
	return pharosCall(ctx, call, in, fence.PermStepsRequest, func(facts pharos.Facts) (pharos.Decision, error) {
		return pharos.Request(facts)
	})
}

func (pharosSteps) ApplyResult(ctx context.Context, call Call, in StepResult) (StepDecision, error) {
	if err := ready(ctx, in.Stage, pharos.Stage); err != nil {
		return StepDecision{}, err
	}
	facts, ok := in.Payload.(pharos.Facts)
	evidence, okEv := in.Evidence.(pharos.Evidence)
	if !ok || !okEv || facts.Operation != in.Operation {
		return StepDecision{}, invalid("pharos result is required")
	}
	if !allows(call, fence.PermStepsApply, pharosPerm(facts.Operation)) {
		return StepDecision{}, ErrDenied
	}
	decision, err := pharos.Apply(facts, in.ClaimedOutcome, evidence)
	if err != nil {
		return StepDecision{}, invalid(err.Error())
	}
	return mapPharos(decision)
}

func pharosCall(ctx context.Context, call Call, in StepRequest, class string, fn func(pharos.Facts) (pharos.Decision, error)) (StepDecision, error) {
	if err := ready(ctx, in.Stage, pharos.Stage); err != nil {
		return StepDecision{}, err
	}
	facts, ok := in.Payload.(pharos.Facts)
	if !ok || facts.Operation != in.Operation {
		return StepDecision{}, invalid("pharos facts are required")
	}
	if !allows(call, class, pharosPerm(facts.Operation)) {
		return StepDecision{}, ErrDenied
	}
	decision, err := fn(facts)
	if err != nil {
		return StepDecision{}, invalid(err.Error())
	}
	return mapPharos(decision)
}

func pharosPerm(operation string) string {
	if operation == pharos.OperationDeploy {
		return fence.PermStageDeploy
	}
	return fence.PermStageVerify
}

func mapPharos(decision pharos.Decision) (StepDecision, error) {
	raw, err := marshalEvidence(decision.Evidence)
	if err != nil {
		return StepDecision{}, err
	}
	return StepDecision{
		Proceed:                decision.Proceed,
		Outcome:                decision.Outcome,
		Blocker:                decision.Blocker,
		EvidenceCeiling:        cloneStrings(decision.EvidenceCeiling),
		ConsumeLaunchAdmission: decision.ConsumeLaunchAdmission,
		Evidence:               raw,
	}, nil
}

type pharosIntegration struct{}

func (pharosIntegration) Call(ctx context.Context, call Call, integrationID string, input any) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	facts, ok := input.(pharos.Facts)
	if !ok {
		return nil, invalid("pharos facts are required")
	}
	var perm string
	switch integrationID {
	case "pharos_deploy":
		perm = fence.PermStageDeploy
		if facts.Operation != pharos.OperationDeploy {
			return nil, invalid("operation does not match integration")
		}
	case "pharos_verify":
		perm = fence.PermStageVerify
		if facts.Operation != pharos.OperationVerify {
			return nil, invalid("operation does not match integration")
		}
	default:
		return nil, notFound("integration not found")
	}
	if !allows(call, fence.PermIntegrationsCall, perm) {
		return nil, ErrDenied
	}
	decision, err := pharos.Evaluate(facts)
	if err != nil {
		return nil, invalid(err.Error())
	}
	return mapPharos(decision)
}

type janusSteps struct{}

func (janusSteps) Evaluate(ctx context.Context, call Call, in StepRequest) (StepDecision, error) {
	return janusCall(ctx, call, in, fence.PermStepsEvaluate, false)
}

func (janusSteps) Request(ctx context.Context, call Call, in StepRequest) (StepDecision, error) {
	return janusCall(ctx, call, in, fence.PermStepsRequest, true)
}

func (janusSteps) ApplyResult(ctx context.Context, call Call, in StepResult) (StepDecision, error) {
	if err := ready(ctx, in.Stage, janus.Stage); err != nil {
		return StepDecision{}, err
	}
	facts, _, err := janusInputs(call, in.StepRequest, fence.PermStepsApply)
	if err != nil {
		return StepDecision{}, err
	}
	got, ok := in.Evidence.([]janus.Evidence)
	if !ok {
		return StepDecision{}, invalid("janus evidence is required")
	}
	decision, err := janus.Apply(call.Principal.ID, facts, in.ClaimedOutcome, got)
	if err != nil {
		return StepDecision{}, invalid(err.Error())
	}
	return mapJanus(decision)
}

func janusCall(ctx context.Context, call Call, in StepRequest, class string, request bool) (StepDecision, error) {
	if err := ready(ctx, in.Stage, janus.Stage); err != nil {
		return StepDecision{}, err
	}
	facts, _, err := janusInputs(call, in, class)
	if err != nil {
		return StepDecision{}, err
	}
	var decision janus.Decision
	if request {
		decision, err = janus.Request(call.Principal.ID, facts)
	} else {
		decision, err = janus.Evaluate(call.Principal.ID, facts)
	}
	if err != nil {
		return StepDecision{}, invalid(err.Error())
	}
	return mapJanus(decision)
}

func janusInputs(call Call, in StepRequest, class string) (janus.Facts, string, error) {
	facts, ok := in.Payload.(janus.Facts)
	if !ok || facts.Operation != in.Operation {
		return janus.Facts{}, "", invalid("janus facts are required")
	}
	perm := fence.PermStageAccessPrepare
	if facts.Operation == janus.OperationApply {
		perm = fence.PermStageAccessApply
	}
	if !allows(call, class, perm) {
		return janus.Facts{}, "", ErrDenied
	}
	return facts, perm, nil
}

func mapJanus(decision janus.Decision) (StepDecision, error) {
	raw, err := marshalEvidence(decision.Evidence)
	if err != nil {
		return StepDecision{}, err
	}
	return StepDecision{
		Proceed:          decision.Proceed,
		Outcome:          decision.Outcome,
		Blocker:          decision.Blocker,
		ConsumePermit:    decision.ConsumePermit,
		PrerequisiteSeal: decision.Seal,
		Evidence:         raw,
	}, nil
}

type janusIntegration struct{}

func (janusIntegration) Call(ctx context.Context, call Call, integrationID string, input any) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	facts, ok := input.(janus.Facts)
	if !ok {
		return nil, invalid("janus facts are required")
	}
	var perm string
	switch integrationID {
	case "janus_prepare":
		perm = fence.PermStageAccessPrepare
		if facts.Operation != janus.OperationPrepare {
			return nil, invalid("operation does not match integration")
		}
	case "janus_apply":
		perm = fence.PermStageAccessApply
		if facts.Operation != janus.OperationApply {
			return nil, invalid("operation does not match integration")
		}
	default:
		return nil, notFound("integration not found")
	}
	if !allows(call, fence.PermIntegrationsCall, perm) {
		return nil, ErrDenied
	}
	decision, err := janus.Evaluate(call.Principal.ID, facts)
	if err != nil {
		return nil, invalid(err.Error())
	}
	return mapJanus(decision)
}

func ready(ctx context.Context, got, want string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if got != "" && got != want {
		return invalid("stage does not match plugin")
	}
	return nil
}

func allows(call Call, class, specific string) bool {
	if !call.Grant.Allows(class) {
		return false
	}
	return specific == "" || specific == class || call.Grant.Allows(specific)
}

func marshalEvidence(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("plugins: evidence: %w", err)
	}
	if string(raw) == "null" {
		return nil, nil
	}
	return raw, nil
}
