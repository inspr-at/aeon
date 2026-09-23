// SPDX-License-Identifier: AGPL-3.0-only
import 'vue-router'
declare module 'vue-router' {
  interface RouteMeta { title?: string; legacySearch?: boolean }
}
export {}
