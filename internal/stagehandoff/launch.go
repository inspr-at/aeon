// SPDX-License-Identifier: AGPL-3.0-only
package stagehandoff

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ClosedLaunchChecks refuses every admission. The server wires it so a missing
// observation source cannot be mistaken for readiness. A replacement must
// independently report a fresh reviewed artifact digest, backup and host
// readiness. Echoing the caller's artifact is not a check.
type ClosedLaunchChecks struct{}

func (ClosedLaunchChecks) CheckLaunch(context.Context, string, string, string, Artifact) (LaunchReadiness, error) {
	return LaunchReadiness{}, errors.New("pharos launch observation is not configured")
}

// LaunchReadiness is fresh, value-free Pharos policy evidence. The provider
// checks artifact review, current backup and host readiness from its own
// bounded integration before the service issues one launch admission.
type LaunchReadiness struct {
	ReviewedArtifactDigestSHA256 string
	BackupReady                  bool
	HostReady                    bool
	ObservedAt                   time.Time
}
type LaunchChecks interface {
	CheckLaunch(context.Context, string, string, string, Artifact) (LaunchReadiness, error)
}

// LaunchAdmission is a one-use in-process authority for a reviewed artifact.
// Pharos must consume it before any host change and bind reported deployment
// evidence to the same artifact identity.
type LaunchAdmission struct {
	ID                   string     `json:"id"`
	HandoffID            string     `json:"handoff_id"`
	BindingDigestSHA256  string     `json:"binding_digest_sha256"`
	ArtifactDigestSHA256 string     `json:"artifact_digest_sha256"`
	AuthorityEpoch       int64      `json:"authority_epoch"`
	ExpiresAt            time.Time  `json:"expires_at"`
	ConsumedAt           *time.Time `json:"consumed_at,omitempty"`
}

