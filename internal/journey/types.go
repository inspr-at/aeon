// SPDX-License-Identifier: AGPL-3.0-only

package journey

// Gate and approval scope names are the R2 contract for journey decisions.
// An agent proposes the scope on the project or release node; a person decides
// it through the approvals API. This package never writes that decision.
const (
	GateShape        = "shape"
	GateRequirements = "requirements"
	GateBuild        = "build"
	GateCandidate    = "candidate"
	GateDeploy       = "deploy"
	GateAccess       = "access"

	ScopeShape        = "journey.shape"
	ScopeRequirements = "journey.requirements"
	ScopeBuild        = "journey.build"
	ScopeCandidate    = "journey.candidate"
	ScopeDeploy       = "journey.deploy"
	ScopeAccess       = "journey.access"
)

// Journey is the derived projection returned by the journey routes.
type Journey struct {
	ProjectNodeID        string            `json:"project_node_id"`
	Profile              string            `json:"profile"`
	Revision             int64             `json:"revision"`
	Stage                string            `json:"stage"`
	StageSource          string            `json:"stage_source"`
	Stages               []JourneyStage    `json:"stages"`
	NextAction           JourneyNextAction `json:"next_action"`
	RequirementsRevision int64             `json:"requirements_revision"`
	RequirementsDigest   string            `json:"requirements_digest_sha256"`
	RequirementsScope    string            `json:"requirements_approval_scope"`
	LaunchReadiness      LaunchReadiness   `json:"launch_readiness"`
	CurrentReleaseID     *string           `json:"current_release_id"`
}

type LaunchReadiness struct {
	CanAdmit bool   `json:"can_admit"`
	Reason   string `json:"reason"`
}

// JourneyStage is one step on the eight-stage rail.
type JourneyStage struct {
	Key            string  `json:"key"`
	State          string  `json:"state"`
	GateApprovalID *string `json:"gate_approval_id"`
	HandoffID      *string `json:"handoff_id"`
}

// JourneyNextAction is the single call to action for the project.
type JourneyNextAction struct {
	Key               string  `json:"key"`
	Label             string  `json:"label"`
	Stage             string  `json:"stage"`
	Available         bool    `json:"available"`
	Reason            string  `json:"reason,omitempty"`
	ApprovalRequestID *string `json:"approval_request_id"`
}

type profileWrite struct {
	Profile          string `json:"profile"`
	ExpectedRevision int64  `json:"expected_revision"`
}

type actionWrite struct {
	Action            string  `json:"action"`
	ExpectedRevision  int64   `json:"expected_revision"`
	IdempotencyKey    string  `json:"idempotency_key"`
	ApprovalRequestID *string `json:"approval_request_id"`
	ReleaseID         *string `json:"release_id"`
	Reason            *string `json:"reason"`
}

type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

func fail(status int, msg string) error {
	return &httpError{status: status, msg: msg}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

func ptrVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
