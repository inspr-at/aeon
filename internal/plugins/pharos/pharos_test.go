// SPDX-License-Identifier: AGPL-3.0-only

package pharos

import (
	"strings"
	"testing"
	"time"

	"github.com/inspr-at/aeon/internal/plugins/fence"
)

func TestDeployRequiresPersonAdmissionAndIdentity(t *testing.T) {
	facts := readyDeploy()
	decision, err := Request(facts)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Proceed || !decision.ConsumeLaunchAdmission || decision.AdvancesRelease {
		t.Fatalf("deploy request = %#v", decision)
	}
	if decision.Evidence == nil || decision.Evidence.Kind != fence.KindDeployment {
		t.Fatalf("evidence = %#v", decision.Evidence)
	}

	facts.PersonApproved = false
	decision, err = Evaluate(facts)
	if err != nil || decision.Proceed || decision.ConsumeLaunchAdmission || decision.Blocker != fence.BlockerPolicyRefused {
		t.Fatalf("missing person = %#v %v", decision, err)
	}

	facts = readyDeploy()
	facts.EvidenceStale = true
	decision, err = Evaluate(facts)
	if err != nil || decision.Blocker != fence.BlockerReporterStale {
		t.Fatalf("stale = %#v %v", decision, err)
	}

	facts = readyDeploy()
	facts.Artifact.DigestSHA256 = strings.Repeat("ff", 32)
	decision, err = Evaluate(facts)
	if err != nil || decision.Proceed || decision.Blocker != fence.BlockerPolicyRefused {
		t.Fatalf("identity = %#v %v", decision, err)
	}

	facts = readyDeploy()
	facts.Artifact.ManifestCoordinate = "https://example.invalid/artifact"
	if _, err := Evaluate(facts); err == nil {
		t.Fatal("url coordinate accepted")
	}
}

func TestVerifyDoesNotConsumeAdmission(t *testing.T) {
	facts := readyDeploy()
	facts.Operation = OperationVerify
	facts.Admission = Admission{}
	facts.PersonApproved = false
	facts.BackupReady = false
	decision, err := Request(facts)
	if err != nil || !decision.Proceed || decision.ConsumeLaunchAdmission {
		t.Fatalf("verify = %#v %v", decision, err)
	}
	if decision.Evidence == nil || decision.Evidence.Kind != fence.KindVerification {
		t.Fatalf("verify evidence = %#v", decision.Evidence)
	}
}

func TestJanusPrerequisiteFailsClosed(t *testing.T) {
	facts := readyDeploy()
	facts.RequiresJanus = true
	decision, err := Evaluate(facts)
	if err != nil || decision.Blocker != fence.BlockerDependencyPending {
		t.Fatalf("empty = %#v %v", decision, err)
	}

	facts.Dependencies = []Dependency{{Kind: fence.KindAuthorization, Required: false, Succeeded: true}}
	decision, err = Evaluate(facts)
	if err != nil || decision.Blocker != fence.BlockerDependencyPending {
		t.Fatalf("optional = %#v %v", decision, err)
	}

	facts.Dependencies = []Dependency{{Kind: fence.KindAuthorization, Required: true, Succeeded: false}}
	seal, err := fence.Seal([]fence.Check{{Kind: fence.KindAuthorization, Required: true, Succeeded: false}})
	if err != nil {
		t.Fatal(err)
	}
	facts.PrerequisiteSeal = seal
	decision, err = Evaluate(facts)
	if err != nil || decision.Blocker != fence.BlockerDependencyFailed {
		t.Fatalf("failed dependency = %#v %v", decision, err)
	}

	facts.Dependencies[0].Succeeded = true
	facts.PrerequisiteSeal = strings.Repeat("ab", 32)
	decision, err = Evaluate(facts)
	if err != nil || decision.Blocker != fence.BlockerPolicyRefused {
		t.Fatalf("bad seal = %#v %v", decision, err)
	}

	facts = readyDeploy()
	facts.Dependencies = []Dependency{{Kind: fence.KindAuthorization, Required: true, Succeeded: true}}
	decision, err = Evaluate(facts)
	if err != nil || decision.Proceed {
		t.Fatalf("unrequested dependencies = %#v %v", decision, err)
	}
}

func TestApplyCannotPromoteFailure(t *testing.T) {
	facts := readyDeploy()
	ok, err := Evaluate(facts)
	if err != nil || ok.Evidence == nil {
		t.Fatal(err)
	}
	facts.Admission.Consumed = true
	failed := *ok.Evidence
	failed.Outcome = "failed"
	decision, err := Apply(facts, "succeeded", failed)
	if err != nil || decision.Proceed || decision.Outcome != "failed" || decision.AdvancesRelease {
		t.Fatalf("promoted failure = %#v %v", decision, err)
	}

	decision, err = Apply(facts, "succeeded", *ok.Evidence)
	if err != nil || !decision.Proceed || decision.Outcome != "succeeded" || decision.AdvancesRelease {
		t.Fatalf("matching success = %#v %v", decision, err)
	}

	facts.Admission.Consumed = false
	decision, err = Apply(facts, "succeeded", *ok.Evidence)
	if err != nil || decision.Outcome != "failed" || decision.Proceed {
		t.Fatalf("unconsumed admission = %#v %v", decision, err)
	}
}

func readyDeploy() Facts {
	artifact := Artifact{
		VersionScheme:        "inspr-calendar-v2",
		Version:              "260923161128.0.0",
		ReleaseChannel:       "stable",
		ReleaseSequence:      1,
		DigestSHA256:         strings.Repeat("ab", 32),
		CommitDigest:         "abc123",
		ManifestCoordinate:   "pharos/aeon",
		ManifestDigestSHA256: strings.Repeat("cd", 32),
	}
	return Facts{
		Operation:      OperationDeploy,
		Artifact:       artifact,
		Expected:       artifact,
		BackupReady:    true,
		Readiness:      true,
		PersonApproved: true,
		Admission:      Admission{Present: true, BindingMatches: true, AuthorityMatches: true},
		Workflow:       "ship",
		Environment:    "prod",
		ObservedAt:     time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
	}
}
