// SPDX-License-Identifier: AGPL-3.0-only
import 'vue-router'
declare module 'vue-router' {
  // fill: the view owns the full height of the page, so the in-flow footer is left out.
  interface RouteMeta { title?: string; legacySearch?: boolean; fill?: boolean }
}
export {}
