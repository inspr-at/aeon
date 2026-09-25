// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/httpapi"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrForbidden = errors.New("permission denied")
var ErrNoStore = errors.New("authorization store missing")

type Scope struct{ ProjectID string }
type RoleRef struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}
type Grant struct {
	Role        *RoleRef `json:"role"`
	Permissions []string `json:"permissions"`
}
type ProjectGrant struct {
	ID          string   `json:"id"`
	Role        *RoleRef `json:"role"`
	Permissions []string `json:"permissions"`
}
type Effective struct {
	Workspace Grant         `json:"workspace"`
	Project   *ProjectGrant `json:"project"`
}

type poolKey struct{}

func BindPool(ctx context.Context, pool *pgxpool.Pool) context.Context {
	return context.WithValue(ctx, poolKey{}, pool)
}

// Require denies unknown permissions, inactive principals, missing bindings,
// and agent keys whose scopes do not include the requested permission.
func Require(ctx context.Context, permission string, scope Scope) error {
	if _, ok := Lookup(permission); !ok {
		return ErrForbidden
	}
	p, ok := tenant.PrincipalFrom(ctx)
	if !ok {
		return ErrForbidden
	}
	pool, ok := ctx.Value(poolKey{}).(*pgxpool.Pool)
	if !ok || pool == nil {
		return ErrNoStore
	}
	effective, err := Load(ctx, pool, p, scope.ProjectID)
	if err != nil {
		return err
	}
	return permitEffective(p, permission, effective)
}

func requireTx(ctx context.Context, tx pgx.Tx, p tenant.Principal, permission string, scope Scope) error {
	if _, ok := Lookup(permission); !ok {
		return ErrForbidden
	}
	effective, err := loadTx(ctx, tx, p, scope.ProjectID)
	if err != nil {
		return err
	}
	return permitEffective(p, permission, effective)
}

// RequireTx makes a decision inside an existing db.InTenant transaction. It
// is used by handlers whose resource lock and access check must be atomic.
func RequireTx(ctx context.Context, tx pgx.Tx, p tenant.Principal, permission string, scope Scope) error {
	return requireTx(ctx, tx, p, permission, scope)
}

func permitEffective(p tenant.Principal, permission string, effective Effective) error {
	allowed := contains(effective.Workspace.Permissions, permission)
	if effective.Project != nil {
		allowed = allowed || contains(effective.Project.Permissions, permission)
	}
	if !allowed {
		return ErrForbidden
	}
	if p.Kind == tenant.Agent && !containsScope(p.Scopes, permission) {
		return ErrForbidden
	}
	return nil
}

func contains(items []string, want string) bool {
	for _, v := range items {
		if v == want {
			return true
		}
	}
	return false
}
func containsScope(items []string, want string) bool {
	for _, v := range items {
		if strings.ReplaceAll(v, ":", ".") == want {
			return true
		}
	}
	return false
}

// Load evaluates bindings inside one tenant transaction. Project support is
// read-ready for P2; P1 writes only workspace bindings.
func Load(ctx context.Context, pool *pgxpool.Pool, p tenant.Principal, projectID string) (Effective, error) {
	var result Effective
	err := db.InTenant(ctx, pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		result, err = loadTx(ctx, tx, p, projectID)
		return err
	})
	return result, err
}

