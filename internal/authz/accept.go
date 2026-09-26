// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"

	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrNoInvite means no pending, unexpired invite matches this verified email.
// A token, when present, must match too: a link never accepts a different address.
var ErrNoInvite = errors.New("no matching invite")

// AcceptInvite enrolls a person for a verified email inside the caller's
// tenant transaction: the person, the invited bindings and the acceptance
// event commit together. The caller must refuse an unverified email before
// calling. An empty token matches the newest pending invite for the email.
func AcceptInvite(ctx context.Context, tx pgx.Tx, tenantID, identityID, email, name, token string) (tenant.Principal, error) {
	email = strings.TrimSpace(email)
	if !emailPattern.MatchString(email) {
		return tenant.Principal{}, ErrNoInvite
	}
	var hash any
	if token != "" {
		sum := sha256.Sum256([]byte(token))
		hash = sum[:]
	}
	var inviteID, createdBy string
	var workspaceRole *string
	err := tx.QueryRow(ctx, `SELECT i.id::text, i.workspace_role_id::text, i.created_by::text
		FROM invites i
		WHERE i.tenant_id=$1::uuid
		  AND lower(i.email)=lower($2)
		  AND i.accepted_at IS NULL AND i.revoked_at IS NULL AND i.expires_at > now()
		  AND ($3::bytea IS NULL OR i.token_hash=$3::bytea)
		ORDER BY i.created_at DESC, i.id
		LIMIT 1
		FOR UPDATE`, tenantID, email, hash).Scan(&inviteID, &workspaceRole, &createdBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenant.Principal{}, ErrNoInvite
	}
	if err != nil {
		return tenant.Principal{}, err
	}
	display := strings.TrimSpace(name)
	if display == "" {
		display = email
	}
	if len(display) > 200 {
		display = display[:200]
	}
	inviter := tenant.Principal{ID: createdBy, TenantID: tenantID, Kind: tenant.Person}
	grants, err := inviteProjectGrants(ctx, tx, tenantID, inviteID)
	if err != nil {
		return tenant.Principal{}, err
	}
	// The inviter's authority is checked again at acceptance, as at creation: an
	// invite from someone since demoted or deactivated grants nothing.
	if err := requireTx(ctx, tx, inviter, "members.manage", Scope{}); err != nil {
		return tenant.Principal{}, ErrNoInvite
	}
	if workspaceRole != nil {
		if _, err := grantRole(ctx, tx, inviter, *workspaceRole, "workspace_role_id"); err != nil {
			return tenant.Principal{}, ErrNoInvite
		}
	}
	for _, g := range grants {
		if _, err := grantRole(ctx, tx, inviter, g.role, "project_roles"); err != nil {
			return tenant.Principal{}, ErrNoInvite
		}
	}
	var person tenant.Principal
	person, err = scanNewPerson(ctx, tx, tenantID, identityID, display, email)
	if err != nil {
		return tenant.Principal{}, err
	}
	if workspaceRole != nil {
		if err := bindScope(ctx, tx, inviter, person.ID, *workspaceRole, "workspace", ""); err != nil {
			return tenant.Principal{}, err
		}
	}
	for _, g := range grants {
		var live bool
		err := tx.QueryRow(ctx, `SELECT deleted_at IS NULL FROM nodes WHERE tenant_id=$1::uuid AND id=$2::uuid`, tenantID, g.project).Scan(&live)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !live {
			continue
		}
		if err != nil {
			return tenant.Principal{}, err
		}
		if err := bindScope(ctx, tx, inviter, person.ID, g.role, "project", g.project); err != nil {
			return tenant.Principal{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE invites SET accepted_at=now(), accepted_by=$3::uuid
		WHERE tenant_id=$1::uuid AND id=$2::uuid AND accepted_at IS NULL`, tenantID, inviteID, person.ID); err != nil {
		return tenant.Principal{}, err
	}
	if err := appendEvent(ctx, tx, person, "invite.accepted", nil, map[string]any{"invite_id": inviteID, "email": email, "principal_id": person.ID}); err != nil {
		return tenant.Principal{}, err
	}
	return person, nil
}

func scanNewPerson(ctx context.Context, tx pgx.Tx, tenantID, identityID, name, email string) (tenant.Principal, error) {
	var p tenant.Principal
	var kind string
	var roles pgtype.FlatArray[string]
	err := tx.QueryRow(ctx, `INSERT INTO principals(tenant_id,kind,identity_id,name,email,roles)
		VALUES($1::uuid,'person',$2::uuid,$3,$4,'{}')
		RETURNING id::text, tenant_id::text, kind, name, roles`, tenantID, identityID, name, email).Scan(&p.ID, &p.TenantID, &kind, &p.Name, &roles)
	if err != nil {
		return tenant.Principal{}, err
	}
	p.Kind = tenant.PrincipalKind(kind)
	p.Roles = []string(roles)
	if p.Roles == nil {
		p.Roles = []string{}
	}
	return p, nil
}

func bindScope(ctx context.Context, tx pgx.Tx, actor tenant.Principal, principalID, roleID, scope, projectID string) error {
	var role RoleRef
	if err := tx.QueryRow(ctx, `SELECT id::text, key, name FROM roles WHERE tenant_id=$1::uuid AND id=$2::uuid`, actor.TenantID, roleID).Scan(&role.ID, &role.Key, &role.Name); err != nil {
		return err
	}
	var bindingID string
	var err error
	if scope == "workspace" {
		err = tx.QueryRow(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type)
			VALUES($1::uuid,$2::uuid,$3::uuid,'workspace')
			ON CONFLICT DO NOTHING RETURNING id::text`, actor.TenantID, principalID, roleID).Scan(&bindingID)
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO role_bindings(tenant_id,principal_id,role_id,scope_type,scope_id)
			VALUES($1::uuid,$2::uuid,$3::uuid,'project',$4::uuid)
			ON CONFLICT DO NOTHING RETURNING id::text`, actor.TenantID, principalID, roleID, projectID).Scan(&bindingID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	after := map[string]any{"principal_id": principalID, "scope": scope, "role": role}
	if projectID != "" {
		after["project_id"] = projectID
	}
	return appendEvent(ctx, tx, actor, "binding.set", nil, after)
}

type projectGrant struct{ project, role string }

func inviteProjectGrants(ctx context.Context, tx pgx.Tx, tenantID, inviteID string) ([]projectGrant, error) {
	rows, err := tx.Query(ctx, `SELECT project_id::text, role_id::text FROM invite_project_roles WHERE tenant_id=$1::uuid AND invite_id=$2::uuid`, tenantID, inviteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := []projectGrant{}
	for rows.Next() {
		var g projectGrant
		if err := rows.Scan(&g.project, &g.role); err != nil {
			return nil, err
		}
		grants = append(grants, g)
	}
	return grants, rows.Err()
}
