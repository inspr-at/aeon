// SPDX-License-Identifier: AGPL-3.0-only

// Package crm implements the business_crm plugin and QP2 backend.
//
// Customers and contacts are R1 organisation/contact nodes. CRM handlers keep
// their full typed fields, addresses and exact minor-unit rates in node fields.
// crm_organisation_profiles and crm_contact_profiles add revisions and one
// primary contact; the contact_for relation remains the graph link. Database
// triggers create profiles for generic node/relation creation and advance CRM
// revisions after generic node edits. A matching email never creates a
// principal binding; POST /crm/contacts/{id}/principals remains explicit.
// Deletion soft-deletes contacts and customers with an event per change. Quotes
// of any state and live linked projects prevent customer deletion. Customer
// numbers are allocated by the QP1 quote service on first offer; this module
// displays them and delegates guarded legacy conversion to QP1. Sender setup
// is the revisioned /api/quotes/settings service; CRM does not keep a second
// sender record. Amounts in CRM are integer minor units.
//
// Optional providers are compiled host adapters. NewHTTPProvider and
// NewHubSpotProvider require a host-supplied HTTPS origin and secret resolver;
// the tenant API stores only an opaque secret reference and cannot choose an
// outbound URL or credential. NewWithProviders mounts those adapters. With no
// adapters and disabled integration grant, manual CRM continues to work.
// Search errors remain isolated; import and sync are explicit admin actions.
// Note rewriting creates a persisted proposal; only a separate admin apply
// action changes customer_notes, fenced by the customer revision. Optional AI
// generation invokes the declared crm_note_optimize agent tool after resolving
// an enabled tenant scout model profile. The host supplies NoteGenerator;
// absent adapter, tool permission, notes or model route disables the action.
// Generation emits only a draft and safe model/usage metadata, never note text
// into tenant-wide events. No vendor credentials are handled by CRM.
//
// Coordinator wiring: register PluginWithProvidersAndNoteGenerator(providers,
// generator) before Seal, mount NewWithProvidersAndNoteGenerator(pool, reg,
// providers, generator), and pass
// events.WithUndoHandlers(crm.UndoHandlers(reg)) to events.New. For manual-only
// installations, Plugin() and New(pool, reg) expose the same manifest and HTTP
// module. The coordinator alone edits cmd/aeon, plugins/builtin.go and the web
// router. An existing installation must upgrade to the new manifest digest.
package crm
