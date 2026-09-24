// SPDX-License-Identifier: AGPL-3.0-only

package journey

import (
	"testing"
	"time"
)

func TestDeriveStagesAndNextAction(t *testing.T) {
	release := func(state string, access bool) *releaseFacts {
		return &releaseFacts{ID: "rel", Number: 1, State: state, AccessRequired: access}
	}
	live := func(id string) gateOffer { return gateOffer{ID: id, DecidedBy: "person", Live: true} }
	cases := []struct {
		name      string
		facts     facts
		stage     string
		next      string
		available bool
		reason    string
		shape     string
		access    string
		live      string
		blocked   bool
	}{
		{
			name:  "inspire until a brief is accepted",
			facts: facts{Profile: "personal"},
			stage: stageInspire, next: actionContinueIntake, available: true,
			shape: "later", access: "later", live: "later",
		},
		{
			name:  "confirm an accepted brief",
			facts: facts{Profile: "professional", AcceptedBrief: true},
			stage: stageInspire, next: actionConfirmBrief, available: true,
			shape: "later", access: "later", live: "later",
		},
		{
			name:  "personal skips shape after the brief",
			facts: facts{Profile: "personal", BriefConfirmed: true, RequirementCount: 1, Requirements: live("req")},
			stage: stageRequirements, next: actionApproveRequirements, available: true,
			shape: "skipped", access: "later", live: "later",
		},
		{
			name:  "professional shape waits for a gate",
			facts: facts{Profile: "professional", BriefConfirmed: true},
			stage: stageShape, next: actionDecide, reason: reasonShapeGate,
			shape: "current", access: "later", live: "later",
		},
		{
			name:  "park stays on shape",
			facts: facts{Profile: "personal", BriefConfirmed: true, Decision: "park", Shape: live("again")},
			stage: stageShape, next: actionReopen, available: true,
			shape: "current", access: "later", live: "later",
		},
		{
			name:  "go without an agreement returns to requirements",
			facts: facts{Profile: "professional", BriefConfirmed: true, Decision: "go"},
			stage: stageRequirements, next: actionApproveRequirements, reason: reasonReqNone,
			shape: "done", access: "later", live: "later",
		},
		{
			name: "manual scope change needs a fresh agreement",
			facts: facts{
				Profile: "professional", BriefConfirmed: true, Decision: "go",
				RequirementsRevision: 2, AgreedRequirementsRevision: 2, RequirementCount: 1,
				Release: release("planning", false), ScopeRevision: true,
			},
			stage: stageRequirements, next: actionApproveRequirements, reason: reasonScope,
			shape: "done", access: "later", live: "later",
		},
		{
			name: "plan without tickets",
			facts: facts{
				Profile: "personal", BriefConfirmed: true,
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release: release("planning", false),
			},
			stage: stagePlan, next: actionStartBuild, reason: reasonNoTickets,
			shape: "skipped", access: "skipped", live: "later",
		},
		{
			name: "professional cap blocks the plan",
			facts: facts{
				Profile: "professional", BriefConfirmed: true, Decision: "go",
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release: release("planning", false), IncludedTickets: 1,
				HasCap: true, CapCents: 1000, PlanCents: 1200, Build: live("build"),
			},
			stage: stagePlan, next: actionStartBuild, reason: reasonOverCap,
			shape: "done", access: "skipped", live: "later",
		},
		{
			name: "spent hours count against the cap",
			facts: facts{
				Profile: "enterprise", BriefConfirmed: true, Decision: "go",
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release: release("planning", false), IncludedTickets: 1,
				HasCap: true, CapCents: 1000, PlanCents: 600, SpentCents: 500,
			},
			stage: stagePlan, next: actionStartBuild, reason: reasonOverCap,
			shape: "done", access: "skipped", live: "later",
		},
		{
			name: "start build when the gate and the cap agree",
			facts: facts{
				Profile: "professional", BriefConfirmed: true, Decision: "reduce_scope",
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release: release("planning", false), IncludedTickets: 2,
				HasCap: true, CapCents: 1000, PlanCents: 400, SpentCents: 500, Build: live("build"),
			},
			stage: stagePlan, next: actionStartBuild, available: true,
			shape: "done", access: "skipped", live: "later",
		},
		{
			name: "active build is a passive wait",
			facts: facts{
				Profile: "personal", BriefConfirmed: true,
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release: release("building", false), IncludedTickets: 1, OpenReleaseTickets: 1,
			},
			stage: stageBuild, next: actionWaitForBuild, reason: reasonBuilding,
			shape: "skipped", access: "skipped", live: "later",
		},
		{
			name: "enterprise reviewer must be someone else",
			facts: facts{
				Profile: "enterprise", BriefConfirmed: true, Decision: "go",
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release:     release("candidate", false),
				BriefAuthor: "author", BuildStarter: "builder",
				Candidate: gateOffer{ID: "cand", DecidedBy: "builder", Live: true},
			},
			stage: stageBuild, next: actionApproveCandidate, reason: reasonReviewer,
			shape: "done", access: "skipped", live: "later",
		},
		{
			name: "enterprise drafts block the candidate",
			facts: facts{
				Profile: "enterprise", BriefConfirmed: true, Decision: "go",
				RequirementsRevision: 1, AgreedRequirementsRevision: 1, DraftCount: 1,
				Release:     release("candidate", false),
				BriefAuthor: "author", BuildStarter: "builder",
				Candidate: live("cand"),
			},
			stage: stageBuild, next: actionApproveCandidate, reason: reasonDrafts,
			shape: "done", access: "skipped", live: "later",
		},
		{
			name: "independent candidate approval",
			facts: facts{
				Profile: "enterprise", BriefConfirmed: true, Decision: "go",
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release:     release("candidate", false),
				BriefAuthor: "author", BuildStarter: "builder", RequirementsDecider: "author",
				Candidate: gateOffer{ID: "cand", DecidedBy: "reviewer", Live: true},
			},
			stage: stageBuild, next: actionApproveCandidate, available: true,
			shape: "done", access: "skipped", live: "later",
		},
		{
			name: "missing deployment evidence blocks deploy",
			facts: facts{
				Profile: "personal", BriefConfirmed: true,
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release: release("deploying", false), DeployGateID: "dep",
			},
			stage: stageDeploy, next: actionApproveDeploy, reason: reasonDeployEvidence, blocked: true,
			shape: "skipped", access: "skipped", live: "later",
		},
		{
			name: "failed deploy offers a retry",
			facts: facts{
				Profile: "personal", BriefConfirmed: true,
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release: release("deploying", false), DeployOutcome: outcomeFailed,
				Deploy: live("again"),
			},
			stage: stageDeploy, next: actionRetryDeploy, available: true, blocked: true,
			shape: "skipped", access: "skipped", live: "later",
		},
		{
			name: "successful deploy without access is live",
			facts: facts{
				Profile: "personal", BriefConfirmed: true,
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release: release("deploying", false), DeployGateID: "dep",
				DeployOutcome: outcomeSucceeded, VerifyOutcome: outcomeSucceeded,
			},
			stage: stageLive, next: actionPlanNext, available: true,
			shape: "skipped", access: "skipped", live: "current",
		},
		{
			name: "access change is not skipped",
			facts: facts{
				Profile: "personal", BriefConfirmed: true,
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release: release("deploying", true), DeployGateID: "dep",
				DeployOutcome: outcomeSucceeded, VerifyOutcome: outcomeSucceeded,
				Access: live("permit"),
			},
			stage: stageAccess, next: actionApprovePermit, available: true,
			shape: "skipped", access: "current", live: "later",
		},
		{
			name: "prior release stays live while the next plan is current",
			facts: facts{
				Profile: "personal", BriefConfirmed: true,
				RequirementsRevision: 1, AgreedRequirementsRevision: 1,
				Release:       &releaseFacts{ID: "rel2", Number: 2, State: "planning"},
				PriorReleased: true, IncludedTickets: 1, Build: live("build"),
			},
			stage: stagePlan, next: actionStartBuild, available: true,
			shape: "skipped", access: "skipped", live: "done",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := derive(tc.facts)
			if got.Stage != tc.stage || got.NextAction.Key != tc.next || got.NextAction.Available != tc.available || got.NextAction.Reason != tc.reason {
				t.Fatalf("stage %s next %s available %v reason %q", got.Stage, got.NextAction.Key, got.NextAction.Available, got.NextAction.Reason)
			}
			if state(got, stageShape) != tc.shape || state(got, stageAccess) != tc.access || state(got, stageLive) != tc.live {
				t.Fatalf("shape %s access %s live %s", state(got, stageShape), state(got, stageAccess), state(got, stageLive))
			}
			wantState := "current"
			if tc.blocked {
				wantState = "blocked"
			}
			if state(got, tc.stage) != wantState {
				t.Fatalf("stage state %s", state(got, tc.stage))
			}
			if len(got.Stages) != 8 {
				t.Fatalf("stages %d", len(got.Stages))
			}
		})
	}
}

