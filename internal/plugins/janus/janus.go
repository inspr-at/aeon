// SPDX-License-Identifier: AGPL-3.0-only

// Package janus is the compiled Access plugin. Prepare may run before
// deployment and does not advance Access. Apply follows a successful
// deployment and a person-approved bounded permit. Evidence is only the
// authorized and credential_ready booleans plus the observed time.
package janus

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/inspr-at/paimos/internal/plugins/fence"
)

const (
	// ID is the registry id.
	ID = "janus"
	// Version is the compiled manifest version.
	Version = "1"
	// Owner is the accountable first-party owner.
	Owner = "janus"
	// OperationPrepare records access checks before deployment.
	OperationPrepare = "prepare"
	// OperationApply records access checks after deployment and a bounded permit.
	OperationApply = "apply"
	// Stage is the journey stage these operations belong to.
	Stage = "access"
)

// Permit is the person-approved bound. It carries no principal, grant, secret,
// path, digest, command, or URL.
type Permit struct {
	Approved bool
	Bounded  bool
	Expired  bool
	Consumed bool
}

// Forbidden is material Janus must refuse rather than copy into evidence.
type Forbidden struct {
	PrincipalIDs []string
	Grants       []string
	Secrets      []string
	Paths        []string
	Digests      []string
	Commands     []string
	URLs         []string
	Text         string
}

// Facts is the Access observation. It is not evidence: evidence is projected
// separately and drops every field except the two booleans and the time.
type Facts struct {
	Operation           string
	DeploymentSucceeded bool
	PersonApproved      bool
	Permit              Permit
	Authorized          *bool
	CredentialReady     *bool
	ObservedAt          time.Time
	Forbidden           Forbidden
}

// Evidence is one Janus fact. Authorization and credential handoff are separate
// items; neither item carries the other boolean.
type Evidence struct {
	Kind            string    `json:"kind"`
	Authorized      *bool     `json:"authorized,omitempty"`
	CredentialReady *bool     `json:"credential_ready,omitempty"`
	ObservedAt      time.Time `json:"observed_at"`
}

// Decision is a pure result. AdvancesAccess and AdvancesRelease stay false.
type Decision struct {
	Proceed         bool       `json:"proceed"`
	Outcome         string     `json:"outcome,omitempty"`
	Blocker         string     `json:"blocker,omitempty"`
	Evidence        []Evidence `json:"evidence"`
	Seal            string     `json:"-"`
	ConsumePermit   bool       `json:"consume_permit"`
	AdvancesAccess  bool       `json:"advances_access"`
	AdvancesRelease bool       `json:"advances_release"`
}

// Evaluate reports whether the observation may succeed.
func Evaluate(actorID string, facts Facts) (Decision, error) {
	return decide(actorID, facts, false)
}

// Request is Evaluate. An accepted apply asks the host to consume the permit.
func Request(actorID string, facts Facts) (Decision, error) {
	decision, err := decide(actorID, facts, false)
	if err != nil {
		return Decision{}, err
	}
	if decision.Proceed && facts.Operation == OperationApply {
		decision.ConsumePermit = true
	}
	return decision, nil
}

// Apply accepts a terminal success only when the claim matches the booleans
// and the policy. Failed observations stay failed. actorID is never copied.
func Apply(actorID string, facts Facts, claimed string, evidence []Evidence) (Decision, error) {
	if claimed != "succeeded" && claimed != "failed" {
		return Decision{}, fmt.Errorf("invalid outcome")
	}
	decision, err := decide(actorID, facts, true)
	if err != nil {
		return Decision{}, err
	}
	if !evidenceMatch(decision.Evidence, evidence) {
		decision.Proceed = false
		decision.Outcome = "failed"
		if decision.Blocker == "" {
			decision.Blocker = fence.BlockerPolicyRefused
		}
		decision.Evidence = nil
		decision.Seal = ""
		decision.ConsumePermit = false
		return decision, nil
	}
	if claimed == "succeeded" && decision.Proceed {
		decision.Outcome = "succeeded"
		return decision, nil
	}
	decision.Proceed = false
	decision.Outcome = "failed"
	decision.ConsumePermit = false
	if decision.Blocker == "" {
		decision.Blocker = fence.BlockerPolicyRefused
	}
	return decision, nil
}

