// SPDX-License-Identifier: AGPL-3.0-only

// Package relations implements typed tenant-local links between live nodes.
// Coordinator wiring: add relations.New(pool) and
// events.New(pool, relations.UndoOption()) to httpapi.Server.Modules.
// Constructors return httpapi.Module; auth must supply tenant.PrincipalFrom.
// Every create/delete and undo writes one full relation snapshot event in the
// same tenant transaction. Relation events use the canonical source as node_id.
// "relates" sorts its endpoints by UUID; other types retain their direction.
// customer_of and contact_for are R4 CRM links. customer_of runs from a live
// organisation to a live project or quote. contact_for runs from a live
// contact to a live organisation, project, or quote. A reversed or wrong-kind
// pair is rejected and writes no event. Undo of those links rechecks the same
// rule and conflicts when either node is gone or the kinds no longer match.
// Lists include either endpoint and paginate by ID, with cursors bound to the
// tenant, node filter and ordering. Undo is actor-or-admin, rejects changed
// resources, and recreates a deleted link only when both nodes remain live.
package relations
