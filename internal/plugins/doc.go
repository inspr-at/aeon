// SPDX-License-Identifier: AGPL-3.0-only

// Package plugins is AEON's first-party plugin registry (R3, migration 0303).
//
// Plugins are compiled Go values. Register rejects a duplicate id, an unknown
// permission, a node-kind or schema collision, and a manifest digest that does
// not match the canonical declaration. Nothing here loads code, SQL, or
// network endpoints at runtime.
//
// Manifest declarations are permission ceilings. A tenant admin pins an
// installation to the compiled digest and a permission subset with
// PUT /api/plugins/{pluginId}/installation. Every installation change and
// every background-job run appends a tenant event inside db.InTenant. A
// disabled installation, a digest mismatch, or a missing permission fails
// closed and starts no new work. Historical installation events keep the
// digest they recorded.
//
// Extension calls receive a context, the tenant principal, and a grant
// narrowed to the permissions that call needs. They do not receive a database
// pool, an environment map, or an HTTP client. The digest commits the public
// manifest and the step-to-permission bindings.
//
// Job leases are tenant-scoped and in-memory. Migration 0303 has no lease
// table; the durable record is the plugin.job_ran event. A lease does not
// outlive the process.
//
// The coordinator mounts the module and does not edit this package:
//
//	srv.Modules = append(srv.Modules, plugins.New(pool))
//
// New returns an httpapi.Module with Pharos and Janus registered. Another
// first-party package can register before the process serves:
//
//	reg := plugins.NewRegistry()
//	if err := reg.Register(plugin); err != nil { ... }
//	reg.Seal()
//	srv.Modules = append(srv.Modules, plugins.NewWithRegistry(pool, reg))
//
// Stage handoff uses Enabled and Registry.AuthorizeHandoff for its
// permission-only boundary and owns the transactional evidence/launch checks.
// Callers with typed observed facts use Module.Evaluate, Request and ApplyResult.
package plugins
