// SPDX-License-Identifier: AGPL-3.0-only

package stagehandoff

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inspr-at/paimos/internal/db"
)

const launchBindingDomain = "inspr.aeon.launch-binding.v1"

// LaunchFreshness is how old a launch_readiness observation may be. It is
// Aeon's clock. Callers cannot pass a window.
const LaunchFreshness = 900 * time.Second

// EvidenceLaunchChecks is the production launch provider. It reads the
// handoff's built artifact and launch_readiness rows. It does not trust the
// admit body as evidence.
type EvidenceLaunchChecks struct {
	Pool *pgxpool.Pool
}

func (c EvidenceLaunchChecks) CheckLaunch(ctx context.Context, tenantID, projectID, releaseID string, a Artifact) (LaunchReadiness, error) {
	var out LaunchReadiness
	if c.Pool == nil {
		return out, fail(409, "Pharos launch checks are unavailable")
	}
	err := db.InTenant(ctx, c.Pool, tenantID, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `SELECT id::text FROM stage_handoffs WHERE project_node_id=$1::uuid AND release_node_id=$2::uuid AND stage='deploy' AND operation='deploy' AND plugin_id='pharos' AND state IN ('requested','active') ORDER BY authority_epoch DESC LIMIT 1`, projectID, releaseID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return fail(409, "launch readiness is missing")
		}
		if err != nil {
			return err
		}
		h, err := loadHandoff(ctx, tx, id, false)
		if err != nil {
			return err
		}
		out, err = c.CheckLaunchTx(ctx, tx, h, a)
		return err
	})
	return out, err
}

// CheckLaunchTx evaluates admission against the open handoff transaction.
func (EvidenceLaunchChecks) CheckLaunchTx(ctx context.Context, tx pgx.Tx, h Handoff, echoed Artifact) (LaunchReadiness, error) {
	var out LaunchReadiness
	if h.Operation != "deploy" || h.PluginID != "pharos" {
		return out, fail(409, "not an active Pharos deployment")
	}
	var state string
	err := tx.QueryRow(ctx, `SELECT state FROM journey_releases WHERE release_node_id=$1::uuid`, h.ReleaseNodeID).Scan(&state)
	if err != nil {
		return out, err
	}
	if state != "candidate" && state != "deploying" {
		return out, fail(409, "release is not a reviewed or approved candidate")
	}
	for _, gate := range []string{"candidate", "deploy"} {
		live, err := gateLive(ctx, tx, h.ReleaseNodeID, gate)
		if err != nil {
			return out, err
		}
		if !live {
			return out, fail(409, "deploy stage has no live gate approval")
		}
	}
	held, ok, err := loadHeldArtifact(ctx, tx, h.ID)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, fail(409, "candidate artifact is not recorded")
	}
	if held != echoed {
		return out, fail(409, "artifact does not match the approved candidate")
	}
	plan, observed, present, err := qualifyLaunchReadiness(ctx, tx, h, time.Now(), true)
	if err != nil {
		return out, err
	}
	if !present {
		return out, fail(409, "launch readiness is missing")
	}
	out.ReviewedArtifactDigestSHA256 = held.DigestSHA256
	out.ReviewedPlanDigest = plan
	out.BackupReady = true
	out.HostReady = true
	out.ObservedAt = observed
	return out, nil
}

func launchFresh(observed, now time.Time) bool {
	if observed.IsZero() || now.IsZero() {
		return false
	}
	delta := now.Sub(observed)
	if delta < 0 {
		delta = -delta
	}
	return delta <= LaunchFreshness
}

type readinessRow struct {
	sequence   int64
	epoch      int64
	plan       string
	hostEval   bool
	targetPass bool
	backup     bool
	observed   time.Time
}

func (r readinessRow) flagsTrue() bool {
	return r.hostEval && r.targetPass && r.backup
}

func falseFlag(r readinessRow) string {
	switch {
	case !r.hostEval:
		return "all_host_eval_passed"
	case !r.targetPass:
		return "target_build_passed"
	default:
		return "backup_ready"
	}
}