// NewService gives the coordinator both an httpapi.Module and the narrow
// admission methods used by the in-process Pharos adapter. The coordinator
// supplies a LaunchChecks provider for current artifact review, backup and host
// readiness. A nil provider fails closed at admission.
func NewService(pool *pgxpool.Pool, registry *plugins.Registry, checks LaunchChecks) *Module {
	return &Module{pool: pool, registry: registry, launchChecks: checks}
}
func launchBinding(h Handoff, a Artifact) string {
	return digest(h.ContextDigest, h.PrerequisiteSealSHA256, a.VersionScheme, a.Version, a.ReleaseChannel, a.DigestSHA256, a.CommitDigest, a.ManifestCoordinate, a.ManifestDigestSHA256)
}
func (m *Module) AdmitLaunch(ctx context.Context, p tenant.Principal, authorization, handoffID string, a Artifact) (LaunchAdmission, error) {
	var out LaunchAdmission
	if !uuidRE.MatchString(handoffID) || !hexRE.MatchString(a.DigestSHA256) || !hexRE.MatchString(a.ManifestDigestSHA256) || a.VersionScheme == "" || a.Version == "" || a.ReleaseChannel == "" || a.ReleaseSequence < 1 || a.CommitDigest == "" || a.ManifestCoordinate == "" {
		return out, fail(400, "invalid launch artifact")
	}
	err := db.InTenant(tenant.WithPrincipal(ctx, p), m.pool, p.TenantID, func(tx pgx.Tx) error {
		h, err := loadHandoff(ctx, tx, handoffID, true)
		if err != nil {
			return err
		}
		if h.Operation != "deploy" || h.PluginID != "pharos" || h.Result != nil || closedHandoff(h.State) {
			return fail(409, "not an active Pharos deployment")
		}
		enabled, err := plugins.Enabled(ctx, tx, m.registry, p.TenantID, "pharos", "deploy")
		if err != nil {
			return err
		}
		if !enabled {
			return fail(403, "Pharos is disabled")
		}
		allowed, err := agentAllowed(ctx, tx, p, authorization, h)
		if err != nil {
			return err
		}
		if !allowed {
			return fail(403, "live agent grant required")
		}
		current, err := current(ctx, tx, h)
		if err != nil {
			return err
		}
		if !current {
			return fail(409, "handoff is stale")
		}
		for _, gate := range []string{"candidate", "deploy"} {
			ok, err := gateLive(ctx, tx, h.ReleaseNodeID, gate)
			if err != nil {
				return err
			}
			if !ok {
				return fail(403, "stage gate is not approved")
			}
		}
		var scheme, version *string
		var number int64
		err = tx.QueryRow(ctx, `SELECT version_scheme,version,number FROM journey_releases WHERE release_node_id=$1::uuid`, h.ReleaseNodeID).Scan(&scheme, &version, &number)
		if err != nil {
			return err
		}
		if scheme == nil || version == nil || *scheme != a.VersionScheme || *version != a.Version || number != a.ReleaseSequence {
			return fail(409, "artifact does not match reviewed release version")
		}
		if m.launchChecks == nil {
			return fail(409, "Pharos launch checks are unavailable")
		}
		readiness, err := m.launchChecks.CheckLaunch(ctx, p.TenantID, h.ProjectNodeID, h.ReleaseNodeID, a)
		if err != nil {
			return fail(409, "Pharos launch checks refused")
		}
		if readiness.ReviewedArtifactDigestSHA256 != a.DigestSHA256 || !readiness.BackupReady || !readiness.HostReady || readiness.ObservedAt.IsZero() || time.Since(readiness.ObservedAt) > 5*time.Minute || readiness.ObservedAt.After(time.Now().Add(time.Minute)) {
			return fail(409, "Pharos launch checks are stale or incomplete")
		}
		binding := launchBinding(h, a)
		err = tx.QueryRow(ctx, `INSERT INTO stage_launch_admissions(tenant_id,handoff_id,binding_digest_sha256,artifact_digest_sha256,authority_epoch,expires_at) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6) ON CONFLICT (tenant_id, handoff_id, binding_digest_sha256) DO NOTHING RETURNING id::text,expires_at`, p.TenantID, h.ID, binding, a.DigestSHA256, h.AuthorityEpoch, h.ExpiresAt).Scan(&out.ID, &out.ExpiresAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return replayAdmission(ctx, tx, h, a, binding, &out)
		}
		if err != nil {
			return err
		}
		out.HandoffID = h.ID
		out.BindingDigestSHA256 = binding
		out.ArtifactDigestSHA256 = a.DigestSHA256
		out.AuthorityEpoch = h.AuthorityEpoch
		_, err = events.Append(ctx, tx, p, events.Change{Type: "stage_handoff.launch_admitted", NodeID: &h.ReleaseNodeID, After: map[string]any{"handoff_id": h.ID, "admission_id": out.ID}})
		return err
	})
	return out, err
}

func closedHandoff(state string) bool {
	switch state {
	case "requested", "active":
		return false
	default:
		return true
	}
}

// replayAdmission returns the unconsumed admission for this binding. A spent
// or expired row is refused and is never replaced.
func replayAdmission(ctx context.Context, tx pgx.Tx, h Handoff, a Artifact, binding string, out *LaunchAdmission) error {
	var consumed *time.Time
	var artifact string
	var epoch int64
	err := tx.QueryRow(ctx, `SELECT id::text, expires_at, consumed_at, artifact_digest_sha256, authority_epoch FROM stage_launch_admissions WHERE handoff_id=$1::uuid AND binding_digest_sha256=$2`, h.ID, binding).Scan(&out.ID, &out.ExpiresAt, &consumed, &artifact, &epoch)
	if err != nil {
		return err
	}
	if consumed != nil || artifact != a.DigestSHA256 || epoch != h.AuthorityEpoch || !out.ExpiresAt.After(time.Now()) {
		return fail(409, "launch admission is spent or expired")
	}
	out.HandoffID = h.ID
	out.BindingDigestSHA256 = binding
	out.ArtifactDigestSHA256 = artifact
	out.AuthorityEpoch = epoch
	return nil
}

