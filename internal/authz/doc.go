// SPDX-License-Identifier: AGPL-3.0-only

// Package authz implements ADR-003 access decisions. Permissions are the unit
// of authority; versioned built-in roles and tenant custom roles bundle them.
// Owner has the entire registry, Admin lacks ownership.transfer, Member can
// perform project work but cannot manage workspace policy, Viewer reads product
// data, Guest reads project work and comments, and Customer has only the own
// quote portal and profile permissions. Custom roles hold registry keys in
// role_permissions. No mutation may grant keys its actor lacks.
//
// A principal's active workspace binding supplies P1 access. Project bindings
// are represented in the schema for P2; effective project access is the union
// of its workspace and project grants. At most one binding exists per scope.
// Agent key scopes narrow this set; an empty scope list grants no API access.
// Deactivated principals have no effective permissions. Database triggers
// revoke their sessions and keys and protect the last active workspace owner.
// Service principals have no binding and use explicit internal call paths.
//
// The migration maps classic role labels into workspace bindings while keeping
// principals.roles readable for one release. Import reruns may seed a missing
// binding but cannot overwrite an existing one. Access mutations append events
// in the same db.InTenant transaction as the changed rows.
//
// New returns an httpapi.Module for /api/authz/permissions, /api/roles,
// /api/members and /api/me/permissions. The coordinator mounts it; this
// package does not edit cmd/aeon, web/src/router.ts or plugins/builtin.go.
// Every authorizing route must use Handle or Require with a declared registry
// permission. RoutePermissions names the permission for every current API
// pattern, including explicit public paths; RequirePattern denies an unknown
// pattern. BindPool attaches the database needed by Require to a request.
package authz