func decide(actorID string, facts Facts, forApply bool) (Decision, error) {
	_ = actorID
	decision := Decision{Evidence: []Evidence{}}
	if facts.Operation != OperationPrepare && facts.Operation != OperationApply {
		return Decision{}, fmt.Errorf("invalid operation")
	}
	if facts.Forbidden.present() {
		decision.Blocker = fence.BlockerPolicyRefused
		return decision, nil
	}
	if facts.ObservedAt.IsZero() {
		return Decision{}, fmt.Errorf("observed_at is required")
	}
	if facts.Operation == OperationApply {
		if !facts.DeploymentSucceeded {
			decision.Blocker = fence.BlockerDependencyPending
			return decision, nil
		}
		if blocker := permitBlocker(facts, forApply); blocker != "" {
			decision.Blocker = blocker
			return decision, nil
		}
	}
	evidence, err := project(facts)
	if err != nil {
		return Decision{}, err
	}
	if len(evidence) != 2 {
		decision.Blocker = fence.BlockerExternalWaiting
		return decision, nil
	}
	checks := checksOf(evidence)
	seal, err := fence.Seal(checks)
	if err != nil {
		decision.Blocker = fence.BlockerDependencyPending
		return decision, nil
	}
	decision.Evidence = evidence
	decision.Seal = seal
	for _, check := range checks {
		if check.Required && !check.Succeeded {
			decision.Blocker = fence.BlockerPolicyRefused
			return decision, nil
		}
	}
	decision.Proceed = true
	return decision, nil
}

func permitBlocker(facts Facts, consumedMustBe bool) string {
	p := facts.Permit
	if facts.PersonApproved && p.Approved && p.Bounded && !p.Expired && p.Consumed == consumedMustBe {
		return ""
	}
	return fence.BlockerPolicyRefused
}

func project(facts Facts) ([]Evidence, error) {
	var out []Evidence
	if facts.Authorized != nil {
		value := *facts.Authorized
		out = append(out, Evidence{
			Kind:       fence.KindAuthorization,
			Authorized: &value,
			ObservedAt: facts.ObservedAt,
		})
	}
	if facts.CredentialReady != nil {
		value := *facts.CredentialReady
		out = append(out, Evidence{
			Kind:            fence.KindCredentialHandoff,
			CredentialReady: &value,
			ObservedAt:      facts.ObservedAt,
		})
	}
	return out, nil
}

func checksOf(evidence []Evidence) []fence.Check {
	checks := make([]fence.Check, 0, len(evidence))
	for _, item := range evidence {
		switch item.Kind {
		case fence.KindAuthorization:
			checks = append(checks, fence.Check{
				Kind: item.Kind, Required: true, Succeeded: item.Authorized != nil && *item.Authorized,
			})
		case fence.KindCredentialHandoff:
			checks = append(checks, fence.Check{
				Kind: item.Kind, Required: true, Succeeded: item.CredentialReady != nil && *item.CredentialReady,
			})
		}
	}
	return checks
}

func evidenceMatch(want, got []Evidence) bool {
	if len(want) != len(got) {
		return false
	}
	left, right := indexEvidence(want), indexEvidence(got)
	if len(left) != len(want) || len(right) != len(got) {
		return false
	}
	rawLeft, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rawRight, err := json.Marshal(right)
	if err != nil {
		return false
	}
	return string(rawLeft) == string(rawRight)
}

func indexEvidence(items []Evidence) map[string]Evidence {
	out := make(map[string]Evidence, len(items))
	for _, item := range items {
		out[item.Kind] = item
	}
	return out
}

func (f Forbidden) present() bool {
	return len(f.PrincipalIDs)+len(f.Grants)+len(f.Secrets)+len(f.Paths)+len(f.Digests)+len(f.Commands)+len(f.URLs) > 0 || f.Text != ""
}
