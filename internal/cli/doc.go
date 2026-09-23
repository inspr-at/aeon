// SPDX-License-Identifier: AGPL-3.0-only

// Package cli is the agent command line for the aeon binary.
//
// Run is the constructor the coordinator calls from cmd/aeon. This package
// does not mount an httpapi.Module and does not register a plugin manifest:
// the verbs below call the APIs the coordinator already mounts (nodes, search,
// models, inbox, auth). When argv[0] is paimos, programName selects
// compatibility mode: the same verbs, PAIMOS_URL / PAIMOS_API_KEY, and the
// classic text shapes documented by the paimos CLI.
//
// Served compatibility verbs (issue, knowledge for the seeded kinds, search,
// model resolve, onboard, session start, tell, listen, message target) talk
// only to the configured Aeon instance. Commands the doctrine still names
// that have no Aeon resource are listed in unsupportedCompat and exit 3
// before any network call.
package cli
