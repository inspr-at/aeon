// SPDX-License-Identifier: AGPL-3.0-only

// Package agentaccounts serves /api/agent-accounts and the allowance ledger
// behind usage-aware routing.
//
// New returns an httpapi.Module. The coordinator mounts it; this package does
// not edit cmd/aeon. Registration stores an opaque account key for the
// calling agent principal and rejects credential-shaped values. Probes are
// accepted only from that principal and only for the account's daemon id.
// A route request names the claiming daemon and its locally enrolled account
// IDs. Selection is restricted to those IDs, that daemon, and accounts
// registered by the calling agent. A reservation requires a successful probe
// newer than ProbeFreshness. The coordinator mounts New as an httpapi.Module.
//
// Pace for elapsed window fraction f in [0,1] is f (steady),
// 1-(1-f)^2 (frontload) or 1 (unrestricted). The cumulative allowed fraction
// is min(1, pace+burst_ratio), and the estimate must also fit the hard
// allowance. Eligible accounts are ordered by projected
// (used+reserved+estimate)/allowance, then by account id. Reservations for
// every active window commit with run.account_id or not at all. A repeated
// route returns that same choice.
//
// Settle and Release run inside the caller's db.InTenant transaction. Settle
// turns monotonic telemetry sums into used units and is idempotent per
// reservation. Release returns unused reserved units only for a queued run or
// a terminal fenced transition, and it does not touch a live process. Draining
// blocks new reservations and leaves an owned run where it is. Account and
// window responses expose provisional=true when a window has no positive
// measurement for its unit or a settled reservation lacked one. Historical
// zero telemetry cannot prove measured zero, so it remains provisional.
package agentaccounts