func TestFoldHandoffsUsesTheAttemptWindow(t *testing.T) {
	rows := []handoffRow{
		{ID: "old-verify", Stage: "deploy", Operation: "verify", State: "failed", Result: "failed", Attempt: 1, At: unix(10)},
		{ID: "deploy", Stage: "deploy", Operation: "deploy", State: "succeeded", Result: "succeeded", Attempt: 1, At: unix(30)},
		{ID: "verify", Stage: "deploy", Operation: "verify", State: "succeeded", Result: "succeeded", Attempt: 2, At: unix(31)},
		{ID: "apply", Stage: "access", Operation: "apply", State: "succeeded", Result: "succeeded", Attempt: 1, At: unix(40)},
	}
	deployID, accessID, deployOutcome, verifyOutcome, accessOutcome := foldHandoffs(rows, unix(20), unix(0))
	if deployID != "verify" && deployID != "deploy" {
		t.Fatalf("deploy handoff %s", deployID)
	}
	if deployOutcome != outcomeSucceeded || verifyOutcome != outcomeSucceeded || accessOutcome != outcomeSucceeded || accessID != "apply" {
		t.Fatalf("outcomes %s %s %s access %s", deployOutcome, verifyOutcome, accessOutcome, accessID)
	}
	_, _, deployOutcome, verifyOutcome, _ = foldHandoffs(rows, unix(0), unix(0))
	if verifyOutcome != outcomeSucceeded {
		t.Fatalf("latest verify attempt should win, got %s outcome %s", verifyOutcome, deployOutcome)
	}
}

func state(j Journey, key string) string {
	for _, stage := range j.Stages {
		if stage.Key == key {
			return stage.State
		}
	}
	return ""
}

func unix(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}
