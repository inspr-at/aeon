// SPDX-License-Identifier: AGPL-3.0-only

// Package auth is the OIDC session and agent-key boundary for PAIMOS AEON.
package auth

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/tenant"
)

// Module serves /api/auth, /api/me and /api/agent-keys, and resolves the caller.
type Module struct {
	cfg      Config
	pool     *pgxpool.Pool
	inTenant func(context.Context, *pgxpool.Pool, string, func(pgx.Tx) error) error

	mu       sync.Mutex
	provider *oidc.Provider
	oauth    oauth2.Config
}

type credKind int

const (
	credNone credKind = iota
	credSession
	credAgent
	credStale
)

// New validates cfg and binds the module to pool. Tenant-scoped queries go
// through db.InTenant. The result implements httpapi.Module; register Middleware
// alongside it (or use Attach). Auth is a core boundary, not a plugin.
func New(cfg Config, pool *pgxpool.Pool) (*Module, error) {
	if len(cfg.SessionKey) < minSessionKey {
		return nil, errShortKey
	}
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")
	if cfg.BootstrapTenantSlug == "" {
		cfg.BootstrapTenantSlug = defaultTenantSlug
	}
	return &Module{cfg: cfg, pool: pool, inTenant: db.InTenant}, nil
}

// Attach registers the module and its middleware on srv. The server calls Mount.
func Attach(srv *httpapi.Server, cfg Config) (*Module, error) {
	m, err := New(cfg, srv.Pool)
	if err != nil {
		return nil, err
	}
	srv.Modules = append(srv.Modules, m)
	srv.Middleware = append(srv.Middleware, m.Middleware)
	return m, nil
}

// Mount adds the auth and agent-key routes. POST /api/auth/dev-login is
// registered only when AEON_ENV=dev.
func (m *Module) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/login", m.handleLogin)
	mux.HandleFunc("GET /api/auth/callback", m.handleCallback)
	mux.HandleFunc("POST /api/auth/logout", m.handleLogout)
	mux.HandleFunc("GET /api/me", m.handleMe)
	mux.HandleFunc("POST /api/agent-keys", m.handleCreateAgentKey)
	mux.HandleFunc("GET /api/agent-keys", m.handleListAgentKeys)
	mux.HandleFunc("DELETE /api/agent-keys/{id}", m.handleRevokeAgentKey)
	if m.cfg.Dev() {
		mux.HandleFunc("POST /api/auth/dev-login", m.handleDevLogin)
	}
}

