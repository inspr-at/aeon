// SPDX-License-Identifier: AGPL-3.0-only

// Package inbox is AEON's durable principal inbox (AEON-27, migration 0200).
//
// Coordinator wiring — this package does not edit cmd/aeon:
//
//	srv.Modules = append(srv.Modules, inbox.New(pool))
//	go inbox.NewWorker(pool, inbox.WorkerOptions{}).Run(ctx)
//
// New returns an httpapi.Module for /api/inbox/messages, /api/inbox/stream
// and /api/inbox/targets. NewWorker posts webhook wake hints. Both use the
// server pool. Auth middleware must leave the bearer token on the request:
// agent sends are allowed only when that token's agent_keys row includes
// inbox.send. Person sessions are not scope-gated.
//
// Every inbox read and write runs inside db.InTenant. The worker's tenant
// list is the one exception, because tenants has no tenant_id and no RLS
// (the same registry read as the embedding worker). Each mutation appends
// one tenant event through events.Append in the same transaction. Event
// snapshots and webhook bodies never include the message text. A webhook
// POST is only {"message_id","event_id"} and is never authority; listen and
// SSE replay unacked rows after the sent event id. NOTIFY on aeon_events is
// only a wake hint. Long polls hold at most 30 seconds. Webhook URLs are
// checked again on every delivery: public HTTPS, no loopback, private,
// link-local, metadata, mixed-record or redirect targets.
package inbox
