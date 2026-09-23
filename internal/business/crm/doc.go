// SPDX-License-Identifier: AGPL-3.0-only

// Package crm is the business_crm plugin (R4, migration 0401).
//
// Organisations and contacts remain ordinary R1 nodes. This package adds the
// explicit contact-principal binding and the compiled manifest. Graph links
// stay on POST /api/relations; customer_of and contact_for are checked for
// live kind and direction there. Email equality never binds a principal and
// never grants quote acceptance.
//
// Coordinator wiring. Register the manifest before Seal, then mount the
// module. Do not edit internal/plugins/builtin.go or cmd/aeon from this
// package:
//
//	plug, err := crm.Plugin()
//	if err != nil { ... }
//	if err := reg.Register(plug); err != nil { ... }
//	reg.Seal()
//	srv.Modules = append(srv.Modules, crm.New(pool, reg))
//
// The Vue view is web/src/views/business/CRMView.vue. Register it in
// web/src/router.ts (this package does not edit that file):
//
//	{ path: '/crm', component: () => import('./views/business/CRMView.vue'), meta: { title: 'CRM' } }
//
// Manifest id business_crm. Permissions are only nodes.contribute,
// views.provide and steps.apply. Kinds are organisation and contact, the view
// id is crm, and the crm_bind step declares observed_state and person_decision.
// crm_bind is bound to steps.apply. The host calls plugins.Enabled for that
// operation inside the tenant transaction and checks again after locking the
// contact. A plugin result does not grant the binding: only an admin person
// session does, and only by writing the row. A repeat of the same contact and
// principal returns the existing row and appends no event.
package crm
