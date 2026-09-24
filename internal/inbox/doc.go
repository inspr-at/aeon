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
// Project messaging wiring: mount NewMessaging(pool, encryptionKey) alongside
// New and register MessagingPlugin in the coordinator's compiled registry.
// Existing migrations suffice for B7; api/openapi.yaml defines the additions.
// GET /projects/{projectId}/messages is person admin/super_admin inspection:
// limit 1..200, newest_first, address (or to), thread and pending filters.
// The reply_to chain selects the whole thread rooted at any supplied member;
// all recursive steps remain project- and tenant-scoped. Ascending sent-event
// ordering stays the default. In descending mode after is an exclusive upper
// bound (zero starts at newest). Every message now includes created_at and a
// nullable human_resolution_outcome. Recipient listen retains its ten-row cap.
//
// POST /projects/{projectId}/messages/{messageId}/resolution accepts decision
// resolved|dismissed and optional note. Only authenticated person admins or
// super_admins may call it; bearer authorization and agent attribution are
// rejected. inbox.action_resolved is the immutable resolution projection,
// with actor attribution and a digest of the note (no note text in events).
// Same decision/note replays the first response; different material conflicts.
// The transaction uses messaging's tenant advisory lock before message locks
// and event insertion. pending=true omits any resolved/dismissed held item;
// full inspection retains it, still held. Resolution never releases a message,
// sends a delivery, acknowledges it, or closes an explicit reply obligation.
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
