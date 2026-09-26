// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/inspr-at/aeon/internal/db"
	"github.com/inspr-at/aeon/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProjectRolePair names a project node key and a project-grantable role key.
type ProjectRolePair struct {
	Project string
	Role    string
}

// OperatorValidateProjectRoles checks a key-creation request before issuing a
// credential. The later bind repeats these checks inside its write transaction.
func OperatorValidateProjectRoles(ctx context.Context, pool *pgxpool.Pool, tenantID string, pairs []ProjectRolePair) error {
	if len(pairs) == 0 {
		return nil
	}
	ctx = db.AllProjects(ctx, "operator project binding preflight")
	return db.InTenant(ctx, pool, tenantID, func(tx pgx.Tx) error {
		for _, pair := range pairs {
			if _, err := operatorProjectTx(ctx, tx, pair.Project); err != nil {
				return err
			}
			roleID, err := operatorRoleTx(ctx, tx, pair.Role)
			if err != nil {
				return err
			}
			role, err := roleTx(ctx, tx, roleID)
			if err != nil {
				return err
			}
			if !projectRoleAllowed(role) {
				return fmt.Errorf("project %q role %q: %w", pair.Project, pair.Role, errProjectRole)
			}
		}
		return nil
	})
}

// OperatorBindProjects applies all requested bindings in one tenant transaction.
// It is for the host CLI only; callers must not expose it through HTTP.
func OperatorBindProjects(ctx context.Context, pool *pgxpool.Pool, tenantID, principal string, pairs []ProjectRolePair) ([]ProjectBinding, error) {
	if len(pairs) == 0 {
		return nil, errors.New("at least one project role is required")
	}
	ctx = db.AllProjects(ctx, "operator project binding")
	out := make([]ProjectBinding, 0, len(pairs))
	err := db.InTenant(ctx, pool, tenantID, func(tx pgx.Tx) error {
		principalID, err := operatorPrincipalTx(ctx, tx, principal)
		if err != nil {
			return err
		}
		m := &Module{pool: pool}
		actor := tenant.Principal{TenantID: tenantID}
		for _, pair := range pairs {
			projectID, err := operatorProjectTx(ctx, tx, pair.Project)
			if err != nil {
				return err
			}
			roleID, err := operatorRoleTx(ctx, tx, pair.Role)
			if err != nil {
				return err
			}
			binding, err := m.setProjectBindingTx(ctx, tx, actor, projectID, principalID, roleID, true)
			if err != nil {
				return fmt.Errorf("project %q role %q: %w", pair.Project, pair.Role, err)
			}
			out = append(out, binding)
		}
		return nil
	})
	return out, err
}

// OperatorUnbindProject removes one project binding. A repeated removal is a
// no-op; workspace access is outside this command's scope.
func OperatorUnbindProject(ctx context.Context, pool *pgxpool.Pool, tenantID, principal, project string) error {
	ctx = db.AllProjects(ctx, "operator project binding")
	return db.InTenant(ctx, pool, tenantID, func(tx pgx.Tx) error {
		principalID, err := operatorPrincipalTx(ctx, tx, principal)
		if err != nil {
			return err
		}
		projectID, err := operatorProjectTx(ctx, tx, project)
		if err != nil {
			return err
		}
		err = (&Module{pool: pool}).removeProjectBindingTx(ctx, tx, tenant.Principal{TenantID: tenantID}, projectID, principalID, true)
		if errors.Is(err, errNoSuchBinding) || errors.Is(err, errViaWorkspace) {
			return nil
		}
		return err
	})
}

func operatorPrincipalTx(ctx context.Context, tx pgx.Tx, principal string) (string, error) {
	if uuidPattern.MatchString(principal) {
		var id string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM principals WHERE id=$1::uuid`, principal).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", fmt.Errorf("principal %q not found", principal)
			}
			return "", err
		}
		return id, nil
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM principals WHERE name=$1 ORDER BY id`, principal)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("principal %q not found", principal)
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("principal %q is ambiguous; candidate IDs: %s", principal, strings.Join(ids, ", "))
	}
	return ids[0], nil
}

func operatorProjectTx(ctx context.Context, tx pgx.Tx, key string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT n.id::text FROM nodes n JOIN node_kinds k ON k.tenant_id=n.tenant_id AND k.id=n.kind_id
		WHERE n.key=$1 AND n.deleted_at IS NULL AND k.slug='project'`, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("project %q not found", key)
	}
	return id, err
}

func operatorRoleTx(ctx context.Context, tx pgx.Tx, key string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM roles WHERE key=$1`, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("role %q not found", key)
	}
	return id, err
}
