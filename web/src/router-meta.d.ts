// SPDX-License-Identifier: AGPL-3.0-only
import 'vue-router'
declare module 'vue-router' {
  // fill: the view owns the full height of the page, so the in-flow footer is left out.
  // bare: no header and no footer (sign-in brings its own).
  interface RouteMeta { title?: string; legacySearch?: boolean; fill?: boolean; bare?: boolean }
}
export {}
