// SPDX-License-Identifier: AGPL-3.0-only

// Package costunits is the business_costs plugin (R4 package A, migration 0400).
//
// Cost units are R1 nodes of kind slug cost_unit, including classic PPM
// cost_unit nodes already imported with their original keys. This package does
// not rekey those nodes and does not read the classic platform. Rates are
// effective-dated rows in cost_unit_rates. Amounts are exact decimals with at
// most four fractional digits, never floats. A new price inserts a row and
// appends cost_unit_rate.created. When one open interval for the same cost
// unit, unit, and currency overlaps the new start, that interval is closed at
// the new start and cost_unit_rate.closed is appended; its amounts do not
// change. An exact retry of an existing start date appends nothing. Intervals
// are half-open and must not overlap. Currencies are not converted.
//
// Plugin returns the compiled manifest. The coordinator registers it before
// Seal and mounts New. This package does not edit internal/plugins/builtin.go,
// cmd/aeon, or web/src/router.ts.
//
//	plug, err := costunits.Plugin()
//	if err != nil { ... }
//	if err := reg.Register(plug); err != nil { ... }
//	reg.Seal()
//	srv.Modules = append(srv.Modules, costunits.New(pool, reg))
//
// The manifest id is business_costs. It declares the cost_unit kind, the
// cost_units view (panel rates), and the cost_rate_change step. The step binds
// steps.apply and declares observed_state and person_decision. The kind binds
// nodes.contribute and the view binds views.provide. There is no agent tool,
// runtime integration, or background job. The field schema is display metadata
// and accepts the classic issue fields already stored on imported nodes,
// including fields.classic.
//
// Routes, mounted on the shared /api mux behind the auth middleware:
//
//	GET  /api/cost-units/{costUnitId}/rates
//	POST /api/cost-units/{costUnitId}/rates
//
// GET requires an enabled installation pinned to this digest with
// views.provide. POST requires a person with the admin role and steps.apply.
// Both fail closed when the installation is missing, disabled, or pinned to
// another digest. A write locks the installation, then the cost-unit node,
// rechecks the pin, and calls plugins.Enabled for cost_rate_change in that
// same db.InTenant transaction. The step result is not authority for the
// insert: the handler checks the live cost_unit node, the admin person, and
// the interval itself.
//
// The coordinator adds the screen:
//
//	{ path: '/business/cost-units', component: () => import('./views/business/CostUnitsView.vue'), meta: { title: 'Cost units' } }
package costunits
