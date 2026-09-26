// SPDX-License-Identifier: AGPL-3.0-only

// Package agents defines AEON R2's contract for durable agent work and local
// harness execution. AEON is the primary interface for people and agents.
// The contract is api/openapi.yaml and migrations 0200-0203. Builders own
// implementation packages below; this package is the handoff and owns no
// shared server wiring.
//
// Runtime port plan from classic paimos/backend (read-only source):
//   - agentd supervisor, workspace provenance, control replay, readiness and
//     bounded child lifecycle remain operator-local. Preserve one daemon
//     generation per owner, physical process ownership, fenced controls and
//     fail-closed loss of ownership. Replace project int64, identity strings,
//     ticket int64 and harness session IDs in server reports with tenant UUID,
//     agent principal UUID, work-order node UUID and run UUID. Keep vendor
//     session IDs only in daemon-private state, linked to the AEON run.
//   - localjournal and ownedprocess move with their atomic write, replay,
//     process-group and ownership tests. Journal format needs an AEON namespace,
//     tenant/principal/run binding and migration marker; old journals are read
//     only for explicit import, never silently resumed. Durable inbox, run,
//     approval and allowance state lives in Postgres, not a second local log.
//   - agentdwire and cmd/aeon-agentd transport, bounded request decoding,
//     authenticated local socket and generation fences are ported to an AEON
//     agentd binary or subcommand. A daemon authenticates to AEON over HTTPS
//     with a scoped agent key; it pulls work and inbox items, reports content-
//     free telemetry and receipts, and never sends credentials or raw vendor
//     protocol payloads. AEON may send a webhook wake hint; the daemon then
//     fetches authenticated state. A heartbeat is not proof of process
//     ownership. After reconnect, reconcile run ID plus generation before any
//     control. The coordinator chooses executable packaging and mounts APIs.
//   - Codex, Claude, Pi, Cursor and Grok adapter state machines and protocol
//     tests move behind the local daemon Adapter interface. Codex app-server
//     thread/turn control, Claude native bridge, Pi RPC and held queue, Cursor
//     ACP decisions, and Grok conversation proxy stay vendor-specific. Classic
//     agentmessage/harness plugins become fallback delivery shims only during
//     import; direct tell targets no longer authorize writes. Adapter identity
//     probes and account homes remain local. AEON sees an opaque account key,
//     family, effective-model evidence and bounded usage, not token material.
//
// Durable inbox: every message names sender and recipient principals in one
// tenant, an idempotency key, optional reply_to, and one sent event ID. Send
// writes inbox.sent metadata and the message in one db.InTenant transaction;
// never put the body in tenant-wide events or webhook payloads. A sender may
// reply only to a message it sent or received. Listen and SSE filter on the
// authenticated recipient, replay by sent event ID and deliver at least once.
// ACK is recipient-only and idempotent; only ACK removes a live message from
// the pending view. Long poll holds at most 30 seconds and reruns the tenant
// query after wake. NOTIFY and HTTPS webhooks are hints; retries use inbox_wakes.
// Reject loopback, private, link-local, metadata, rebinding and redirect URLs
// for public webhooks. A webhook contains only IDs and is never authority.
// This replaces classic paimos tell/listen/message targets, with import of
// historical messages explicitly outside the live delivery path.
//
// Work orders use the seeded work_order node kind. The node supplies title,
// body, key and tree position; work_orders adds assignment, state and hard
// cost/time ceilings. Criteria are individually checkable and evidence links
// to a criterion and optionally a run. A done order must have all criteria
// checked and evidence attached. Run creation and each usage update lock the
// order, check aggregate run cost and elapsed time against its ceilings, and
// block new dispatch on exhaustion. Agent runs use monotonic telemetry sequence
// numbers: exact replay is idempotent, divergent replay is a conflict. The
// daemon's fenced generation owns running-state reports. Store requested
// model separately from vendor-reported effective model. No vendor text or
// secret enters telemetry. Every state mutation appends a tenant event.
//
// Approvals are approval.proposed, approval.approved and approval.denied events
// in R1's append-only log. Projection rows make pending requests and matching
// permission grants cheap to query. An authenticated agent proposes only its
// own scoped permission; a person session decides before expiry. The DB
// rejects agent decisions and grants without an approved matching request.
// The handler verifies the resource belongs to the same tenant, writes the
// event and decision/grant in one transaction, and checks expiry and revocation
// at each sensitive action. API key scopes are an outer ceiling: an approval
// cannot enlarge a key's base scope. No daemon, CLI or model verdict substitutes
// for the person decision. Revoke appends a separate event and closes the grant.
//
// Model policy ports classic dispatchprofile catalog, role ladder, profile
// validation and trusted command rendering, with tenant-scoped versioned
// profiles and ordered routes. Seed each tenant's empty registry from the
// classic static catalog at first setup; pin each immutable profile version
// and retain past pins for run history. Role names remain scout, mechanical, build,
// build-hard and review-gate so existing INSPR doctrine can switch from
// `paimos model resolve` to `aeon model resolve`. For review-gate require
// author_family, exclude that family, preserve explicit fallback order and
// owner_required if none qualifies. Resolve exposes skip reasons and a trusted
// command template; it never executes a command. A selected profile is pinned
// to each run. Expiring route suppressions do not mutate historical profiles.
//
// Agent-account juggling (AEON-14): tenant account rows advertise opaque
// daemon-local enrollments only. No vendor token, auth home, cookie or API key
// is accepted in the AEON account API or database. Each allowance window has
// one unit, bound, pacing model and used/reserved counters. Route a queued run
// only to a matching account with a recent successful owner-daemon probe;
// registration records the daemon agent principal, and probe must use its key
// and match daemon_id. Claim binds daemon_id, generation and reservation to
// the run; the caller is its assigned agent or holds an approved run.claim
// grant for that run. Lock active windows and choose by
// remaining allowance, recent usage, pace target and parallel occupancy.
// For elapsed window fraction f in [0,1], steady pace is f, frontload pace
// is 1-(1-f)^2, and unrestricted pace is 1. The cumulative allowed fraction
// is min(1, pace+burst_ratio); eligibility must also fit the hard allowance.
// Rank eligible accounts by projected (used+reserved+estimate)/allowance,
// then by account UUID for a stable tie break, after filtering stale probes
// and parallel occupancy. Reserve estimates atomically before dispatch and
// set run.account_id in that same transaction; retries replay that choice.
// Settle actual usage from monotonic run telemetry, release stale reservations
// under a fenced run
// transition, and make retries idempotent by run/window. A drain stops new
// reservations but does not move an owned live process. Unknown allowance or
// stale probe fails closed. Window overlap for the same account and unit is
// rejected in the handler; multi-window reservations are all-or-nothing.
//
// Worker file ownership (AEON-25 handoff; builders do not edit shared files):
//   - inbox: internal/inbox/*.go; /inbox/*, webhook wake worker;
//     consumes migration 0200.
//   - work: internal/workorders/*.go and internal/agentruns/*.go;
//     /work-orders/* and /runs/*; consumes migration 0201.
//   - approvals: internal/approvals/*.go; /approvals/*;
//     consumes migration 0202.
//   - registry/accounts: internal/modelregistry/*.go and
//     internal/agentaccounts/*.go; /models/* and /agent-accounts/*,
//     consumes migration 0203.
//   - local runtime: internal/agentd/*.go, internal/agentdwire/*.go,
//     internal/localjournal/*.go, internal/ownedprocess/*.go and
//     cmd/aeon-agentd/*.go; no server migration or shared API edits.
//
// Contract worker owns api/openapi.yaml, migrations 0200-0203 and this doc.go.
// The coordinator alone reconciles contract changes, wires httpapi.Module into
// cmd/aeon and chooses binary packaging. Builders consume tenant.PrincipalFrom,
// db.InTenant, the R1 event writer and httpapi.Module. They do not change R0/R1
// packages or the shared OpenAPI file independently. Each module tests its own
// authorization, tenant isolation, idempotency, replay and budget edge cases.
package agents
