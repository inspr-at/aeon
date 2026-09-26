// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func ownerCount(ctx context.Context, tx pgx.Tx, tenantID string) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM role_bindings b
		JOIN principals p ON p.tenant_id=b.tenant_id AND p.id=b.principal_id
		JOIN roles r ON r.tenant_id=b.tenant_id AND r.id=b.role_id
		WHERE b.tenant_id=$1::uuid AND b.scope_type='workspace' AND r.key='owner'
		  AND p.status='active' AND p.linked_to IS NULL`, tenantID).Scan(&n)
	return n, err
}

func applyOwnerFlags(people []Member, count int) {
	for i := range people {
		role := people[i].WorkspaceRole
		people[i].LastOwner = people[i].Status == "active" && role != nil && role.Key == "owner" && count == 1
	}
}

func attachProjectRoles(ctx context.Context, tx pgx.Tx, tenantID string, people []Member) error {
	rows, err := tx.Query(ctx, `SELECT b.principal_id::text, n.id::text, n.key, n.title, r.id::text, r.key, r.name
		FROM role_bindings b
		JOIN nodes n ON n.tenant_id=b.tenant_id AND n.id=b.scope_id
		JOIN roles r ON r.tenant_id=b.tenant_id AND r.id=b.role_id
		WHERE b.tenant_id=$1::uuid AND b.scope_type='project'
		ORDER BY n.key, n.id, r.key`, tenantID)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := map[string]*Member{}
	for i := range people {
		byID[people[i].PrincipalID] = &people[i]
	}
	for rows.Next() {
		var principal string
		var role ProjectRole
		if err := rows.Scan(&principal, &role.ProjectID, &role.ProjectKey, &role.ProjectTitle, &role.Role.ID, &role.Role.Key, &role.Role.Name); err != nil {
			return err
		}
		if person := byID[principal]; person != nil {
			person.ProjectRoles = append(person.ProjectRoles, role)
		}
	}
	return rows.Err()
}

func readMember(ctx context.Context, tx pgx.Tx, tenantID, id string) (Member, error) {
	var result Member
	var legacy []string
	var roleID, roleKey, roleName, issuer *string
	var avatarHash string
	err := tx.QueryRow(ctx, `SELECT p.id::text,p.name,p.email,p.status,p.roles,r.id::text,r.key,r.name,
		i.issuer,coalesce(pp.avatar_original_hash,''),
		EXISTS (SELECT 1 FROM personal_profiles avatar WHERE avatar.tenant_id=p.tenant_id AND avatar.principal_id=p.id AND avatar.avatar_hashes <> '{}'::jsonb),
		(SELECT max(s.last_seen_at) FROM sessions s WHERE s.tenant_id=p.tenant_id AND s.principal_id=p.id)
		FROM principals p
		LEFT JOIN role_bindings b ON b.tenant_id=p.tenant_id AND b.principal_id=p.id AND b.scope_type='workspace'
		LEFT JOIN roles r ON r.tenant_id=b.tenant_id AND r.id=b.role_id
		LEFT JOIN identities i ON i.id=p.identity_id
		LEFT JOIN personal_profiles pp ON pp.tenant_id=p.tenant_id AND pp.principal_id=p.id
		WHERE p.tenant_id=$1::uuid AND p.id=$2::uuid`, tenantID, id).Scan(
		&result.PrincipalID, &result.Name, &result.Email, &result.Status, &legacy, &roleID, &roleKey, &roleName, &issuer, &avatarHash, &result.HasAvatar, &result.LastActiveAt)
	if err != nil {
		return Member{}, err
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
	result.ProjectRoles = []ProjectRole{}
	result.Aliases = []Alias{}
	rows, err := tx.Query(ctx, `SELECT id::text,name FROM principals WHERE tenant_id=$1::uuid AND linked_to=$2::uuid ORDER BY name,id`, tenantID, id)
	if err != nil {
		return Member{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var source, name string
		if err := rows.Scan(&source, &name); err != nil {
			return Member{}, err
		}
		result.Aliases = append(result.Aliases, Alias{PrincipalID: source, Name: name, Source: "classic"})
	}
	if err := rows.Err(); err != nil {
		return Member{}, err
	}
	people := []Member{result}
	if err := attachProjectRoles(ctx, tx, tenantID, people); err != nil {
		return Member{}, err
	}
	count, err := ownerCount(ctx, tx, tenantID)
	if err != nil {
		return Member{}, err
	}
	applyOwnerFlags(people, count)
	return people[0], nil
}
