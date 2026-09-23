// SPDX-License-Identifier: AGPL-3.0-only

// Package principallink supplies operator-only identity reconciliation.
// The coordinator wires Run(ctx, pool, args, stdout) to `aeon principal`:
//
//	aeon principal link --tenant SLUG --from ID_OR_NAME --to ID_OR_NAME
//	aeon principal unlink --tenant SLUG --from ID_OR_NAME
//	aeon principal link --tenant SLUG --suggest
//
// New(pool) constructs the service; Link, Unlink and Suggest accept tenant
// slugs. There is deliberately no httpapi.Module or plugin manifest: possession
// of the operator's database connection is the CLI boundary, and ordinary
// sessions/plugins must not expose these methods. Link/unlink events name the
// dedicated operator agent, never pretend the target person performed the action.
// Exact replay is a no-op; changing a target requires an explicit unlink first.
// Suggestions are read-only evidence, never authorization or automatic linking.
//
// Links change presentation and future assignment/comment writes only. They do
// not transfer roles, identities, sessions or access grants. Historical IDs and
// source records remain intact; unlink restores their original presentation.
// Principals retain classic usernames and optional email for manual matching;
// OIDC email is read from the bound identity when no principal email is stored.
// For existing imports the coordinator may call importer.BackfillPrincipals
// after migration to restore usernames and populate principal email from stored
// identity/user metadata. This is an explicit, evented operation; neither the
// migration nor Suggest mutates existing principal data.
// Migration 0532 supplies the tenant FK and person-only one-hop invariant.
package principallink
