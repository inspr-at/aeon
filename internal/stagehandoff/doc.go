// SPDX-License-Identifier: AGPL-3.0-only

// Package stagehandoff implements one fenced request, ordered evidence and a
// terminal result for compiled stage plugins. The coordinator mounts New.
//
// # Launch readiness
//
// Kind launch_readiness is Pharos observation posted by the routed principal
// on POST /api/stage-handoffs/{id}/evidence. It is allowed on an active Pharos
// deploy handoff. The fields are reviewed_plan_digest (lowercase sha256 hex),
// host, all_host_eval_passed, target_build_passed, backup_ready,
// backup_observed_at, restart_required, running_kernel, expected_kernel,
// observed_at and authority_epoch, plus the evidence sequence and outcome.
// Flags are the authority. The outcome word does not admit a launch. Other
// evidence kinds must omit these fields.
//
// Aeon's artifact record is stage_handoff_build_evidence on that deploy
// handoff, written when a built receipt is accepted. It is the reviewed or
// approved candidate for the release, and admission also requires a live
// deploy gate (the deploy stage's gate_approval_id) and a live candidate
// gate while the release is in candidate or deploying. The mapping onto
// StageArtifact is version_scheme, version, release_channel, release_sequence,
// digest_sha256 = oci_config_digest, commit_digest, manifest_coordinate =
// release_manifest_coordinate, manifest_digest_sha256 = release_manifest_digest.
// The admit body must equal that row. The body confirms identity. It is not
// evidence. A missing row refuses.
//
// EvidenceLaunchChecks admits only when a launch_readiness row exists for
// this handoff and its current authority_epoch, all_host_eval_passed,
// target_build_passed and backup_ready are true, and observed_at is within
// LaunchFreshness (900 seconds) of Aeon's clock. The caller cannot supply
// that window. A later launch_readiness row for the handoff contradicts the
// claim when it has a false flag or a different reviewed_plan_digest, and
// admission is refused. restart_required is stored and is not an admission
// input.
//
// # Launch binding
//
// binding_digest_sha256 is hex(sha256("inspr.aeon.launch-binding.v1" ||
// 0x00 || canonical_json)). canonical_json is UTF-8 with sorted keys and no
// whitespace:
//
//	{"artifact_digest_sha256":"<hex>","authority_epoch":<number>,"handoff_id":"<uuid>","release_node_id":"<uuid>","reviewed_plan_digest":"<hex>"}
//
// authority_epoch is a JSON number. Consume recomputes the digest from the
// current handoff, the built artifact and the qualifying readiness row, and
// refuses on any drift.
package stagehandoff
