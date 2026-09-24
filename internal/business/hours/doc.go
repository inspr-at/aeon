// SPDX-License-Identifier: AGPL-3.0-only

// Package hours implements AEON-47 / P4.4. The coordinator registers Plugin()
// together with business_costs before sealing the shared plugins.Registry, then
// mounts New(pool, registry) behind auth.Middleware. Mount HoursView.vue at
// /business/hours; this package does not wire the router or server.
//
// Routes: GET/POST /api/time-periods, GET /api/time-periods/{periodId},
// POST /api/time-periods/{periodId}/approve, GET/POST /api/time-entries, PATCH/DELETE /api/time-entries/{id}, and
// GET /api/nodes/{nodeId}/time-totals. Agent keys require hours.read or
// hours.write and can access only their own periods/entries/totals. People
// require member/admin roles; an admin person may record another person's time
// or convert another agent's terminal run. Only admin people may approve.
//
// GET period emits X-Entries-SHA256: <entries_sha256> for the exact ordered entry set,
// including an empty set. Send that digest and the returned
// revision to approve. Entries increment the period revision; approval retains
// that sealed revision. Identical approval and run-conversion retries are
// no-ops; divergent retries conflict. Manual creation has no idempotency key
// in the R4 contract and must not be automatically retried.
//
// Amounts are bill-rate snapshots, computed and summed with Postgres numeric
// and encoded as JSON numbers without float conversion. Digest v1 is SHA-256
// over encoding/json's compact Entry array sorted by UUID, with UTC timestamps
// and database numeric text (four decimals). Clients should use the X-Entries-SHA256 rather
// than implement this encoding. Period reads lock against insertions so their
// revision and digest represent one snapshot.
//
// B9 / AEON-75 adds person-only entry corrections: the entry's principal or
// a current admin may correct/delete an open-period entry. Approved periods
// reject corrections and undo. A start-only edit retains duration; duration
// and end are mutually exclusive. Agent-run node/timing remain source facts.
// Changing cost unit or the UTC start date selects an effective rate; other
// edits preserve the original snapshot. All monetary arithmetic is SQL numeric.
// PATCH and DELETE accept optional If-Match (quoted updated_at, strong tags,
// lists or *) or If-Unmodified-Since (exact updated_at, RFC3339/HTTP date).
// If-Match takes precedence; stale values return 412 with current and ETag.
// No-op PATCH emits no event. Entry mutations and undo advance the period
// revision; reads recompute totals and the digest, preserving digest v1 by
// excluding updated_at. Full snapshots are in time_entry.updated/deleted.
// Coordinator: add events.WithUndoHandlers(hours.UndoHandlers()) to the
// event module (or supply the shared registry to UndoHandlers). Undo checks
// current actor/owner/admin authority, installations, period state and stale
// snapshots, then returns one compensating change for the event host to append.
// This package does not wire cmd/aeon, builtin plugins or the web router.
//
// Migration 0403 requires exact whole-second durations while retaining the
// run's persisted timestamps. Terminal runs with fractional-second duration
// are explicitly ineligible (409); no rounding or fabricated interval is used.
// Supporting them needs coordinator-owned contract/migration reconciliation.
package hours