func (m *Module) admitLaunch(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r)
	if !ok {
		return
	}
	id := r.PathValue("handoffId")
	if !uuidRE.MatchString(id) {
		respond(w, 0, nil, fail(404, "handoff not found"))
		return
	}
	var in Artifact
	if err := decode(w, r, &in); err != nil {
		respond(w, 0, nil, err)
		return
	}
	out, err := m.AdmitLaunch(r.Context(), p, r.Header.Get("Authorization"), id, in)
	respond(w, http.StatusOK, out, err)
}

func (m *Module) consumeLaunch(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r)
	if !ok {
		return
	}
	id := r.PathValue("handoffId")
	if !uuidRE.MatchString(id) {
		respond(w, 0, nil, fail(404, "handoff not found"))
		return
	}
	var in struct {
		AdmissionID string `json:"admission_id"`
	}
	if err := decode(w, r, &in); err != nil {
		respond(w, 0, nil, err)
		return
	}
	err := m.ConsumeLaunch(r.Context(), p, r.Header.Get("Authorization"), id, in.AdmissionID)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"handoff_id": id, "admission_id": in.AdmissionID, "consumed": true}, nil)
}

func (m *Module) ConsumeLaunch(ctx context.Context, p tenant.Principal, authorization, handoffID, admissionID string) error {
	if !uuidRE.MatchString(admissionID) || !uuidRE.MatchString(handoffID) {
		return fail(404, "admission not found")
	}
	return db.InTenant(tenant.WithPrincipal(ctx, p), m.pool, p.TenantID, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `SELECT handoff_id::text FROM stage_launch_admissions WHERE id=$1::uuid FOR UPDATE`, admissionID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return fail(404, "admission not found")
		}
		if err != nil {
			return err
		}
		if id != handoffID {
			return fail(404, "admission not found")
		}
		h, err := loadHandoff(ctx, tx, id, true)
		if err != nil {
			return err
		}
		enabled, err := plugins.Enabled(ctx, tx, m.registry, p.TenantID, "pharos", "deploy")
		if err != nil {
			return err
		}
		if !enabled {
			return fail(403, "Pharos is disabled")
		}
		for _, gate := range []string{"candidate", "deploy"} {
			live, err := gateLive(ctx, tx, h.ReleaseNodeID, gate)
			if err != nil {
				return err
			}
			if !live {
				return fail(403, "stage gate is no longer approved")
			}
		}
		allowed, err := agentAllowed(ctx, tx, p, authorization, h)
		if err != nil {
			return err
		}
		if !allowed {
			return fail(403, "live agent grant required")
		}
		current, err := current(ctx, tx, h)
		if err != nil {
			return err
		}
		if !current {
			return fail(409, "handoff is stale")
		}
		tag, err := tx.Exec(ctx, `UPDATE stage_launch_admissions SET consumed_at=now() WHERE id=$1::uuid AND handoff_id=$2::uuid AND authority_epoch=$3 AND consumed_at IS NULL AND expires_at>now()`, admissionID, id, h.AuthorityEpoch)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fail(409, "launch admission is spent or expired")
		}
		_, err = events.Append(ctx, tx, p, events.Change{Type: "stage_handoff.launch_consumed", NodeID: &h.ReleaseNodeID, After: map[string]any{"handoff_id": h.ID, "admission_id": admissionID}})
		return err
	})
}
func consumedAdmission(ctx context.Context, tx pgx.Tx, h Handoff, a Artifact) error {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM stage_launch_admissions WHERE handoff_id=$1::uuid AND binding_digest_sha256=$2 AND artifact_digest_sha256=$3 AND authority_epoch=$4 AND consumed_at IS NOT NULL AND expires_at>now())`, h.ID, launchBinding(h, a), a.DigestSHA256, h.AuthorityEpoch).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return fail(http.StatusConflict, "deployment lacks consumed launch admission")
	}
	return nil
}
