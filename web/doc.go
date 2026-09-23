// SPDX-License-Identifier: AGPL-3.0-only

// Package web supplies the Vue application through Static. P1.4b (AEON-23)
// adds the R1 workspace at /: a tenant-configured, paginated work tree, filtered
// lists, saved views, keyboard search (Ctrl/Cmd+K), and a Markdown node sidebar.
// Nodes can be created, edited, moved and deleted through the R1 API contract.
//
// Integration uses the existing R0 Static() (fs.FS, bool) entry point and
// httpapi.Server.Web. This frontend package adds no /api routes, so it does not
// introduce an httpapi.Module constructor. The coordinator alone wires the R1
// backend modules into cmd/aeon. Build with npm run build in web, then build
// the binary with -tags webembed to include these assets.
//
// src/lib/api.ts owns typed R1 calls and EventSource lifecycle. SSE named
// resource events and reconnection refresh loaded pages, kinds, saved views,
// active search and the selected node. Native EventSource manages replay IDs.
// Keep its event-name list aligned with backend writers during integration.
// Unsaved drafts survive refreshes; observed concurrent changes block saving.
// R1 has no conditional-write token, so the preflight version check cannot
// prevent a write racing between the GET and PATCH. Markdown raw HTML and
// embedded images are disabled; links use markdown-it's protocol validation.
//
// Validation: npm run typecheck, npm run build, npm run test:unit, and npm test
// (playwright.ui.config.ts). UI tests mock R1 routes and EventSource, covering
// pagination, filters, views, draft safety, rendering, mutation errors, live
// updates and mobile light/dark layouts without another backend worker.
package web
