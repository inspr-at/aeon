// SPDX-License-Identifier: AGPL-3.0-only
package stagehandoff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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
	ReviewedPlanDigest           string
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
	AuthorityOpen        bool       `json:"authority_open"`
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

func (m *Module) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// canonicalBodyDigest hashes sorted-key JSON from the request's decoded values.
// The launch bodies have only strings and one integral release_sequence; using
// json.Number preserves the latter without binary floating-point conversion.
func canonicalBodyDigest(raw []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil || body == nil {
		return "", fail(400, "invalid request")
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return "", fail(400, "invalid request")
	}
	canonical, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func launchRequestBody(w http.ResponseWriter, r *http.Request, dst any) (string, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		return "", fail(400, "invalid request")
	}
	copyRequest := *r
	copyRequest.Body = io.NopCloser(bytes.NewReader(raw))
	if err := decode(w, &copyRequest, dst); err != nil {
		return "", err
	}
	return canonicalBodyDigest(raw)
}

func launchKey(r *http.Request) (string, error) {
	key := r.Header.Get("Idempotency-Key")
	if !uuidRE.MatchString(key) {
		return "", fail(400, "Idempotency-Key must be a UUID")
	}
	return key, nil
}

// launchReplay runs under the handoff lock after the AEON-169 routed-principal
// check. It retains the receipt's original result and refreshes authority_open
// so a closed or superseded attempt cannot look actionable.
func (m *Module) launchReplay(ctx context.Context, tx pgx.Tx, h Handoff, p tenant.Principal, action, key, bodyDigest string, dst any) (bool, error) {
	var storedAction, storedPrincipal, storedDigest string
	var response []byte
	err := tx.QueryRow(ctx, `SELECT action,principal_id::text,body_digest_sha256,response FROM stage_launch_request_receipts WHERE handoff_id=$1::uuid AND idempotency_key=$2::uuid`, h.ID, key).Scan(&storedAction, &storedPrincipal, &storedDigest, &response)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if storedAction != action || storedPrincipal != p.ID || storedDigest != bodyDigest {
		return true, fail(409, "idempotency_conflict")
	}
	if h.Result != nil && !m.clock().Before(h.Result.CompletedAt.Add(24*time.Hour)) {
		return true, fail(409, "idempotency_conflict")
	}
	if err := json.Unmarshal(response, dst); err != nil {
		return true, err
	}
	open, err := authorityOpen(ctx, tx, h)
	if err != nil {
		return true, err
	}
	switch receipt := dst.(type) {
	case *LaunchAdmission:
		receipt.AuthorityOpen = open
	case *LaunchConsumption:
		receipt.AuthorityOpen = open
	}
	return true, nil
}

