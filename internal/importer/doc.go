// SPDX-License-Identifier: AGPL-3.0-only

// Package importer moves the readable classic Paimos API snapshot into Aeon's
// R1 node model. The source is never mutated. Mapping, field by field:
//
//   - projects.id -> nodes.key PRJ-<id> and fields.classic.record.id;
//     projects.key -> fields.classic.record.key (the project prefix is retained
//     there and each issue's full issue_key is copied verbatim to nodes.key);
//     name -> title; description -> body; status -> state; created_at and
//     updated_at -> node timestamps. node_depth, product_owner, customer_label,
//     customer_id/name, logo_path, tags, rate_hourly/lp, ai_defaults/policy,
//     and derived activity/count/effective-rate values -> fields.classic.record.
//     product_owner also resolves to fields.classic.principals.product_owner.
//     A project is a root node of kind project. Active, frozen, archived and
//     deleted projects are read separately because status=all excludes trash.
//   - issues.id -> fields.classic.record.id; issue_key -> nodes.key unchanged;
//     type -> node kind, including epic, ticket, task, release, sprint and
//     cost_unit; title -> title; description (or knowledge body) -> body;
//     status -> state; created_at/updated_at -> node timestamps; deleted_at ->
//     fields.classic.record.deleted_at pending an R1 trash policy. project_id
//     -> project parent node; parent_id and parent relation -> issue parent
//     node on a second pass. issue_number, priority, acceptance_criteria,
//     notes, report_summary, cost_unit, release, billing_type, total_budget,
//     rate_hourly/lp, start/end dates, estimate_hours/lp, ar_hours/lp,
//     time_override, group_state, sprint_state, jira_id/version/text,
//     pharos_request_id, color, sprint_ids, archived, assignee_id, created_by,
//     accepted_at/by, invoiced_at/number, deleted_by, tags, booked/budget
//     hours, time_logged/rollup/total, and AI work status -> fields.classic.record.
//     assignee_id, created_by, accepted_by and deleted_by also resolve to
//     fields.classic.principals UUIDs when their users are in the snapshot.
//     Orphan sprint issues are root nodes (their full SPRINT-<id> keys stay
//     intact); deleted issues are read through /issues/trash. Derived display
//     fields (assignee, children, creator/editor names) are
//     retained there too; no inferred authorship is manufactured.
//   - knowledge entries of types memory, runbook, external_system,
//     related_project and guideline are read from the unified per-project
//     knowledge API, merged with GET /issues/<id> for their original issue_key,
//     then imported as nodes of matching kinds. slug, metadata, reference_count,
//     last_referenced_at, content_revised_at, needs_review and review_reason
//     -> fields.classic.record; title/body/status -> native node columns.
//     Project knowledge is imported. User and instance memory APIs are separate
//     classic scopes; the project knowledge list does not enumerate them.
//   - issue_relations parent -> nodes.parent_id; depends_on -> directed blocks
//     from dependency to dependent; relates -> canonical symmetric relates;
//     duplicates and cites -> same-named node_relations. groups, sprint,
//     cost_unit, release and other classic relation types cannot fit the
//     R1 relation type constraint; impacts also has no equivalent. Their full
//     records are retained as import.relation events. All relations are also
//     retained as events.
//     Relations to out-of-scope nodes remain in import.relation events, but
//     no dangling Aeon relation is created.
//   - comments.id, issue_id, author_id/name, avatar_path, body, visibility,
//     client_request_id and created_at -> import.comment event payload and
//     event time. R1 has no separate comment table yet.
//   - issue_history.id, issue_id, changed_by/name, snapshot, changed_at,
//     agent_name and session_id -> import.history event payload and event time.
//     Historical snapshots are retained verbatim; they are not replayed as
//     native edits. R1 event actor is the importer agent because classic user
//     identity is historical data within the payload.
//   - attachment.id, issue_id, object_key, filename, content_type,
//     size_bytes, uploaded_by/uploader and created_at -> import.attachment
//     event payload and event time. File bytes are not fetched. The dry-run
//     report calls this out as attachments.file_bytes.
//   - users.id -> identities.subject <source-hash>:<id> with issuer
//     paimos-classic; username -> identity display_name and principal name;
//     email -> identity email; role -> principal roles; created_at -> both
//     identity and principal creation timestamps; remaining public user
//     profile/preferences and status are reported as unmapped. Password hashes,
//     API keys, sessions and TOTP secrets are never requested or imported.
//
// Every source API field not assigned a native column is reported by dry-run
// as unmapped and retained under fields.classic.record for nodes or the
// classic record in auxiliary events. The source identity is a hash of the
// source URL, not the bearer key. Re-runs update a node only when its imported
// fields change, reject a key owned by another source, and deduplicate
// auxiliary events by stable classic reference (and content digest for editable
// comment/attachment metadata). Native and auxiliary changes use events.Append
// within db.InTenant, preserving available classic timestamps on auxiliary
// events. Writes are serialized per tenant and source with a transaction
// advisory lock.
package importer
