// SPDX-License-Identifier: AGPL-3.0-only

package cli

// Doctrine command inventory, read from these trees and not modified:
//
//	/Users/markus/Code/paimos/doctrine/docs
//	/Users/markus/Code/paimos/AGENTS.md
//
// Served verbs are implemented against Aeon. The rows below are the
// invocations that still have no Aeon resource. Each exits 3 with Reason
// and does not open a network connection.

const (
	reasonHarness = "Aeon has no harness-session control plane; work orders and agent runs do not register, bind, heartbeat, drain or stop a vendor session"

	reasonAnchors = "anchor scan and verify read a repo index file; Aeon has no anchor resource"

	reasonSkill = "skill render writes a harness artifact; Aeon has no skill-render resource"

	reasonRunAgent = "run-agent watch is an operator-local vendor process; Aeon work orders do not spawn one"

	reasonBaseline = "baseline-batch report-built is replaced by stage handoffs and is not a compat verb"

	reasonSync = "sync check diffs a local knowledge cache; Aeon has no sync verb"

	reasonAgentd = "paimos-agentd serve is a separate operator binary (cmd/aeon-agentd), not an aeon subcommand"

	reasonHarnessAddress = "harness:agent addresses are classic encrypted targets; pass the recipient principal UUID to tell"

	reasonListenAs = "listen --as harness:agent selects a classic receiver target; Aeon listen reads the authenticated principal"

	reasonListenFollow = "listen --follow and --deliver stream through a classic adapter; Aeon listen returns the current inbox page"

	reasonClassicTarget = "message target set with --address, --adapter or a classic kind stores an encrypted receiver Aeon does not keep; use --principal and --kind pull"

	reasonDeliveries = "message deliveries is the classic redacted delivery log; Aeon inbox targets have no delivery ledger"

	reasonBundle = "session start --bundle full and --format files need the classic knowledge bundle cache; Aeon session start emits the agent and session id only"

	reasonKnowledgeKind = "Aeon seeds memory, runbook and guideline nodes; external-system and related-project are not node kinds"

	reasonExpectsReply = "tell --expects-reply and --action-request open a classic obligation; Aeon inbox send has no held reply"
)

// unsupportedCompat is the table of doctrine invocations Aeon does not serve.
// Args is what follows the program name. An empty Args row is a separate
// binary and is not executed.
var unsupportedCompat = []struct {
	Name   string
	Args   []string
	Reason string
}{
	{"anchors scan", []string{"anchors", "scan"}, reasonAnchors},
	{"anchors verify", []string{"anchors", "verify"}, reasonAnchors},
	{"skill render", []string{"skill", "render", "ops"}, reasonSkill},
	{"run-agent watch", []string{"run-agent", "watch"}, reasonRunAgent},
	{"baseline-batch report-built", []string{"baseline-batch", "report-built"}, reasonBaseline},
	{"sync check", []string{"sync", "check"}, reasonSync},
	{"paimos-agentd serve", nil, reasonAgentd},
	{"tell harness:agent", []string{"tell", "codex:worker", "--project", "AEON", "--level", "simple", "-m", "hi"}, reasonHarnessAddress},
	{"tell --expects-reply", []string{"tell", "00000000-0000-4000-8000-000000000001", "--project", "AEON", "--expects-reply", "-m", "hi"}, reasonExpectsReply},
	{"listen --as harness:agent", []string{"listen", "--as", "codex:worker", "--project", "AEON"}, reasonListenAs},
	{"listen --follow", []string{"listen", "--project", "AEON", "--follow"}, reasonListenFollow},
	{"message target set classic", []string{"message", "target", "set", "--project", "AEON", "--address", "codex:worker", "--adapter", "codex", "--kind", "codex_thread"}, reasonClassicTarget},
	{"message deliveries", []string{"message", "deliveries"}, reasonDeliveries},
	{"session start --bundle full", []string{"session", "start", "--project", "AEON", "--agent", "worker", "--bundle", "full"}, reasonBundle},
	{"knowledge external-system", []string{"knowledge", "create", "--type", "external-system", "--slug", "x", "--project", "AEON", "--title", "X"}, reasonKnowledgeKind},
	{"knowledge related-project", []string{"knowledge", "get", "related-project", "x", "--project", "AEON"}, reasonKnowledgeKind},
	{"harness register", []string{"harness", "register"}, reasonHarness},
	{"harness list", []string{"harness", "list"}, reasonHarness},
	{"harness status", []string{"harness", "status"}, reasonHarness},
	{"harness orchestrator", []string{"harness", "orchestrator"}, reasonHarness},
	{"harness bind", []string{"harness", "bind"}, reasonHarness},
	{"harness heartbeat", []string{"harness", "heartbeat"}, reasonHarness},
	{"harness yield", []string{"harness", "yield"}, reasonHarness},
	{"harness drain", []string{"harness", "drain"}, reasonHarness},
	{"harness complete-delivery", []string{"harness", "complete-delivery"}, reasonHarness},
	{"harness interrupt", []string{"harness", "interrupt"}, reasonHarness},
	{"harness stop", []string{"harness", "stop"}, reasonHarness},
	{"harness complete-control", []string{"harness", "complete-control"}, reasonHarness},
	{"harness mark-stopped", []string{"harness", "mark-stopped"}, reasonHarness},
}
