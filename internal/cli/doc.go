// SPDX-License-Identifier: AGPL-3.0-only

// Package cli is the agent command line for the aeon binary.
//
// RunMessaging is the complete CLI entry point called from cmd/aeon. This package
// does not mount an httpapi.Module and does not register a plugin manifest:
// the verbs below call the APIs the coordinator already mounts (nodes, search,
// models, inbox, auth). When argv[0] is paimos, programName selects
// compatibility mode: the same verbs, ~/.paimos/config.yaml routing,
// PAIMOS_URL / PAIMOS_API_KEY, and the classic text shapes documented by the
// paimos CLI. Run remains available for
// callers that intentionally need the earlier command tree.
//
// Served compatibility verbs (issue, project list, knowledge for memory, runbook, guideline,
// external-system and related-project, search, model resolve, onboard,
// session start, tell, listen, message target, anchors scan/verify, skill
// render, and sync check) talk only to the configured Aeon instance, except
// anchors scan/verify, which read and write the repo-side index
// .paimos/anchors.json and do not open a network connection. skill render
// builds the canonical agent artifact from the project node and its knowledge
// children, passes it through a harness adapter (claude-code, codex, grok, pi,
// cursor), and writes a file whose paimos-managed header lets sync check
// detect drift. run-agent watch executes local Claude work orders and reports
// evidence to Aeon. baseline-batch report-built resolves classic batch aliases
// to stage handoffs and records typed built evidence.
//
// This package exports no httpapi.Module and no plugins.Plugin. The nine
// starter kinds stay as they are. external_system and related_project are
// ensured through POST /api/kinds on first use (kind.created); the
// coordinator already mounts the nodes module that writes the entries.
package cli
