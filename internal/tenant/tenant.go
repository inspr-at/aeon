// SPDX-License-Identifier: AGPL-3.0-only

// Package tenant carries the acting principal and its tenant through a request.
// Shared contract between P0.2 (core) and P0.3 (auth); extend, do not rename.
package tenant

import (
	"context"
	"slices"
)

// PrincipalKind distinguishes people from agents; both are first-class.
type PrincipalKind string

const (
	Person PrincipalKind = "person"
	Agent  PrincipalKind = "agent"
)

// Principal is who acts, always inside exactly one tenant.
type Principal struct {
	ID       string // principals.id (uuid)
	TenantID string // tenants.id (uuid)
	Kind     PrincipalKind
	Name     string
	Roles    []string // e.g. "admin", "member"
	Scopes   []string // authenticated agent key's outer permission ceiling
}

type ctxKey struct{}

// WithPrincipal returns a context carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	// A global administrator has tenant-admin authority in every handler that
	// checks the existing admin role. Never confer that authority on an agent.
	if p.Kind == Person && slices.Contains(p.Roles, "super_admin") && !slices.Contains(p.Roles, "admin") {
		p.Roles = append(slices.Clone(p.Roles), "admin")
	}
	return context.WithValue(ctx, ctxKey{}, p)
}

// IsAdmin requires a person, including a global administrator.
func IsAdmin(p Principal) bool {
	return p.Kind == Person && (slices.Contains(p.Roles, "admin") || slices.Contains(p.Roles, "super_admin"))
}

// PrincipalFrom returns the principal set by the auth middleware, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}
