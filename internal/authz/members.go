// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/events"
	"github.com/jackc/pgx/v5"
)

type Alias struct {
	PrincipalID string `json:"principal_id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
}
type Member struct {
	PrincipalID   string     `json:"principal_id"`
	Name          string     `json:"name"`
	AvatarURL     *string    `json:"avatar_url"`
	Email         *string    `json:"email"`
	Status        string     `json:"status"`
	Identity      *string    `json:"identity"`
	WorkspaceRole *RoleRef   `json:"workspace_role"`
	ProjectRoles  []any      `json:"project_roles"`
	Aliases       []Alias    `json:"aliases"`
	ClassicRole   *string    `json:"classic_role"`
	LastActiveAt  *time.Time `json:"last_active_at"`
}
type AgentMember struct {
	PrincipalID   string     `json:"principal_id"`
	Name          string     `json:"name"`
	WorkspaceRole *RoleRef   `json:"workspace_role"`
	KeyCount      int        `json:"key_count"`
	LastSeenAt    *time.Time `json:"last_seen_at"`
	Service       bool       `json:"service"`
}
type ImportedMember struct {
	PrincipalID string  `json:"principal_id"`
	Name        string  `json:"name"`
	ClassicRole *string `json:"classic_role"`
}
type MemberDirectory struct {
	People   []Member         `json:"people"`
	Agents   []AgentMember    `json:"agents"`
	Invites  []any            `json:"invites"`
	Imported []ImportedMember `json:"imported"`
}

func (m *Module) members(w http.ResponseWriter, r *http.Request) {
	p := actor(r)
	out := MemberDirectory{People: []Member{}, Agents: []AgentMember{}, Invites: []any{}, Imported: []ImportedMember{}}
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT p.id::text,p.kind,p.name,p.email,p.status,p.roles,
          i.issuer,coalesce(pp.avatar_original_hash,''),br.id::text,br.key,br.name,
          (SELECT max(s.last_seen_at) FROM sessions s WHERE s.tenant_id=p.tenant_id AND s.principal_id=p.id),
          (SELECT count(*) FROM agent_keys k WHERE k.tenant_id=p.tenant_id AND k.principal_id=p.id AND k.revoked_at IS NULL),
          (SELECT max(k.last_used_at) FROM agent_keys k WHERE k.tenant_id=p.tenant_id AND k.principal_id=p.id)
          FROM principals p
          LEFT JOIN identities i ON i.id=p.identity_id
          LEFT JOIN personal_profiles pp ON pp.tenant_id=p.tenant_id AND pp.principal_id=p.id
          LEFT JOIN role_bindings b ON b.tenant_id=p.tenant_id AND b.principal_id=p.id AND b.scope_type='workspace'
          LEFT JOIN roles br ON br.tenant_id=b.tenant_id AND br.id=b.role_id
          WHERE p.tenant_id=$1::uuid ORDER BY p.kind DESC,p.name,p.id`, p.TenantID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, kind, name, status, avatarHash string
			var email, issuer, roleID, roleKey, roleName *string
			var legacy []string
			var lastActive, lastSeen *time.Time
			var keyCount int
			if err := rows.Scan(&id, &kind, &name, &email, &status, &legacy, &issuer, &avatarHash, &roleID, &roleKey, &roleName, &lastActive, &keyCount, &lastSeen); err != nil {
				rows.Close()
				return err
			}
			var role *RoleRef
			if roleID != nil {
				role = &RoleRef{ID: *roleID, Key: *roleKey, Name: *roleName}
			}
			if kind == "agent" {
				service := false
				for _, v := range legacy {
					if v == "system" || v == "importer" || v == "operator" || v == "embedding" || strings.HasPrefix(v, "quote_") {
						service = true
					}
				}
				out.Agents = append(out.Agents, AgentMember{PrincipalID: id, Name: name, WorkspaceRole: role, KeyCount: keyCount, LastSeenAt: lastSeen, Service: service})
				continue
			}
			var classic *string
			if len(legacy) > 0 {
				v := legacy[0]
				classic = &v
			}
			var identity *string
			if issuer != nil && *issuer != "paimos-classic" {
				v := "inspr_id"
				identity = &v
			}
			var avatar *string
			if avatarHash != "" {
				v := "/api/people/" + id + "/avatar"
				avatar = &v
			}
			item := Member{PrincipalID: id, Name: name, AvatarURL: avatar, Email: email, Status: status, Identity: identity, WorkspaceRole: role, ProjectRoles: []any{}, Aliases: []Alias{}, ClassicRole: classic, LastActiveAt: lastActive}
			out.People = append(out.People, item)
			if issuer != nil && *issuer == "paimos-classic" {
				out.Imported = append(out.Imported, ImportedMember{PrincipalID: id, Name: name, ClassicRole: classic})
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		aliases, err := tx.Query(r.Context(), `SELECT linked_to::text,id::text,name FROM principals WHERE tenant_id=$1::uuid AND linked_to IS NOT NULL ORDER BY name,id`, p.TenantID)
		if err != nil {
			return err
		}
		for aliases.Next() {
			var target, source, name string
			if err := aliases.Scan(&target, &source, &name); err != nil {
				aliases.Close()
				return err
			}
			for i := range out.People {
				if out.People[i].PrincipalID == target {
					out.People[i].Aliases = append(out.People[i].Aliases, Alias{PrincipalID: source, Name: name, Source: "classic"})
					break
				}
			}
		}
		err = aliases.Err()
		aliases.Close()
		return err
	})
	if err != nil {
		internalFail(w, err)
		return
	}
	reply(w, 200, out)
}

