// SPDX-License-Identifier: AGPL-3.0-only

// Package agentruns implements AEON-28 / P2.3 queued runs, fenced claims and
// content-free telemetry. New(pool, recorder) returns an httpapi.Module for
// /api/runs/* and /api/work-orders/{workOrderId}/runs. Mount alongside
// workorders.New(pool) behind auth.Middleware; only the coordinator edits cmd.
//
// Agent keys require exact run.read, run.create, run.claim or run.telemetry
// scopes. People may create/read runs; queue, claim and telemetry are agent-only.
// Agents create their own runs, read their own runs, and see only their queue.
// Claim requires the assigned agent or a live, unrevoked run.claim grant for
// this run. The caller must also own the reserved daemon account. A new claim
// verifies all reservation IDs, active windows, compatible profile, recent
// successful probe (two minutes), account availability and remaining order
// budget. Same-generation claims replay without mutation; another generation
// cannot take over a live process. Profile and requested model are pinned when
// queued; effective model is separately retained with vendor evidence.
//
// Coordinator contract reconciliation: R2 RunTelemetry lacks fencing fields.
// This module therefore requires X-Aeon-Daemon-ID and X-Aeon-Daemon-Generation
// on every telemetry request, matching the claim and current account generation.
// The coordinator must add these headers to OpenAPI and the daemon client; this
// worker does not edit the shared contract or another worker's package. Model
// and daemon identifiers accept at most 128 ASCII letters/digits or . _ - : /.
// Free-form fields and unknown JSON fields are rejected. No vendor transcript,
// credential, raw error text or request body is logged.
//
// Sequences must strictly increase but may have gaps. The numeric telemetry
// row, usage totals, status, budget block and R1 event commit atomically. Since
// migration 0201 has no columns for status/model replay, run.telemetry events
// retain the complete canonical content-free report. Exact replay returns the
// current run without charging or appending again; changes to any field conflict,
// including status/model fields. Terminal runs accept only historical replay.
// started defaults to running; finished defaults to completed. Claim starts
// server elapsed-time accounting, including starting/waiting time.
//
// Account integration: pass the account module's UsageRecorder to New for
// production allowance settlement. The callback receives updated cumulative
// totals and the accepted report in the same db.InTenant transaction, once per
// new sequence. It must append events for its own changed projections and must
// not nest transactions. It can release reservations on terminal transitions.
// Nil performs no allowance settlement: the account worker owns its projections,
// while work-order budget accounting is always enforced here. Lock order is
// work_order, run, account, then windows/reservations ordered by window UUID.
package agentruns
