// SPDX-License-Identifier: AGPL-3.0-only

// Package tenantbootstrap is the operator-only CLI backend for a second
// tenant. The coordinator wires these commands into cmd/aeon:
//
//	paimos tenant create --slug SLUG --name NAME
//	paimos tenant principal bind-oidc --tenant SLUG --issuer URL \
//	    --subject SUBJECT --name NAME --role admin|member|customer
//
// Create and BindOIDC return stable IDs for machine-readable command output.
// The create command rejects a duplicate slug. Binding the same identity,
// name and role again is a no-op; changing name or role requires the same
// explicit bind command and records an event. Neither command may run as an
// HTTP handler, read a source PMA system, or infer membership from email.
// The coordinator then uses GET /auth/login?tenant=SLUG and the ordinary
// tenant-admin API for plugin installation and kinds. No bootstrap command
// installs a plugin or seeds business data without that admin sequence.
package tenantbootstrap
