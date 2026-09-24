// SPDX-License-Identifier: AGPL-3.0-only

// Package views implements tenant-scoped saved list views. New returns an
// httpapi.Module for mounting the /api/views routes. View mutations append an
// event in the same tenant transaction through EventWriter; until the shared
// internal/events writer is available, the default writer inserts the event
// row directly.
//
// It also serves GET and PUT /api/preferences/{key}: small per-person JSON
// objects for UI state such as list columns, widths and the panel split
// (migration 0541). They are private to the calling principal and append no event.
package views
