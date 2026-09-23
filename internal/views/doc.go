// SPDX-License-Identifier: AGPL-3.0-only

// Package views implements tenant-scoped saved list views. New returns an
// httpapi.Module for mounting the /api/views routes. View mutations append an
// event in the same tenant transaction through EventWriter; until the shared
// internal/events writer is available, the default writer inserts the event
// row directly.
package views