// Middleware resolves a session cookie or an agent bearer token onto the
// request context. Unauthenticated /api requests, other than health, version
// and /api/auth/* and /api/public/quotes/*, get 401 JSON.
func (m *Module) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, kind, err := m.authenticate(r)
		if err != nil && !isPublicAPI(r.URL.Path) {
			writeInternal(w)
			return
		}
		if err != nil {
			kind = credNone
		}
		switch kind {
		case credStale:
			if r.URL.Path != "/api/auth/logout" {
				m.clearSessionCookie(w)
			}
		case credSession:
			if r.URL.Path != "/api/auth/logout" {
				if c, cErr := r.Cookie(sessionCookieName); cErr == nil {
					m.setSessionCookie(w, c.Value)
				}
			}
			r = r.WithContext(tenant.WithPrincipal(r.Context(), p))
		case credAgent:
			r = r.WithContext(tenant.WithPrincipal(r.Context(), p))
		}
		if kind != credSession && kind != credAgent && isProtectedAPI(r.URL.Path) {
			if r.URL.Path == "/api/me" {
				m.writeMeUnauthorized(w)
			} else {
				writeUnauthorized(w)
			}
			return
		}
		if kind == credSession && p.Kind == tenant.Person && !slices.Contains(p.Roles, "admin") && !slices.Contains(p.Roles, "member") && !customerRouteAllowed(r, p) {
			httpapi.WriteError(w, http.StatusForbidden, "customer access denied")
			return
		}
		if kind == credAgent {
			if scope, controlled := coreAgentScope(r); controlled && (scope == "" || !slices.Contains(p.Scopes, scope)) {
				httpapi.WriteError(w, http.StatusForbidden, "agent key scope required")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func validRouteUUID(s string) bool {
	var id pgtype.UUID
	return len(s) == 36 && id.Scan(s) == nil && id.Valid
}

// Customer sessions may access only their own profile and quote handlers that
// independently verify the contact binding and frozen recipient digest.
func customerRouteAllowed(r *http.Request, p tenant.Principal) bool {
	if isPublicAPI(r.URL.Path) || r.URL.Path == "/api/me" && r.Method == http.MethodGet {
		return true
	}
	if r.URL.Path == "/api/me/greeting" && r.Method == http.MethodGet || r.URL.Path == "/api/me/profile" && (r.Method == http.MethodGet || r.Method == http.MethodPatch) || r.URL.Path == "/api/me/avatar" && (r.Method == http.MethodPost || r.Method == http.MethodDelete) {
		return true
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 5 && parts[0] == "api" && parts[1] == "people" && parts[2] == p.ID && parts[3] == "avatar" && r.Method == http.MethodGet {
		return true
	}
	if len(parts) < 3 || parts[0] != "api" || parts[1] != "quotes" || !validRouteUUID(parts[2]) {
		return false
	}
	if len(parts) == 3 {
		return r.Method == http.MethodGet
	}
	if len(parts) < 5 || parts[3] != "versions" {
		return false
	}
	version, err := strconv.Atoi(parts[4])
	if err != nil || version < 1 {
		return false
	}
	if len(parts) == 5 {
		return r.Method == http.MethodGet
	}
	return len(parts) == 6 && (parts[5] == "accept" && r.Method == http.MethodPost || parts[5] == "export" && r.Method == http.MethodGet)
}

// General work APIs predate module-local agent grants. Apply key scopes before
// they reach those handlers; specialised agent modules keep their own checks.
func coreAgentScope(r *http.Request) (string, bool) {
	path := r.URL.Path
	read := r.Method == http.MethodGet || r.Method == http.MethodHead
	scope := func(resource string) (string, bool) {
		if read {
			return resource + ":read", true
		}
		return resource + ":write", true
	}
	switch {
	case path == "/api/projects", path == "/api/nodes", strings.HasPrefix(path, "/api/nodes/"), strings.HasPrefix(path, "/api/node-keys/"):
		return scope("nodes")
	case path == "/api/kinds", strings.HasPrefix(path, "/api/kinds/"):
		if !read {
			return "nodes:configure", true
		}
		return "nodes:read", true
	case path == "/api/relations", strings.HasPrefix(path, "/api/relations/"):
		return scope("relations")
	case path == "/api/events", strings.HasPrefix(path, "/api/events/"):
		if read {
			return "events:read", true
		}
		return "events:undo", true
	case path == "/api/search":
		return "search:read", true
	case path == "/api/views", strings.HasPrefix(path, "/api/views/"), strings.HasPrefix(path, "/api/preferences/"),
		path == "/api/project-groups", strings.HasPrefix(path, "/api/project-groups/"):
		return scope("views")
	default:
		return "", false
	}
}

func isPublicAPI(path string) bool {
	switch path {
	case "/api/health", "/api/version":
		return true
	default:
		return strings.HasPrefix(path, "/api/auth/") || strings.HasPrefix(path, "/api/public/quotes/")
	}
}

func isProtectedAPI(path string) bool {
	return strings.HasPrefix(path, "/api/") && !isPublicAPI(path)
}

func (m *Module) authenticate(r *http.Request) (tenant.Principal, credKind, error) {
	if h := strings.TrimSpace(r.Header.Get("Authorization")); h != "" {
		prefix, secret, ok := parseBearer(h)
		if !ok {
			return tenant.Principal{}, credNone, nil
		}
		p, ok, err := m.authenticateAgent(r.Context(), prefix, secret)
		if err != nil {
			return tenant.Principal{}, credNone, err
		}
		if !ok {
			return tenant.Principal{}, credNone, nil
		}
		return p, credAgent, nil
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return tenant.Principal{}, credNone, nil
	}
	raw, err := decodeSessionToken(c.Value)
	if err != nil {
		return tenant.Principal{}, credStale, nil
	}
	p, ok, err := m.authenticateSession(r.Context(), raw)
	if err != nil {
		return tenant.Principal{}, credNone, err
	}
	if !ok {
		return tenant.Principal{}, credStale, nil
	}
	return p, credSession, nil
}

func parseBearer(h string) (prefix, secret string, ok bool) {
	scheme, token, found := strings.Cut(h, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", "", false
	}
	token = strings.TrimSpace(token)
	rest, ok := strings.CutPrefix(token, "aeon_")
	if !ok {
		return "", "", false
	}
	prefix, secret, ok = strings.Cut(rest, "_")
	if !ok || prefix == "" || secret == "" {
		return "", "", false
	}
	return prefix, secret, true
}

func (m *Module) oidcProvider(ctx context.Context) (*oidc.Provider, oauth2.Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.provider != nil {
		return m.provider, m.oauth, nil
	}
	if m.cfg.OIDCIssuer == "" || m.cfg.OIDCClientID == "" || m.cfg.PublicURL == "" {
		return nil, oauth2.Config{}, errOIDCNotConfigured
	}
	p, err := oidc.NewProvider(ctx, m.cfg.OIDCIssuer)
	if err != nil {
		return nil, oauth2.Config{}, err
	}
	ep := p.Endpoint()
	// Public client: client_id in the body, no client secret.
	ep.AuthStyle = oauth2.AuthStyleInParams
	oc := oauth2.Config{
		ClientID:    m.cfg.OIDCClientID,
		RedirectURL: m.cfg.PublicURL + "/api/auth/callback",
		Endpoint:    ep,
		Scopes:      []string{oidc.ScopeOpenID, "profile", "email"},
	}
	m.provider = p
	m.oauth = oc
	return p, oc, nil
}
