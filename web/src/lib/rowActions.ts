// SPDX-License-Identifier: AGPL-3.0-only
// The shape of a list row's actions (quotes, customers): what RowMenu shows.
import type { BizIconName } from '../components/business/BizIcon.vue'

// One entry of a row's action menu. `reason` makes it unavailable and says why;
// `group` starts a new block under a thin line; `danger` marks what removes.
export interface RowAction { id: string; label: string; icon: BizIconName; group: number; reason?: string; danger?: boolean; keys?: string; busy?: boolean }
// A menu opens at its … button, or where the pointer was for a right-click.
export type RowMenuAnchor = HTMLElement | { x: number; y: number }
// The keys that open a row's menu from the keyboard: the context-menu key or Shift+F10.
export const opensRowMenu = (event: KeyboardEvent) => event.key === 'ContextMenu' || (event.key === 'F10' && event.shiftKey)
