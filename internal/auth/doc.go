// SPDX-License-Identifier: AGPL-3.0-only

// Package auth authenticates people and agent keys. The middleware applies an
// outer, deny-by-default key-scope ceiling before any module handler runs.
// Module handlers still check principal kind, resource ownership and live grants.
//
// Agent route-to-scope table (GET and HEAD are reads unless noted):
//
//	/api/projects, /api/nodes, /api/node-keys, /api/kinds:
//	  nodes.read; node writes use nodes.write; kind and tag writes use
//	  nodes.configure. Kind mutations and tag rename/delete require a person
//	  admin; other tag metadata edits retain the member route.
//	/api/nodes/{id}/activity|comments|attachments, /api/attachments:
//	  nodes.read or nodes.write.
//	/api/relations: relations.read or relations.write.
//	/api/events: events.read or events.undo.
//	/api/search: search.read.
//	/api/views, /api/preferences, /api/project-groups:
//	  views.read or views.write.
//	/api/knowledge: knowledge.read or knowledge.write.
//	/api/projects/{id}/journey|requirements|releases:
//	  journey.read for reads; agent writes have no mapping.
//	/api/approvals: approvals.read for list, approvals.request for proposal;
//	  decisions and revocations have no agent mapping.
//	/api/inbox, /api/projects/{id}/messages|message-targets|message-deliveries:
//	  inbox.read or inbox.send.
//	/api/models, /api/plugins: models.read or plugins.read; writes have no
//	  agent mapping.
//	/api/work-orders: work_orders.read or work_orders.write; run creation
//	  uses run.create. /api/runs: run.read, run.claim or run.telemetry.
//	/api/harness-sessions, /api/projects/{id}/harness-sessions:
//	  harness.read, harness.write, harness.worker or harness.control.
//	/api/projects/{id}/intake: intake.read or intake.write.
//	/api/stage-handoffs, /api/projects/{id}/baseline-batches:
//	  stage.<op>; the handler rechecks the exact operation and grant.
//	/api/agent-accounts, GET /api/me: account.manage.
//	/api/time-entries, /api/time-periods, /api/nodes/{id}/time-totals:
//	  hours.read or hours.write.
//
// Every other API path returns 403 to an authenticated agent key, including
// business/customer/admin and public-capability paths when a key is presented.
// Empty scope lists grant nothing. Anonymous public calls retain their normal
// behavior. Person sessions are governed by role and module checks.
package auth