func saveLaunchReceipt(ctx context.Context, tx pgx.Tx, p tenant.Principal, handoffID, action, key, bodyDigest string, response any) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO stage_launch_request_receipts(tenant_id,handoff_id,action,principal_id,idempotency_key,body_digest_sha256,response) VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5::uuid,$6,$7::jsonb)`, p.TenantID, handoffID, action, p.ID, key, bodyDigest, raw)
	return err
}

// AdmitLaunch accepts an optional UUID key for in-process callers. HTTP always
// supplies it; an omitted key retains the original in-process interface.
func (m *Module) AdmitLaunch(ctx context.Context, p tenant.Principal, authorization, handoffID string, a Artifact, key ...string) (LaunchAdmission, error) {
	body, err := json.Marshal(a)
	if err != nil {
		return LaunchAdmission{}, err
	}
	bodyDigest, err := canonicalBodyDigest(body)
	if err != nil {
		return LaunchAdmission{}, err
	}
	requestKey := ""
	if len(key) > 0 {
		requestKey = key[0]
	}
	return m.admitLaunchRequest(ctx, p, authorization, handoffID, a, requestKey, bodyDigest)
}

func (m *Module) admitLaunchRequest(ctx context.Context, p tenant.Principal, authorization, handoffID string, a Artifact, key, bodyDigest string) (LaunchAdmission, error) {
	var out LaunchAdmission
	if (key != "" && !uuidRE.MatchString(key)) || !uuidRE.MatchString(handoffID) {
		return out, fail(400, "invalid launch artifact")
	}
	err := db.InTenant(tenant.WithPrincipal(ctx, p), m.pool, p.TenantID, func(tx pgx.Tx) error {
		if err := requireActiveAgent(ctx, tx, p); err != nil {
			return err
		}
		h, err := loadHandoff(ctx, tx, handoffID, true)
		if err != nil {
			return err
		}
		if err := requireRoutedPrincipal(ctx, tx, p, h); err != nil {
			return err
		}
		if key != "" {
			found, err := m.launchReplay(ctx, tx, h, p, "admit", key, bodyDigest, &out)
			if found || err != nil {
				return err
			}
			if h.Admission != nil {
				return fail(409, "launch admission already exists")
			}
		}
		if !hexRE.MatchString(a.DigestSHA256) || !hexRE.MatchString(a.ManifestDigestSHA256) || a.VersionScheme == "" || a.Version == "" || a.ReleaseChannel == "" || a.ReleaseSequence < 1 || a.CommitDigest == "" || a.ManifestCoordinate == "" {
			return fail(400, "invalid launch artifact")
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
		var readiness LaunchReadiness
		if checker, ok := m.launchChecks.(interface {
			CheckLaunchTx(context.Context, pgx.Tx, Handoff, Artifact) (LaunchReadiness, error)
		}); ok {
			readiness, err = checker.CheckLaunchTx(ctx, tx, h, a)
		} else {
			readiness, err = m.launchChecks.CheckLaunch(ctx, p.TenantID, h.ProjectNodeID, h.ReleaseNodeID, a)
		}
		if err != nil {
			var closed *apiError
			if errors.As(err, &closed) {
				return closed
			}
			return fail(409, "Pharos launch checks refused")
		}
		if readiness.ReviewedArtifactDigestSHA256 != a.DigestSHA256 || !readiness.BackupReady || !readiness.HostReady || !launchFresh(readiness.ObservedAt, time.Now()) {
			return fail(409, "Pharos launch checks are stale or incomplete")
		}
		binding := launchBinding(h, a.DigestSHA256, readiness.ReviewedPlanDigest)
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
		out.AuthorityOpen = true
		_, err = events.Append(ctx, tx, p, events.Change{Type: "stage_handoff.launch_admitted", NodeID: &h.ReleaseNodeID, After: map[string]any{"handoff_id": h.ID, "admission_id": out.ID}})
		if err != nil || key == "" {
			return err
		}
		return saveLaunchReceipt(ctx, tx, p, h.ID, "admit", key, bodyDigest, out)
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
	out.AuthorityOpen = true
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
	key, err := launchKey(r)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	var in Artifact
	bodyDigest, err := launchRequestBody(w, r, &in)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	out, err := m.admitLaunchRequest(r.Context(), p, r.Header.Get("Authorization"), id, in, key, bodyDigest)
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
	key, err := launchKey(r)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	var in struct {
		AdmissionID string `json:"admission_id"`
	}
	bodyDigest, err := launchRequestBody(w, r, &in)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	out, err := m.consumeLaunchRequest(r.Context(), p, r.Header.Get("Authorization"), id, in.AdmissionID, key, bodyDigest)
	respond(w, http.StatusOK, out, err)
}

func (m *Module) ConsumeLaunch(ctx context.Context, p tenant.Principal, authorization, handoffID, admissionID string) error {
	_, err := m.consumeLaunchRequest(ctx, p, authorization, handoffID, admissionID, "", "")
	return err
}

type LaunchConsumption struct {
	HandoffID     string    `json:"handoff_id"`
	AdmissionID   string    `json:"admission_id"`
	Consumed      bool      `json:"consumed"`
	ConsumedAt    time.Time `json:"consumed_at"`
	AuthorityOpen bool      `json:"authority_open"`
}

// ConsumeLaunchWithKey gives in-process adapters the same durable receipt as
// HTTP. The key and canonical request digest are bound in one transaction.
func (m *Module) ConsumeLaunchWithKey(ctx context.Context, p tenant.Principal, authorization, handoffID, admissionID, key string) (LaunchConsumption, error) {
	body, err := json.Marshal(struct {
		AdmissionID string `json:"admission_id"`
	}{admissionID})
	if err != nil {
		return LaunchConsumption{}, err
	}
	digest, err := canonicalBodyDigest(body)
	if err != nil {
		return LaunchConsumption{}, err
	}
	return m.consumeLaunchRequest(ctx, p, authorization, handoffID, admissionID, key, digest)
}

func (m *Module) consumeLaunchRequest(ctx context.Context, p tenant.Principal, authorization, handoffID, admissionID, key, bodyDigest string) (LaunchConsumption, error) {
	var out LaunchConsumption
	if !uuidRE.MatchString(handoffID) {
		return out, fail(404, "admission not found")
	}
	if key != "" && !uuidRE.MatchString(key) {
		return out, fail(400, "Idempotency-Key must be a UUID")
	}
	err := db.InTenant(tenant.WithPrincipal(ctx, p), m.pool, p.TenantID, func(tx pgx.Tx) error {
		if err := requireActiveAgent(ctx, tx, p); err != nil {
			return err
		}
		h, err := loadHandoff(ctx, tx, handoffID, true)
		if err != nil {
			return err
		}
		if err := requireRoutedPrincipal(ctx, tx, p, h); err != nil {
			return err
		}
		if key != "" {
			found, err := m.launchReplay(ctx, tx, h, p, "consume", key, bodyDigest, &out)
			if found || err != nil {
				return err
			}
		}
		if !uuidRE.MatchString(admissionID) {
			return fail(404, "admission not found")
		}
		var id, storedBinding, storedArtifact string
		var storedEpoch int64
		err = tx.QueryRow(ctx, `SELECT handoff_id::text, binding_digest_sha256, artifact_digest_sha256, authority_epoch FROM stage_launch_admissions WHERE id=$1::uuid FOR UPDATE`, admissionID).Scan(&id, &storedBinding, &storedArtifact, &storedEpoch)
		if errors.Is(err, pgx.ErrNoRows) {
			return fail(404, "admission not found")
		}
		if err != nil {
			return err
		}
		if id != handoffID {
			return fail(404, "admission not found")
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
		if storedEpoch != h.AuthorityEpoch {
			return fail(409, "launch binding drifted")
		}
		binding, err := recomputeLaunchBinding(ctx, tx, h, storedArtifact, true)
		if err != nil {
			var closed *apiError
			if errors.As(err, &closed) {
				return fail(409, "launch binding drifted")
			}
			return err
		}
		if binding != storedBinding {
			return fail(409, "launch binding drifted")
		}
		err = tx.QueryRow(ctx, `UPDATE stage_launch_admissions SET consumed_at=now(),consumed_by_principal_id=$4::uuid WHERE id=$1::uuid AND handoff_id=$2::uuid AND authority_epoch=$3 AND consumed_at IS NULL AND expires_at>now() RETURNING consumed_at`, admissionID, id, h.AuthorityEpoch, p.ID).Scan(&out.ConsumedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return fail(409, "launch admission is spent or expired")
		}
		if err != nil {
			return err
		}
		out.HandoffID = h.ID
		out.AdmissionID = admissionID
		out.Consumed = true
		out.AuthorityOpen = true
		_, err = events.Append(ctx, tx, p, events.Change{Type: "stage_handoff.launch_consumed", NodeID: &h.ReleaseNodeID, After: map[string]any{"handoff_id": h.ID, "admission_id": admissionID}})
		if err != nil || key == "" {
			return err
		}
		return saveLaunchReceipt(ctx, tx, p, h.ID, "consume", key, bodyDigest, out)
	})
	return out, err
}
func consumedAdmission(ctx context.Context, tx pgx.Tx, h Handoff, a Artifact) error {
	binding, err := recomputeLaunchBinding(ctx, tx, h, a.DigestSHA256, false)
	if err != nil {
		return err
	}
	var ok bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM stage_launch_admissions WHERE handoff_id=$1::uuid AND binding_digest_sha256=$2 AND artifact_digest_sha256=$3 AND authority_epoch=$4 AND consumed_at IS NOT NULL AND expires_at>now())`, h.ID, binding, a.DigestSHA256, h.AuthorityEpoch).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return fail(http.StatusConflict, "deployment lacks consumed launch admission")
	}
	return nil
}
