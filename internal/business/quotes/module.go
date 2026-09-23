// SPDX-License-Identifier: AGPL-3.0-only

package quotes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/plugins"
	"github.com/inspr-at/aeon/internal/plugins/fence"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	pool     *pgxpool.Pool
	registry *plugins.Registry
}

var _ httpapi.Module = (*Module)(nil)
var uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
var shaRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// New exposes the quote httpapi.Module. The coordinator registers
// ManifestPlugin, seals the registry and mounts this module; it owns no shared
// server wiring. Dependencies business_costs and business_crm must be present.
func New(pool *pgxpool.Pool, registry *plugins.Registry) (httpapi.Module, error) {
	if pool == nil || registry == nil {
		return nil, fmt.Errorf("quotes: pool and registry required")
	}
	_, ok := registry.Lookup(PluginID)
	if !ok {
		return nil, fmt.Errorf("quotes: manifest is not registered")
	}
	return &Module{pool: pool, registry: registry}, nil
}
func (m *Module) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/quotes", m.list)
	mux.HandleFunc("POST /api/quotes", m.create)
	mux.HandleFunc("GET /api/quotes/{quoteId}", m.get)
	mux.HandleFunc("GET /api/quotes/{quoteId}/versions", m.versions)
	mux.HandleFunc("POST /api/quotes/{quoteId}/versions", m.freeze)
	mux.HandleFunc("GET /api/quotes/{quoteId}/versions/{version}", m.version)
	mux.HandleFunc("POST /api/quotes/{quoteId}/versions/{version}/issue", m.issue)
	mux.HandleFunc("POST /api/quotes/{quoteId}/versions/{version}/accept", m.accept)
	mux.HandleFunc("GET /api/quotes/{quoteId}/versions/{version}/export", m.export)
}

type failure struct {
	status  int
	message string
}

func (f failure) Error() string { return f.message }
func bad(s string) error        { return failure{400, s} }
func denied() error             { return failure{403, "quote operation is not available"} }
func missing() error            { return failure{404, "quote not found"} }
func conflict(s string) error   { return failure{409, s} }
func respond(w http.ResponseWriter, status int, value any, err error) {
	if err != nil {
		var f failure
		if errors.As(err, &f) {
			httpapi.WriteError(w, f.status, f.message)
		} else {
			httpapi.WriteError(w, 500, "quote operation failed")
		}
		return
	}
	httpapi.WriteJSON(w, status, value)
}
func caller(r *http.Request) (tenant.Principal, error) {
	p, ok := tenant.PrincipalFrom(r.Context())
	if !ok || !uuidRe.MatchString(p.TenantID) || !uuidRe.MatchString(p.ID) {
		return p, denied()
	}
	return p, nil
}
func person(p tenant.Principal) bool { return p.Kind == tenant.Person }
func staff(p tenant.Principal) bool {
	if !person(p) {
		return false
	}
	for _, role := range p.Roles {
		if role == "admin" || role == "member" {
			return true
		}
	}
	return false
}
func admin(p tenant.Principal) bool {
	if !person(p) {
		return false
	}
	for _, role := range p.Roles {
		if role == "admin" {
			return true
		}
	}
	return false
}
func pathID(r *http.Request) (string, error) {
	s := r.PathValue("quoteId")
	if !uuidRe.MatchString(s) {
		return "", bad("invalid quote id")
	}
	return s, nil
}
func pathVersion(r *http.Request) (int, error) {
	n, e := strconv.Atoi(r.PathValue("version"))
	if e != nil || n < 1 {
		return 0, bad("invalid version")
	}
	return n, nil
}
func decode(r *http.Request, v any) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20+1))
	d.DisallowUnknownFields()
	d.UseNumber()
	if err := d.Decode(v); err != nil {
		return bad("invalid JSON body")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return bad("invalid JSON body")
	}
	return nil
}
func (m *Module) tx(ctx context.Context, p tenant.Principal, perm string, write bool, fn func(pgx.Tx) error) error {
	return db.InTenant(ctx, m.pool, p.TenantID, func(tx pgx.Tx) error {
		if err := m.enabled(ctx, tx, p.TenantID, perm, write); err != nil {
			return err
		}
		return fn(tx)
	})
}
func (m *Module) enabled(ctx context.Context, tx pgx.Tx, tenantID, perm string, lock bool) error {
	for _, id := range []string{PluginID, "business_costs", "business_crm"} {
		plug, ok := m.registry.Lookup(id)
		if !ok {
			return denied()
		}
		q := `SELECT enabled,manifest_digest_sha256,permissions FROM plugin_installations WHERE tenant_id=$1::uuid AND plugin_id=$2`
		if lock {
			q += ` FOR SHARE`
		}
		var on bool
		var digest string
		var perms []string
		err := tx.QueryRow(ctx, q, tenantID, id).Scan(&on, &digest, &perms)
		if errors.Is(err, pgx.ErrNoRows) {
			return denied()
		}
		if err != nil {
			return err
		}
		if !on || digest != plug.Manifest.DigestSHA256 {
			return denied()
		}
		if id == PluginID {
			found := false
			for _, p := range perms {
				if p == perm {
					found = true
				}
			}
			if !found {
				return denied()
			}
		}
	}
	return nil
}

