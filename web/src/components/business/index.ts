// SPDX-License-Identifier: AGPL-3.0-only
// Business public surface (AEON-70): shared pieces of the Business pages.
//
// Routes live in web/src/router.ts: /business (overview), /business/hours and
// /business/rates. Every page frames itself with BusinessPage, which fails
// closed while its plugin is not enabled; the server checks every action.
// Money on screen comes from formatAmount or MoneyText with exact decimal
// strings (parseJSONExact keeps the JSON spelling; nothing becomes a float).
// Currency totals are listed per currency and never converted.
// TicketHours shows a ticket's logged time (with everything below it) in the
// ticket panel's properties once mounted there.

export { businessAreas, businessSections, businessViews, pluginLabel } from './areas'
export type { BusinessArea, BusinessIconName } from './areas'
export {
  availability,
  configurePlugin,
  disablePermissions,
  installationWrite,
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
export { addDecimal, formatAmount, formatDecimal, formatMinor, minorScale, multiplyDecimal, parseDecimal, parseJSONExact, readExactJSON, sumAmounts } from './money'
export { formatClock, formatDuration, formatSpan, parseDurationInput } from './duration'
export { default as BizIcon } from './BizIcon.vue'
export { default as BusinessPage } from './BusinessPage.vue'
export { default as MoneyText } from './MoneyText.vue'
export { default as TicketHours } from './TicketHours.vue'
