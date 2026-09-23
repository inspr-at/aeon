// SPDX-License-Identifier: AGPL-3.0-only

// Package hours implements AEON-47 / P4.4. The coordinator registers Plugin()
// together with business_costs before sealing the shared plugins.Registry, then
// mounts New(pool, registry) behind auth.Middleware. Mount HoursView.vue at
// /business/hours; this package does not wire the router or server.
//
// Routes: GET/POST /api/time-periods, GET /api/time-periods/{periodId},
// POST /api/time-periods/{periodId}/approve, GET/POST /api/time-entries and
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
// Migration 0403 requires exact whole-second durations while retaining the
// run's persisted timestamps. Terminal runs with fractional-second duration
// are explicitly ineligible (409); no rounding or fabricated interval is used.
// Supporting them needs coordinator-owned contract/migration reconciliation.
package hours
