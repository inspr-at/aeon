// SPDX-License-Identifier: AGPL-3.0-only

// Package importer copies the read-only classic Paimos API snapshot to Aeon.
// The importer never changes the source. Mapping, field by field:
//
//   - Project id -> node key PRJ-<id>; name -> title; description -> body;
//     status -> state; created_at/updated_at -> node timestamps. Product owner
//     resolves to fields.product_owner when its principal exists. Tags are in
//     fields.tags (R1 has no tag relation). Every fetched noncomputed project
//     value is also copied verbatim to fields.classic.<name>.
//   - Issue id and issue_key identify the node; type -> kind; title -> title;
//     description or knowledge body -> body; status -> state; timestamps ->
//     node timestamps; project_id/parent_id and parent relation -> parent node.
//     fields.acceptance_criteria and fields.notes retain Markdown for the
//     sidebar. fields.priority, tags, estimate_hours, estimate_lp, budget_hours,
//     total_budget, start_date, end_date, release, sprint_ids, needs_review,
//     archived, and accepted_at hold the corresponding source values. The
//     assignee_id, created_by and accepted_by user IDs resolve to principal
//     UUIDs in fields.assignee, fields.created_by and fields.accepted_by.
//     Unresolved source IDs remain in fields.classic. Every fetched issue
//     value is also copied verbatim to fields.classic.<name>, including other
//     billing, time, Jira, metadata and historical fields. The source URL hash
//     is kept in fields.classic.source_id for safe reruns.
//   - Project knowledge is merged with its issue record, then imported with
//     its original issue key. Orphan sprints become root nodes. Trash issues
//     are fetched, and deleted_at is retained in fields.classic.deleted_at.
//   - Relations become native parent, blocks, relates, duplicates and cites
//     links where possible. related, follows_from, impacts, applies_to_memory
//     and groups become relates. depends_on becomes blocks with the dependency
//     blocking the dependent. blocks keeps source as the blocker. A release
//     row is release membership: the classic container is the release and the
//     other end is the member issue (either column order is accepted when the
//     node kinds show which end is the release). Membership registers that
//     release on journey_releases for its project and, when the member is a
//     ticket, sets journey_tickets.release_node_id. It does not choose
//     journey_projects.current_release_node_id and does not invent a released
//     state. The classic type stays on the import.relation payload as
//     record.type and classic_type. Every relation, including unsupported
//     types and out-of-scope targets, is retained in an import.relation event.
//     Comments, history and attachment metadata are retained verbatim in
//     import events.
//   - Users become classic identities and tenant principals. User id,
//     username, email, role and created_at are used; user preferences stay
//     skipped. Passwords, keys, sessions and TOTP secrets are never requested.
//
// Intentionally skipped: project active_issue_count, done_issue_count,
// issue_count, open_issue_count, effective_rate_hourly, effective_rate_lp,
// last_activity, node_depth and rate_inherited, all derived/computed values;
// user fields status, nickname, first_name, last_name, avatar_path,
// markdown_default, monospace_fields, recent_projects_limit,
// internal_rate_hourly, show_alt_unit_table, show_alt_unit_detail, locale,
// recent_timers_limit, timezone, preview_hover_delay,
// issue_auto_refresh_enabled, issue_auto_refresh_interval_seconds,
// search_scope_shortcut, command_palette_shortcut,
// intake_confidence_threshold, last_login_at, totp_enabled,
// accruals_stats_enabled, accruals_extra_statuses and is_super_admin;
// attachment file bytes (only metadata is fetched); and nonproject user and
// instance memory, which the project knowledge endpoint does not enumerate.
// These skips are not reported as unmapped_fields. All fetched work fields
// survive either in node fields or in import event payloads. Dry runs perform
// the same source reads and return an empty unmapped_fields array. Source GETs
// are capped at four concurrently by default; --concurrency and --delay can
// lower pressure on classic PPM. A project whose issues or knowledge endpoint
// returns 404 is skipped. An issue whose detail endpoint returns 404 is also
// skipped. The report lists each skipped item with its type, source ID, request
// path and HTTP status; counts includes skipped, skipped_projects and
// skipped_issues. Other source HTTP errors abort the import.
//
// Reruns update nodes only when imported content changes and deduplicate
// auxiliary events. Native writes are serialized per tenant and source with a
// transaction advisory lock and use events.Append inside db.InTenant.
//
// BackfillRelations replays import.relation events already stored for one
// tenant and applies the mapping above. It is the function the coordinator
// calls from cmd/aeon; this package does not register a command.
//
//	aeon import backfill-relations --tenant SLUG
//
// Resolve SLUG to tenants.id, open the pool with db.Open, then:
//
//	report, err := importer.BackfillRelations(ctx, pool, tenantID)
//	json.NewEncoder(stdout).Encode(report)
//
// report.Writes is the number of projection rows inserted or updated.
// A second successful call returns Writes == 0 and appends no events.
package importer
