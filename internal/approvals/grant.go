// SPDX-License-Identifier: AGPL-3.0-only

package approvals

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

// scopePattern is the approval scope check from migration 0202.
var scopePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// ScopeWithinKey reports whether scope is covered by an API key.
// A key scope covers itself and any dotted refinement (nodes.read covers
// nodes.read.fields). A broader scope would enlarge the key and is rejected.
// A nil or empty key scope list covers nothing.
func ScopeWithinKey(scope string, keyScopes []string) bool {
	if !scopePattern.MatchString(scope) {
		return false
	}
	for _, keyScope := range keyScopes {
		if keyScope == scope || strings.HasPrefix(scope, keyScope+".") {
			return true
		}
	}
	return false
}

// LiveGrant reports whether agentID holds an unrevoked, unexpired grant for
// scope on one resource, and that scope is inside keyScopes.
//
// Call it inside the sensitive action's db.InTenant transaction. resourceID
// is nil only for resourceKind "tenant". Matching is exact: a tenant grant
// does not authorize a node or a run. A grant outside the acting key's
// scopes is not live, so an approval cannot enlarge that key.
func LiveGrant(ctx context.Context, tx pgx.Tx, agentID, scope, resourceKind string, resourceID *string, keyScopes []string) (bool, error) {
	if !ScopeWithinKey(scope, keyScopes) {
		return false, nil
	}
	switch resourceKind {
	case "tenant", "node", "run":
	default:
		return false, nil
	}
	if (resourceKind == "tenant") != (resourceID == nil) {
		return false, nil
	}
	if !uuidPattern.MatchString(agentID) {
		return false, fmt.Errorf("approvals: agent id is not a uuid")
	}
	if resourceID != nil && !uuidPattern.MatchString(*resourceID) {
		return false, fmt.Errorf("approvals: resource id is not a uuid")
	}
	var live bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM agent_permission_grants
			WHERE agent_principal_id = $1::uuid
			  AND scope = $2
			  AND resource_kind = $3
			  AND resource_id IS NOT DISTINCT FROM $4::uuid
			  AND revoked_at IS NULL
			  AND valid_until > now()
		)`, agentID, scope, resourceKind, resourceID).Scan(&live)
	return live, err
}