// loadTx lets access mutations recheck grants in the same tenant transaction
// that writes the binding. It also works when the pool has one connection.
func loadTx(ctx context.Context, tx pgx.Tx, p tenant.Principal, projectID string) (Effective, error) {
	result := Effective{Workspace: Grant{Permissions: []string{}}}
	if projectID != "" {
		result.Project = &ProjectGrant{ID: projectID, Permissions: []string{}}
	}
	var status, kind string
	if err := tx.QueryRow(ctx, `SELECT status,kind FROM principals WHERE tenant_id=$1::uuid AND id=$2::uuid`, p.TenantID, p.ID).Scan(&status, &kind); err != nil {
		return Effective{}, err
	}
	if status != "active" || kind != string(p.Kind) {
		return Effective{}, ErrForbidden
	}
	rows, err := tx.Query(ctx, `SELECT b.scope_type,coalesce(b.scope_id::text,''),r.id::text,r.key,r.name,r.builtin,rp.permission
          FROM role_bindings b JOIN roles r ON r.tenant_id=b.tenant_id AND r.id=b.role_id
          LEFT JOIN role_permissions rp ON rp.tenant_id=r.tenant_id AND rp.role_id=r.id
          WHERE b.tenant_id=$1::uuid AND b.principal_id=$2::uuid
            AND (b.scope_type='workspace' OR b.scope_type='project' AND b.scope_id=$3::uuid)
          ORDER BY b.scope_type,rp.permission`, p.TenantID, p.ID, nullUUID(projectID))
	if err != nil {
		return Effective{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var scopeType, scopeID, id, key, name string
		var builtin bool
		var perm *string
		if err := rows.Scan(&scopeType, &scopeID, &id, &key, &name, &builtin, &perm); err != nil {
			return Effective{}, err
		}
		ref := &RoleRef{ID: id, Key: key, Name: name}
		var target *Grant
		if scopeType == "workspace" {
			target = &result.Workspace
		} else if result.Project != nil && scopeID == projectID {
			result.Project.Role = ref
		}
		if target != nil {
			target.Role = ref
		}
		perms := []string{}
		if builtin {
			perms = builtinPermissions(key)
		} else if perm != nil {
			perms = []string{*perm}
		}
		if target != nil {
			target.Permissions = append(target.Permissions, perms...)
		} else if result.Project != nil && scopeID == projectID {
			result.Project.Permissions = append(result.Project.Permissions, perms...)
		}
	}
	if err := rows.Err(); err != nil {
		return Effective{}, err
	}
	result.Workspace.Permissions = unique(result.Workspace.Permissions)
	if result.Project != nil {
		result.Project.Permissions = unique(append(result.Project.Permissions, result.Workspace.Permissions...))
	}
	if p.Kind == tenant.Agent && p.KeyCreatorID != "" {
		creator := tenant.Principal{ID: p.KeyCreatorID, TenantID: p.TenantID, Kind: tenant.Person}
		ceiling, err := loadTx(ctx, tx, creator, projectID)
		if err != nil {
			return Effective{}, err
		}
		result.Workspace.Permissions = intersect(result.Workspace.Permissions, ceiling.Workspace.Permissions)
		if result.Project != nil && ceiling.Project != nil {
			result.Project.Permissions = intersect(result.Project.Permissions, ceiling.Project.Permissions)
		}
	}
	return result, nil
}

func intersect(grants, ceiling []string) []string {
	out := make([]string, 0, len(grants))
	for _, key := range grants {
		if contains(ceiling, key) {
			out = append(out, key)
		}
	}
	return out
}

// EffectiveTx is for an existing db.InTenant transaction, chiefly key issuance.
// It keeps the grant check and key insert on one connection and snapshot.
func EffectiveTx(ctx context.Context, tx pgx.Tx, p tenant.Principal, projectID string) (Effective, error) {
	return loadTx(ctx, tx, p, projectID)
}

func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func unique(in []string) []string {
	sort.Strings(in)
	out := in[:0]
	for _, v := range in {
		if len(out) == 0 || out[len(out)-1] != v {
			out = append(out, v)
		}
	}
	return out
}

// Handle records a permission at registration and enforces it before the
// handler. A nil permission is never accepted for a protected route.
func Handle(mux *http.ServeMux, pool *pgxpool.Pool, pattern, permission string, handler http.HandlerFunc) {
	if _, ok := Lookup(permission); !ok {
		panic("undeclared permission: " + permission)
	}
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		scope := Scope{ProjectID: r.PathValue("projectId")}
		ctx := BindPool(r.Context(), pool)
		if err := Require(ctx, permission, scope); err != nil {
			httpapi.WriteJSON(w, http.StatusForbidden, map[string]any{"error": "permission denied", "code": "forbidden", "reason": err.Error()})
			return
		}
		handler(w, r.WithContext(ctx))
	})
}
