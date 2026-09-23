// SPDX-License-Identifier: AGPL-3.0-only

// Package stagehandoff implements one fenced request, ordered evidence and a
// terminal result for compiled stage plugins. The coordinator mounts New.
package stagehandoff

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var hexRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
var symbolicRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,127}$`)

func digest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

type Module struct {
	pool         *pgxpool.Pool
	registry     *plugins.Registry
	launchChecks LaunchChecks
}

var _ httpapi.Module = (*Module)(nil)

// New exposes the handoff routes. Use plugins.Builtin for the shared registry,
// then mount this module and plugins.NewWithRegistry from cmd/aeon.
// A LaunchChecks provider is required before Pharos can admit a host change.
func New(pool *pgxpool.Pool, registry *plugins.Registry, checks ...LaunchChecks) httpapi.Module {
	var guard LaunchChecks
	if len(checks) > 0 {
		guard = checks[0]
	}
	return &Module{pool: pool, registry: registry, launchChecks: guard}
}
func (m *Module) Mount(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/stage-handoffs", m.request)
	mux.HandleFunc("GET /api/stage-handoffs/{handoffId}", m.get)
	mux.HandleFunc("POST /api/stage-handoffs/{handoffId}/evidence", m.evidence)
	mux.HandleFunc("POST /api/stage-handoffs/{handoffId}/result", m.result)
}

type RequestWrite struct {
	ProjectNodeID           string `json:"project_node_id"`
	ReleaseNodeID           string `json:"release_node_id"`
	Stage                   string `json:"stage"`
	Operation               string `json:"operation"`
	ExpectedJourneyRevision int64  `json:"expected_journey_revision"`
	IdempotencyKey          string `json:"idempotency_key"`
}
type Handoff struct {
	ID                     string    `json:"id"`
	ProjectNodeID          string    `json:"project_node_id"`
	ReleaseNodeID          string    `json:"release_node_id"`
	Stage                  string    `json:"stage"`
	Operation              string    `json:"operation"`
	PluginID               string    `json:"plugin_id"`
	Attempt                int       `json:"attempt"`
	AuthorityEpoch         int64     `json:"authority_epoch"`
	JourneyRevision        int64     `json:"journey_revision"`
	State                  string    `json:"state"`
	ExpiresAt              time.Time `json:"expires_at"`
	EvidenceCeiling        []string  `json:"evidence_ceiling"`
	PlanDigest             string    `json:"plan_digest"`
	PredecessorDigest      string    `json:"predecessor_digest"`
	ContextDigest          string    `json:"context_digest"`
	PrerequisiteSealSHA256 string    `json:"prerequisite_seal_sha256"`
	Result                 *Result   `json:"result,omitempty"`
}
type Artifact struct {
	VersionScheme        string `json:"version_scheme"`
	Version              string `json:"version"`
	ReleaseChannel       string `json:"release_channel"`
	ReleaseSequence      int64  `json:"release_sequence"`
	DigestSHA256         string `json:"digest_sha256"`
	CommitDigest         string `json:"commit_digest"`
	ManifestCoordinate   string `json:"manifest_coordinate"`
	ManifestDigestSHA256 string `json:"manifest_digest_sha256"`
}
type EvidenceWrite struct {
	Sequence        int64     `json:"sequence"`
	Kind            string    `json:"kind"`
	Outcome         string    `json:"outcome"`
	ObservedAt      time.Time `json:"observed_at"`
	AuthorityEpoch  int64     `json:"authority_epoch"`
	Workflow        *string   `json:"workflow,omitempty"`
	Environment     *string   `json:"environment,omitempty"`
	Artifact        *Artifact `json:"artifact,omitempty"`
	Authorized      *bool     `json:"authorized,omitempty"`
	CredentialReady *bool     `json:"credential_ready,omitempty"`
}
type Evidence struct {
	EvidenceWrite
	HandoffID  string    `json:"handoff_id"`
	ReceivedAt time.Time `json:"received_at"`
}
type ResultWrite struct {
	Outcome                string  `json:"outcome"`
	TerminalSequence       int64   `json:"terminal_sequence"`
	AuthorityEpoch         int64   `json:"authority_epoch"`
	PrerequisiteSealSHA256 string  `json:"prerequisite_seal_sha256"`
	BlockerCode            *string `json:"blocker_code,omitempty"`
}
type Result struct {
	ResultWrite
	HandoffID   string    `json:"handoff_id"`
	CompletedAt time.Time `json:"completed_at"`
}

type apiError struct {
	code    int
	message string
}

func (e *apiError) Error() string     { return e.message }
func fail(code int, msg string) error { return &apiError{code, msg} }
func respond(w http.ResponseWriter, status int, v any, err error) {
	w.Header().Set("Cache-Control", "no-store")
	if err == nil {
		httpapi.WriteJSON(w, status, v)
		return
	}
	var e *apiError
	if errors.As(err, &e) {
		httpapi.WriteError(w, e.code, e.message)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.WriteError(w, 404, "handoff not found")
		return
	}
	slog.Error("stagehandoff", "err", err)
	httpapi.WriteError(w, 500, "internal")
}
func principal(w http.ResponseWriter, r *http.Request) (tenant.Principal, bool) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok || !uuidRE.MatchString(p.ID) || !uuidRE.MatchString(p.TenantID) {
		httpapi.WriteError(w, 401, "authentication required")
		return tenant.Principal{}, false
	}
	return p, true
}
func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fail(400, "invalid request")
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return fail(400, "invalid request")
	}
	return nil
}
func route(stage, operation string) (plugin string, ceiling []string, gate string, ok bool) {
	switch {
	case stage == "access" && operation == "prepare":
		return "janus", []string{"authorization", "credential_handoff"}, "", true
	case stage == "access" && operation == "apply":
		return "janus", []string{"authorization", "credential_handoff"}, "access", true
	case stage == "deploy" && operation == "deploy":
		return "pharos", []string{"deployment"}, "deploy", true
	case stage == "deploy" && operation == "verify":
		return "pharos", []string{"verification"}, "deploy", true
	}
	return "", nil, "", false
}
func (m *Module) request(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r)
	if !ok {
		return
	}
	var in RequestWrite
	if err := decode(w, r, &in); err != nil {
		respond(w, 0, nil, err)
		return
	}
	if !uuidRE.MatchString(in.ProjectNodeID) || !uuidRE.MatchString(in.ReleaseNodeID) || in.ExpectedJourneyRevision < 1 || len(in.IdempotencyKey) < 1 || len(in.IdempotencyKey) > 128 {
		respond(w, 0, nil, fail(400, "invalid handoff request"))
		return
	}
	plugin, ceiling, gate, ok := route(in.Stage, in.Operation)
	if !ok {
		respond(w, 0, nil, fail(400, "invalid stage operation"))
		return
	}
	var out Handoff
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = m.create(r.Context(), tx, p, in, plugin, ceiling, gate)
		return err
	})
	respond(w, 201, out, err)
}
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r)
	if !ok {
		return
	}
	id := r.PathValue("handoffId")
	if !uuidRE.MatchString(id) {
		respond(w, 0, nil, fail(404, "handoff not found"))
		return
	}
	var out Handoff
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error { var err error; out, err = loadHandoff(r.Context(), tx, id, false); return err })
	respond(w, 200, out, err)
}
func (m *Module) evidence(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r)
	if !ok {
		return
	}
	id := r.PathValue("handoffId")
	if !uuidRE.MatchString(id) {
		respond(w, 0, nil, fail(404, "handoff not found"))
		return
	}
	var in EvidenceWrite
	if err := decode(w, r, &in); err != nil {
		respond(w, 0, nil, err)
		return
	}
	if err := validateEvidence(in); err != nil {
		respond(w, 0, nil, err)
		return
	}
	var out Evidence
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = m.appendEvidence(r.Context(), tx, p, r.Header.Get("Authorization"), id, in)
		return err
	})
	respond(w, 201, out, err)
}
func (m *Module) result(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r)
	if !ok {
		return
	}
	id := r.PathValue("handoffId")
	if !uuidRE.MatchString(id) {
		respond(w, 0, nil, fail(404, "handoff not found"))
		return
	}
	var in ResultWrite
	if err := decode(w, r, &in); err != nil {
		respond(w, 0, nil, err)
		return
	}
	if in.TerminalSequence < 1 || in.AuthorityEpoch < 1 || !hexRE.MatchString(in.PrerequisiteSealSHA256) || (in.Outcome != "succeeded" && in.Outcome != "failed") || (in.Outcome == "succeeded" && in.BlockerCode != nil) || (in.Outcome == "failed" && in.BlockerCode == nil) {
		respond(w, 0, nil, fail(400, "invalid result"))
		return
	}
	if in.BlockerCode != nil && !slices.Contains([]string{"dependency_pending", "dependency_failed", "reporter_stale", "external_waiting", "policy_refused"}, *in.BlockerCode) {
		respond(w, 0, nil, fail(400, "invalid blocker"))
		return
	}
	var out Result
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = m.close(r.Context(), tx, p, r.Header.Get("Authorization"), id, in)
		return err
	})
	respond(w, 200, out, err)
}
func validateEvidence(e EvidenceWrite) error {
	if e.Sequence < 1 || e.AuthorityEpoch < 1 || e.ObservedAt.IsZero() || e.ObservedAt.After(time.Now().Add(5*time.Minute)) {
		return fail(400, "invalid evidence")
	}
	if !slices.Contains([]string{"succeeded", "failed", "satisfied", "blocked"}, e.Outcome) {
		return fail(400, "invalid evidence outcome")
	}
	switch e.Kind {
	case "deployment", "verification":
		if e.Workflow == nil || e.Environment == nil || e.Artifact == nil || e.Authorized != nil || e.CredentialReady != nil {
			return fail(400, "invalid Pharos evidence")
		}
		a := e.Artifact
		if !symbolicRE.MatchString(*e.Workflow) || !symbolicRE.MatchString(*e.Environment) || !slices.Contains([]string{"legacy", "inspr-calendar-v1", "inspr-calendar-v2"}, a.VersionScheme) || a.Version == "" || a.ReleaseChannel == "" || a.ReleaseSequence < 1 || !hexRE.MatchString(a.DigestSHA256) || a.CommitDigest == "" || a.ManifestCoordinate == "" || !hexRE.MatchString(a.ManifestDigestSHA256) {
			return fail(400, "invalid artifact identity")
		}
	case "authorization":
		if e.Authorized == nil || e.CredentialReady != nil || e.Artifact != nil || e.Workflow != nil || e.Environment != nil {
			return fail(400, "invalid Janus evidence")
		}
	case "credential_handoff":
		if e.CredentialReady == nil || e.Authorized != nil || e.Artifact != nil || e.Workflow != nil || e.Environment != nil {
			return fail(400, "invalid Janus evidence")
		}
	default:
		return fail(400, "invalid evidence kind")
	}
	return nil
}