func (m *Module) putWorkspaceRole(w http.ResponseWriter, r *http.Request) {
	p := actor(r)
	id := r.PathValue("principal_id")
	if !uuidPattern.MatchString(id) {
		apiFail(w, 400, "invalid", "principal_id", "Principal ID must be a UUID")
		return
	}
	var raw map[string]json.RawMessage
	if err := decode(r, &raw); err != nil {
		apiFail(w, 400, "invalid", "body", err.Error())
		return
	}
	value, ok := raw["role_id"]
	if !ok || len(raw) != 1 {
		apiFail(w, 400, "invalid", "role_id", "Supply only role_id")
		return
	}
	var roleID *string
	if string(value) != "null" {
		var idValue string
		if err := json.Unmarshal(value, &idValue); err != nil || !uuidPattern.MatchString(idValue) {
			apiFail(w, 400, "invalid", "role_id", "Role ID must be a UUID or null")
			return
		}
		roleID = &idValue
	}
	err := db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		if err := m.authorizeMutation(r.Context(), tx, p, "members.manage", nil); err != nil {
			return err
		}
		if roleID != nil {
			targetRole, err := roleTx(r.Context(), tx, *roleID)
			if err != nil {
				return err
			}
			if err := m.canGrant(r.Context(), p, targetRole.Permissions); err != nil {
				return err
			}
			if targetRole.Key == "owner" {
				if err := Require(BindPool(r.Context(), m.pool), "ownership.transfer", Scope{}); err != nil {
					return err
				}
			}
		}
		var kind, status string
		var legacy []string
		if err := tx.QueryRow(r.Context(), `SELECT kind,status,roles FROM principals WHERE tenant_id=$1::uuid AND id=$2::uuid FOR UPDATE`, p.TenantID, id).Scan(&kind, &status, &legacy); err != nil {
			return err
		}
		if status != "active" {
			return ErrForbidden
		}
		if kind == "agent" {
			for _, v := range legacy {
				if v == "system" || v == "importer" || v == "operator" || v == "embedding" || strings.HasPrefix(v, "quote_") {
					return ErrForbidden
				}
			}
		}
		var priorID *string
		var priorKey *string
		err := tx.QueryRow(r.Context(), `SELECT r.id::text,r.key FROM role_bindings b JOIN roles r ON r.tenant_id=b.tenant_id AND r.id=b.role_id WHERE b.tenant_id=$1::uuid AND b.principal_id=$2::uuid AND b.scope_type='workspace' FOR UPDATE OF b`, p.TenantID, id).Scan(&priorID, &priorKey)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if priorKey != nil && *priorKey == "owner" {
			if err := Require(BindPool(r.Context(), m.pool), "ownership.transfer", Scope{}); err != nil {
				return err
			}
		}
		if priorID != nil && roleID != nil && *priorID == *roleID {
			return nil
		}
		if roleID == nil {
			if _, err := tx.Exec(r.Context(), `DELETE FROM role_bindings WHERE tenant_id=$1::uuid AND principal_id=$2::uuid AND scope_type='workspace'`, p.TenantID, id); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(r.Context(), `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type) VALUES($1::uuid,$2::uuid,$3::uuid,'workspace') ON CONFLICT (tenant_id,principal_id) WHERE scope_type='workspace' DO UPDATE SET role_id=EXCLUDED.role_id`, p.TenantID, id, *roleID); err != nil {
				return err
			}
		}
		_, err = events.Append(r.Context(), tx, p, events.Change{Type: "authz.workspace_role_changed", Before: map[string]any{"principal_id": id, "role_id": priorID}, After: map[string]any{"principal_id": id, "role_id": roleID}})
		return err
	})
	if err != nil {
		internalFail(w, err)
		return
	}
	// The directory shape is shared with GET /members. Return the changed row.
	result := Member{}
	err = db.InTenant(r.Context(), m.pool, p.TenantID, func(tx pgx.Tx) error {
		var legacy []string
		var roleID, roleKey, roleName, issuer *string
		var avatarHash string
		if err := tx.QueryRow(r.Context(), `SELECT p.id::text,p.name,p.email,p.status,p.roles,r.id::text,r.key,r.name,
		  i.issuer,coalesce(pp.avatar_original_hash,''),
		  (SELECT max(s.last_seen_at) FROM sessions s WHERE s.tenant_id=p.tenant_id AND s.principal_id=p.id)
          FROM principals p LEFT JOIN role_bindings b ON b.tenant_id=p.tenant_id AND b.principal_id=p.id AND b.scope_type='workspace'
		  LEFT JOIN roles r ON r.tenant_id=b.tenant_id AND r.id=b.role_id
		  LEFT JOIN identities i ON i.id=p.identity_id
		  LEFT JOIN personal_profiles pp ON pp.tenant_id=p.tenant_id AND pp.principal_id=p.id
		  WHERE p.tenant_id=$1::uuid AND p.id=$2::uuid`, p.TenantID, id).Scan(&result.PrincipalID, &result.Name, &result.Email, &result.Status, &legacy, &roleID, &roleKey, &roleName, &issuer, &avatarHash, &result.LastActiveAt); err != nil {
			return err
		}
		if roleID != nil {
			result.WorkspaceRole = &RoleRef{ID: *roleID, Key: *roleKey, Name: *roleName}
		}
		if len(legacy) > 0 {
			v := legacy[0]
			result.ClassicRole = &v
		}
		if issuer != nil && *issuer != "paimos-classic" {
			v := "inspr_id"
			result.Identity = &v
		}
		if avatarHash != "" {
			v := "/api/people/" + id + "/avatar"
			result.AvatarURL = &v
		}
		result.ProjectRoles = []any{}
		result.Aliases = []Alias{}
		rows, err := tx.Query(r.Context(), `SELECT id::text,name FROM principals WHERE tenant_id=$1::uuid AND linked_to=$2::uuid ORDER BY name,id`, p.TenantID, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var source, name string
			if err := rows.Scan(&source, &name); err != nil {
				return err
			}
			result.Aliases = append(result.Aliases, Alias{PrincipalID: source, Name: name, Source: "classic"})
		}
		return rows.Err()
	})
	if err != nil {
		internalFail(w, err)
		return
	}
	reply(w, 200, result)
}
