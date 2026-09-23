// SPDX-License-Identifier: AGPL-3.0-only

// Package relations implements typed tenant-local links between live nodes.
// Coordinator wiring: add relations.New(pool) and
// events.New(pool, relations.UndoOption()) to httpapi.Server.Modules.
// Constructors return httpapi.Module; auth must supply tenant.PrincipalFrom.
// Every create/delete and undo writes one full relation snapshot event in the
// same tenant transaction. Relation events use the canonical source as node_id.
// "relates" sorts its endpoints by UUID; other types retain their direction.
// Lists include either endpoint and paginate by ID, with cursors bound to the
// tenant, node filter and ordering. Undo is actor-or-admin, rejects changed
// resources, and recreates a deleted link only when both nodes remain live.
package relations