type quote struct {
	QuoteNodeID       string `json:"quote_node_id"`
	ProjectNodeID     string `json:"project_node_id"`
	CustomerOrgNodeID string `json:"customer_org_node_id"`
	CurrentVersion    int    `json:"current_version"`
	State             string `json:"state"`
	Revision          int64  `json:"revision"`
}
type createWrite struct {
	Title             string `json:"title"`
	ProjectNodeID     string `json:"project_node_id"`
	CustomerOrgNodeID string `json:"customer_org_node_id"`
}

func readQuote(ctx context.Context, tx pgx.Tx, id string, lock bool) (quote, error) {
	q := `SELECT quote_node_id::text,project_node_id::text,customer_org_node_id::text,current_version,state,revision FROM business_quotes WHERE quote_node_id=$1::uuid`
	if lock {
		q += ` FOR UPDATE`
	}
	var out quote
	err := tx.QueryRow(ctx, q, id).Scan(&out.QuoteNodeID, &out.ProjectNodeID, &out.CustomerOrgNodeID, &out.CurrentVersion, &out.State, &out.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, missing()
	}
	return out, err
}
func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	p, e := caller(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	project := r.URL.Query().Get("project_node_id")
	org := r.URL.Query().Get("customer_org_node_id")
	if (project != "" && !uuidRe.MatchString(project)) || (org != "" && !uuidRe.MatchString(org)) {
		respond(w, 0, nil, bad("invalid filter"))
		return
	}
	out := []quote{}
	e = m.tx(r.Context(), p, fence.PermViewsProvide, false, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT quote_node_id::text,project_node_id::text,customer_org_node_id::text,current_version,state,revision FROM business_quotes WHERE ($1::text='' OR project_node_id=NULLIF($1,'')::uuid) AND ($2::text='' OR customer_org_node_id=NULLIF($2,'')::uuid) ORDER BY created_at DESC,quote_node_id LIMIT 200`, project, org)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var q quote
			if err := rows.Scan(&q.QuoteNodeID, &q.ProjectNodeID, &q.CustomerOrgNodeID, &q.CurrentVersion, &q.State, &q.Revision); err != nil {
				return err
			}
			out = append(out, q)
		}
		return rows.Err()
	})
	respond(w, 200, out, e)
}
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	p, e := caller(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	id, e := pathID(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	var out quote
	e = m.tx(r.Context(), p, fence.PermViewsProvide, false, func(tx pgx.Tx) error { var err error; out, err = readQuote(r.Context(), tx, id, false); return err })
	respond(w, 200, out, e)
}

func (m *Module) create(w http.ResponseWriter, r *http.Request) {
	p, e := caller(r)
	if e != nil {
		respond(w, 0, nil, e)
		return
	}
	if !staff(p) {
		respond(w, 0, nil, denied())
		return
	}
	var in createWrite
	if e = decode(r, &in); e != nil {
		respond(w, 0, nil, e)
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 512 || !uuidRe.MatchString(in.ProjectNodeID) || !uuidRe.MatchString(in.CustomerOrgNodeID) {
		respond(w, 0, nil, bad("invalid quote"))
		return
	}
	var out quote
	e = m.tx(r.Context(), p, fence.PermNodesContribute, true, func(tx pgx.Tx) error {
		var exists bool
		err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM node_relations rel JOIN nodes org ON org.tenant_id=rel.tenant_id AND org.id=rel.source_node_id JOIN node_kinds ok ON ok.tenant_id=org.tenant_id AND ok.id=org.kind_id JOIN nodes prj ON prj.tenant_id=rel.tenant_id AND prj.id=rel.target_node_id JOIN node_kinds pk ON pk.tenant_id=prj.tenant_id AND pk.id=prj.kind_id WHERE rel.type='customer_of' AND rel.source_node_id=$1::uuid AND rel.target_node_id=$2::uuid AND org.deleted_at IS NULL AND prj.deleted_at IS NULL AND ok.slug='organisation' AND pk.slug='project')`, in.CustomerOrgNodeID, in.ProjectNodeID).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return bad("project and customer organisation must be linked")
		}
		err = tx.QueryRow(r.Context(), `INSERT INTO nodes(tenant_id,key,kind_id,title) SELECT $1::uuid,aeon_next_node_key($1::uuid,k.short_prefix),k.id,$2 FROM node_kinds k WHERE k.tenant_id=$1::uuid AND k.slug='quote' RETURNING id::text`, p.TenantID, in.Title).Scan(&out.QuoteNodeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return conflict("quote kind is not configured")
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO business_quotes(tenant_id,quote_node_id,project_node_id,customer_org_node_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid)`, p.TenantID, out.QuoteNodeID, in.ProjectNodeID, in.CustomerOrgNodeID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO node_relations(tenant_id,source_node_id,target_node_id,type) VALUES($1::uuid,$2::uuid,$3::uuid,'customer_of')`, p.TenantID, in.CustomerOrgNodeID, out.QuoteNodeID)
		if err != nil {
			return err
		}
		out, err = readQuote(r.Context(), tx, out.QuoteNodeID, false)
		if err != nil {
			return err
		}
		return appendEvent(r.Context(), tx, p, out.QuoteNodeID, "quote.created", nil, out)
	})
	respond(w, 201, out, e)
}
