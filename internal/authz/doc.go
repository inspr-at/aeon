// SPDX-License-Identifier: AGPL-3.0-only

// Package authz implements ADR-003 access decisions. Permissions are the unit
// of authority; versioned built-in roles and tenant custom roles bundle them.
// A principal's active workspace binding supplies P1 access. Project bindings
// are represented in the schema for P2; effective project access is the union
// of its workspace and project grants. Agent key scopes narrow this set.
// Service principals have no binding and use explicit internal call paths.
//
// New returns an httpapi.Module for /api/authz/permissions, /api/roles,
// /api/members and /api/me/permissions. The coordinator mounts it; this
// package does not edit cmd/aeon, web/src/router.ts or plugins/builtin.go.
// Every authorizing route must use Handle or Require with a declared registry
// permission. BindPool attaches the database needed by Require to a request.
package authz
