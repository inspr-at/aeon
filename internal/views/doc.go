// SPDX-License-Identifier: AGPL-3.0-only

// Package views implements tenant-scoped saved list views. New returns an
// httpapi.Module for mounting the /api/views routes. View mutations append an
// event in the same tenant transaction through EventWriter; until the shared
// internal/events writer is available, the default writer inserts the event
// row directly.
//
// A view may belong to one project (project_id, migration 0760) and then shows
// in that project's view bar; GET /api/views?project_id= lists the caller's own
// and the shared views of that project in the order they were made. A view keeps
// the whole list shape: filters (an opaque object the web app owns), sort_keys
// in the node list's spelling ("state,-updated_at" as ["state","-updated_at"]),
// group_by and columns. Delete is soft: POST /api/views/{id}/restore brings the
// view back with its id. Only the owner changes a view; shared views are
// read-only for everyone else.
//
// UndoHandlers makes view.created, view.updated, view.deleted and view.restored
// reversible through POST /api/events/{id}/undo. The coordinator registers them
// with events.WithUndoHandlers(views.UndoHandlers()) in cmd/aeon; the web app
// itself undoes a delete through the restore endpoint, so it works either way.
//
// It also serves GET and PUT /api/preferences/{key}: small per-person JSON
// objects for UI state such as list columns, widths and the panel split
// (migration 0541). They are private to the calling principal and append no event.
package views
