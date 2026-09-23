// SPDX-License-Identifier: AGPL-3.0-only

// Package requirements implements R3 requirement creation and agreement.
// Mount New(pool) alongside releases.New(pool) through the coordinator's mux.
package requirements

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type module struct{ pool *pgxpool.Pool }

// New returns the requirements httpapi.Module. It never initializes a journey;
// the journey module must create the project projection and requirement kind.
func New(pool *pgxpool.Pool) httpapi.Module { return &module{pool: pool} }
func (m *module) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{projectId}/requirements", m.list)
	mux.HandleFunc("POST /api/projects/{projectId}/requirements", m.create)
	mux.HandleFunc("POST /api/projects/{projectId}/requirements/agree", m.agree)
}

type Requirement struct {
	NodeID    string   `json:"node_id"`
	ProjectID string   `json:"project_node_id"`
	Kind      string   `json:"kind"`
	Revision  int64    `json:"revision"`
	Status    string   `json:"status"`
	Title     string   `json:"title"`
	FeatureID *string  `json:"feature_node_id"`
	TicketIDs []string `json:"generated_ticket_ids"`
}
type createInput struct {
	Kind     string  `json:"kind"`
	Title    string  `json:"title"`
	Body     *string `json:"body"`
	Revision int64   `json:"expected_revision"`
	Key      string  `json:"idempotency_key"`
}
type agreeInput struct {
	Revision   int64  `json:"expected_revision"`
	ApprovalID string `json:"approval_request_id"`
	Key        string `json:"idempotency_key"`
}
type failure struct {
	code    int
	message string
}

func (e *failure) Error() string          { return e.message }
func fail(code int, message string) error { return &failure{code, message} }

var uuid = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func respond(w http.ResponseWriter, status int, out any, err error) {
	w.Header().Set("Cache-Control", "no-store")
	if err == nil {
		httpapi.WriteJSON(w, status, out)
		return
	}
	var f *failure
	if errors.As(err, &f) {
		httpapi.WriteError(w, f.code, f.message)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		httpapi.WriteError(w, 404, "project or requirement not found")
		return
	}
	slog.Error("requirements", "err", err)
	httpapi.WriteError(w, 500, "internal")
}
func principal(w http.ResponseWriter, r *http.Request, write bool) (tenant.Principal, bool) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok {
		httpapi.WriteError(w, 401, "authentication required")
		return p, false
	}
	if (p.Kind != tenant.Person && p.Kind != tenant.Agent) || (write && p.Kind != tenant.Person) {
		httpapi.WriteError(w, 403, "person required")
		return p, false
	}
	if !uuid.MatchString(r.PathValue("projectId")) {
		httpapi.WriteError(w, 400, "invalid project id")
		return p, false
	}
	return p, true
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		httpapi.WriteError(w, 400, "invalid request")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		httpapi.WriteError(w, 400, "invalid request")
		return false
	}
	return true
}
func validKey(s string) bool { return len(s) <= 128 && strings.TrimSpace(s) != "" }
func (m *module) list(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r, false)
	if !ok {
		return
	}
	var out []Requirement
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		if _, err := lockProject(r.Context(), tx, r.PathValue("projectId"), false); err != nil {
			return err
		}
		var err error
		out, err = load(r.Context(), tx, r.PathValue("projectId"))
		return err
	})
	respond(w, 200, out, err)
}
func (m *module) create(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r, true)
	if !ok {
		return
	}
	var in createInput
	if !decode(w, r, &in) {
		return
	}
	if (in.Kind != "functional" && in.Kind != "nonfunctional") || strings.TrimSpace(in.Title) == "" || in.Body == nil || in.Revision < 1 || !validKey(in.Key) {
		respond(w, 0, nil, fail(400, "invalid requirement"))
		return
	}
	var out Requirement
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = create(r.Context(), tx, p, r.PathValue("projectId"), in)
		return err
	})
	respond(w, 201, out, err)
}
func (m *module) agree(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r, true)
	if !ok {
		return
	}
	var in agreeInput
	if !decode(w, r, &in) {
		return
	}
	if in.Revision < 1 || !validKey(in.Key) || !uuid.MatchString(in.ApprovalID) {
		respond(w, 0, nil, fail(400, "invalid agreement"))
		return
	}
	var out []Requirement
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = agree(r.Context(), tx, p, r.PathValue("projectId"), in)
		return err
	})
	respond(w, 200, out, err)
}

// Project mutations take the R1 tree lock before row locks to serialize tree
// changes and generation. Digest additionally locks nodes against content edits.
func lockProject(ctx context.Context, tx pgx.Tx, id string, write bool) (int64, error) {
	if write {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_setting('aeon.tenant_id',true),0))`); err != nil {
			return 0, err
		}
	}
	if !write {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(hashtextextended(current_setting('aeon.tenant_id',true),0))`); err != nil {
			return 0, err
		}
	}
	query := `SELECT j.revision FROM journey_projects j JOIN nodes n ON n.tenant_id=j.tenant_id AND n.id=j.project_node_id WHERE j.project_node_id=$1 AND n.deleted_at IS NULL`
	if write {
		query += " FOR UPDATE OF j"
	} else {
		query += " FOR SHARE OF j"
	}
	var rev int64
	err := tx.QueryRow(ctx, query, id).Scan(&rev)
	return rev, err
}
func load(ctx context.Context, tx pgx.Tx, project string) ([]Requirement, error) {
	rows, err := tx.Query(ctx, `SELECT r.requirement_node_id::text,r.project_node_id::text,r.kind,r.revision,r.status,n.title,f.feature_node_id::text,
 ARRAY(SELECT t.ticket_node_id::text FROM journey_tickets t JOIN nodes tn ON tn.tenant_id=t.tenant_id AND tn.id=t.ticket_node_id WHERE t.feature_node_id=f.feature_node_id AND t.source='requirements' AND tn.deleted_at IS NULL ORDER BY t.walker_position,t.ticket_node_id)
 FROM journey_requirements r JOIN nodes n ON n.tenant_id=r.tenant_id AND n.id=r.requirement_node_id
 LEFT JOIN journey_features f ON f.tenant_id=r.tenant_id AND f.requirement_node_id=r.requirement_node_id
 WHERE r.project_node_id=$1 AND n.deleted_at IS NULL ORDER BY r.revision,r.requirement_node_id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Requirement{}
	for rows.Next() {
		var x Requirement
		if err := rows.Scan(&x.NodeID, &x.ProjectID, &x.Kind, &x.Revision, &x.Status, &x.Title, &x.FeatureID, &x.TicketIDs); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
