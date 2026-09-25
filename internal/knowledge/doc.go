// SPDX-License-Identifier: AGPL-3.0-only

// Package knowledge is the human surface of the knowledge plane: runbooks,
// guidelines, memory, external systems and related projects that INSPR
// agents read before they work (AEON-126).
//
// Entries stay ordinary nodes, so imports, the paimos CLI verbs
// (knowledge list/get/create/update through /api/nodes), search and the
// event log keep working unchanged. Node kind slugs are snake_case
// (external_system); the wire type is the classic kebab-case URL segment
// (external-system). fields.slug names an entry for agents and fields.metadata
// keeps type-specific values (guideline rule, external system url, ...).
// Every other field is preserved on write.
//
// What this package adds over /api/nodes:
//   - GET /api/knowledge lists and searches entries across projects or in one
//     (project_id), with the nearest project, a plain-text excerpt around the
//     match, the last writer, link counts and type/status counts.
//   - GET /api/knowledge/{id} and GET /api/knowledge/resolve?project_id&type&slug
//     return one entry with its body, author and linked nodes. resolve follows
//     slug renames recorded in knowledge.updated and node.updated events and
//     reports renamed_from, so an old link or agent reference still lands.
//   - GET /api/knowledge/graph?project_id&types&status&include=tickets derives
//     a body-free graph (AEON-146): live knowledge, directed relations and
//     resolved wiki/Markdown/code-slug/key mentions. Same-kind slugs, including
//     rename history, take precedence over other kinds in the project. Typed
//     Markdown links stay in their named project/type. Direct ticket satellites
//     are optional and their bodies are never read. Directed pairs are unique,
//     relations win over mentions, and degree counts unique returned neighbours.
//     The response flags truncation at 2,000 nodes or 8,000 edges. This read-only
//     projection needs no migration, event write or separate plugin manifest;
//     knowledge.New already mounts it, without additional coordinator wiring.
//   - POST, PATCH and DELETE /api/knowledge[/{id}] enforce the slug rules of
//     classic Paimos (^[a-z][a-z0-9_-]*$, at most 64 characters, unique per
//     project and type among live entries; memory reserves references, stale,
//     proposed and needs-review) under a per-project advisory lock. PATCH and
//     DELETE honour If-Unmodified-Since (exact updated_at); a stale value
//     returns 412 with the current entry and writes nothing.
//   - Status is the classic trio: active (stored as backlog), proposed and
//     archived (stored as cancelled). Any other stored state reads as active and
//     is kept until the status class changes.
//   - Principals with a viewer or read-only role cannot write (403).
//
// Every write runs in db.InTenant and appends one event in the same
// transaction: knowledge.created, knowledge.updated or knowledge.deleted, each
// with complete node snapshots in the nodes package's JSON shape. Missing
// external_system or related_project kinds are created on first use with a
// kind.created event, like the CLI does. UndoHandlers makes all three
// reversible through POST /api/events/{id}/undo: undo re-checks that the entry
// is exactly as the event left it and that the restored slug is still free,
// otherwise it answers 409.
//
// Coordinator wiring (this package does not edit cmd/aeon):
//
//	knowledge.New(pool)                                   // add to Server.Modules
//	events.WithUndoHandlers(knowledge.UndoHandlers())     // add to events.New(...)
//
// Migration 0750 adds two partial indexes: live nodes by (parent, kind, slug)
// and knowledge/node update events by the slug they renamed away from.
package knowledge
