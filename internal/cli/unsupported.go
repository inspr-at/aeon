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
	reasonAgentd = "paimos-agentd serve is a separate operator binary (cmd/aeon-agentd), not an aeon subcommand"

	reasonHarnessAddress = "harness:agent addresses are classic encrypted targets; pass the recipient principal UUID to tell"

	reasonListenAs = "listen --as harness:agent selects a classic receiver target; Aeon listen reads the authenticated principal"

	reasonListenFollow = "listen --follow and --deliver stream through a classic adapter; Aeon listen returns the current inbox page"

	reasonClassicTarget = "message target set with --address, --adapter or a classic kind stores an encrypted receiver Aeon does not keep; use --principal and --kind pull"

	reasonDeliveries = "message deliveries is the classic redacted delivery log; Aeon inbox targets have no delivery ledger"

	reasonKnowledgeKind = "Aeon knowledge types are memory, runbook, guideline, external-system, and related-project"

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
	{"paimos-agentd serve", nil, reasonAgentd},
	{"listen --follow", []string{"listen", "--project", "AEON", "--follow"}, reasonListenFollow},
}
