// SPDX-License-Identifier: AGPL-3.0-only
// Business shell public surface (package P4.6, AEON-49).
//
// There is no httpapi.Module and no plugin manifest here. Cost units, CRM,
// quotes, and hours each export those constructors. This package exports the
// shared Vue surface and the route table the coordinator registers.
//
// Coordinator splice in web/src/router.ts, before the catch-all, plus a
// workspace link to /business. This package does not edit the router.
//
//   import BusinessHome from './views/business/BusinessHome.vue'
//   { path: '/business', component: BusinessHome, meta: { title: 'Business' } },
//   { path: '/business/costs', component: () => import('./views/business/CostUnitsView.vue'), meta: { title: 'Cost units' } },
//   { path: '/business/crm', component: () => import('./views/business/CRMView.vue'), meta: { title: 'Organisations' } },
//   { path: '/business/quotes', component: () => import('./views/business/QuotesView.vue'), meta: { title: 'Quotes' } },
//   { path: '/business/hours', component: () => import('./views/business/HoursView.vue'), meta: { title: 'Hours' } },
//
// Sibling views share one catalog fetch and fail closed:
//
//   const { items, error, busy, refresh } = useBusinessCatalog()
//   <BusinessShell title="Quotes" :catalog="items" :busy="busy" :error="error" @refresh="refresh">
//     <PluginGate plugin-id="business_quotes" :catalog="items" :error="error" :busy="busy">
//       <!-- server-checked section -->
//     </PluginGate>
//   </BusinessShell>
//
// Money on screen comes from formatDecimal, formatMinor, or MoneyAmount.
// Pass exact decimal strings or integer minor-unit strings. parseJSONExact
// keeps the JSON spelling so a response is never turned into a binary float.
// Currency totals are listed per currency and are not converted.

export { businessAreas, businessSections, businessViews, pluginLabel } from './areas'
export type { BusinessArea, BusinessIconName } from './areas'
export {
  availability,
  configurePlugin,
  disablePermissions,
  installationWrite,
  isTenantAdmin,
  listPlugins,
  parseCatalog,
  pinAction,
  pluginGate,
  shortDigest,
  statusLabel,
  useBusinessCatalog,
  BusinessAPIError,
} from './catalog'
export type { AreaAvailability, BusinessPlugin, GateState, InstallationWrite } from './catalog'
export { addDecimal, formatDecimal, formatMinor, minorScale, multiplyDecimal, parseDecimal, parseJSONExact, readExactJSON } from './money'
export { formatDuration } from './duration'
export { default as BusinessIcon } from './BusinessIcon.vue'
export { default as BusinessShell } from './BusinessShell.vue'
export { default as CurrencyTotals } from './CurrencyTotals.vue'
export { default as DurationText } from './DurationText.vue'
export { default as MoneyAmount } from './MoneyAmount.vue'
export { default as PluginGate } from './PluginGate.vue'