// qualifyLaunchReadiness returns the reviewed plan digest sealed into a launch
// binding. present is false only when the handoff has no launch_readiness row
// at all. Any row that does not qualify is an error. requireFresh applies
// LaunchFreshness; binding recomputation after a consumed admission does not,
// because ageing is not a change of identity.
func qualifyLaunchReadiness(ctx context.Context, tx pgx.Tx, h Handoff, now time.Time, requireFresh bool) (plan string, observed time.Time, present bool, err error) {
	rows, err := tx.Query(ctx, `SELECT sequence, authority_epoch, reviewed_plan_digest, all_host_eval_passed, target_build_passed, backup_ready, observed_at FROM stage_handoff_evidence WHERE handoff_id=$1::uuid AND kind='launch_readiness' ORDER BY sequence`, h.ID)
	if err != nil {
		return "", time.Time{}, false, err
	}
	defer rows.Close()
	var all, epochRows []readinessRow
	for rows.Next() {
		var r readinessRow
		if err := rows.Scan(&r.sequence, &r.epoch, &r.plan, &r.hostEval, &r.targetPass, &r.backup, &r.observed); err != nil {
			return "", time.Time{}, false, err
		}
		all = append(all, r)
		if r.epoch == h.AuthorityEpoch {
			epochRows = append(epochRows, r)
		}
	}
	if err := rows.Err(); err != nil {
		return "", time.Time{}, false, err
	}
	if len(all) == 0 {
		return "", time.Time{}, false, nil
	}
	if len(epochRows) == 0 {
		return "", time.Time{}, true, fail(409, "launch readiness authority_epoch does not match")
	}
	anchor := epochRows[0]
	for _, later := range all {
		if later.sequence > anchor.sequence && (!later.flagsTrue() || later.plan != anchor.plan) {
			return "", time.Time{}, true, fail(409, "launch readiness was contradicted")
		}
	}
	if !anchor.flagsTrue() {
		return "", time.Time{}, true, fail(409, "launch readiness flag "+falseFlag(anchor)+" is false")
	}
	var freshAt time.Time
	fresh := false
	for _, r := range epochRows {
		if launchFresh(r.observed, now) {
			fresh = true
			freshAt = r.observed
		}
	}
	if requireFresh && !fresh {
		return "", time.Time{}, true, fail(409, "launch readiness is stale")
	}
	if !fresh {
		freshAt = epochRows[len(epochRows)-1].observed
	}
	return anchor.plan, freshAt, true, nil
}

func loadHeldArtifact(ctx context.Context, tx pgx.Tx, handoffID string) (Artifact, bool, error) {
	var a Artifact
	err := tx.QueryRow(ctx, `SELECT version_scheme, version, release_channel, release_sequence, oci_config_digest, commit_digest, release_manifest_coordinate, release_manifest_digest FROM stage_handoff_build_evidence WHERE handoff_id=$1::uuid`, handoffID).Scan(&a.VersionScheme, &a.Version, &a.ReleaseChannel, &a.ReleaseSequence, &a.DigestSHA256, &a.CommitDigest, &a.ManifestCoordinate, &a.ManifestDigestSHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return Artifact{}, false, nil
	}
	if err != nil {
		return Artifact{}, false, err
	}
	return a, true, nil
}

func launchBinding(h Handoff, artifactDigest, reviewedPlan string) string {
	body, err := json.Marshal(struct {
		ArtifactDigestSHA256 string `json:"artifact_digest_sha256"`
		AuthorityEpoch       int64  `json:"authority_epoch"`
		HandoffID            string `json:"handoff_id"`
		ReleaseNodeID        string `json:"release_node_id"`
		ReviewedPlanDigest   string `json:"reviewed_plan_digest"`
	}{
		ArtifactDigestSHA256: artifactDigest,
		AuthorityEpoch:       h.AuthorityEpoch,
		HandoffID:            h.ID,
		ReleaseNodeID:        h.ReleaseNodeID,
		ReviewedPlanDigest:   reviewedPlan,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(append(append([]byte(launchBindingDomain), 0), body...))
	return hex.EncodeToString(sum[:])
}

// recomputeLaunchBinding seals the current handoff. A handoff with no
// launch_readiness row seals an empty plan, which is how a test double that
// does not record readiness stays internally consistent. A recorded row that
// no longer qualifies is drift.
func recomputeLaunchBinding(ctx context.Context, tx pgx.Tx, h Handoff, artifactDigest string, requireFresh bool) (string, error) {
	held, ok, err := loadHeldArtifact(ctx, tx, h.ID)
	if err != nil {
		return "", err
	}
	if ok && held.DigestSHA256 != artifactDigest {
		return "", fail(409, "launch binding drifted")
	}
	plan, _, present, err := qualifyLaunchReadiness(ctx, tx, h, time.Now(), requireFresh)
	if err != nil {
		return "", err
	}
	if !present {
		plan = ""
	}
	return launchBinding(h, artifactDigest, plan), nil
}
