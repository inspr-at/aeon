// SPDX-License-Identifier: AGPL-3.0-only
// Ticket list columns: which show at a given table width, in which order and how
// wide. Without a saved choice the table fills its width: narrow tables drop
// columns, wide ones (from ~1500px of table, about an 1800px screen) add Assignee,
// Epic, Estimate and Created. A saved choice fixes order and visibility; columns
// that cannot fit still step aside from the end. Free of Vue for unit tests.
import type { SortField } from './work.ts'

export type ColumnId = 'key' | 'title' | 'status' | 'priority' | 'assignee' | 'epic' | 'estimate' | 'created' | 'updated'
export interface ColumnDef { id: ColumnId; label: string; sort: SortField | null; width: number; min: number; max: number; end?: boolean }
export interface ListPrefs { order?: ColumnId[]; visible?: ColumnId[]; widths?: Partial<Record<ColumnId, number>> }

export const COLUMNS: ColumnDef[] = [
  { id: 'key', label: 'Key', sort: 'key', width: 118, min: 84, max: 220 },
  { id: 'title', label: 'Title', sort: 'title', width: 0, min: 240, max: 4000 },
  { id: 'status', label: 'Status', sort: 'state', width: 138, min: 84, max: 260 },
  { id: 'priority', label: 'Priority', sort: 'priority', width: 112, min: 72, max: 200 },
  { id: 'assignee', label: 'Assignee', sort: null, width: 156, min: 96, max: 320 },
  { id: 'epic', label: 'Epic', sort: null, width: 220, min: 110, max: 480 },
  { id: 'estimate', label: 'Estimate', sort: null, width: 96, min: 72, max: 180, end: true },
  { id: 'created', label: 'Created', sort: null, width: 104, min: 80, max: 200, end: true },
  { id: 'updated', label: 'Updated', sort: 'updated_at', width: 104, min: 80, max: 200, end: true },
]
export const COLUMN_BY_ID = new Map(COLUMNS.map(column => [column.id, column]))
// Key and Title always lead; the rest can be hidden and reordered.
export const PINNED: ColumnId[] = ['key', 'title']
export const WIDE_TABLE = 1500
const PHONE: ColumnId[] = ['key', 'title', 'status', 'priority', 'updated']
// The order columns leave in when space runs out: the least essential first.
const DROP_ORDER: ColumnId[] = ['estimate', 'created', 'epic', 'assignee', 'updated', 'priority', 'status']

export function orderOf(prefs: ListPrefs | null | undefined): ColumnId[] {
  const valid = (prefs?.order ?? []).filter((id): id is ColumnId => COLUMN_BY_ID.has(id as ColumnId) && !PINNED.includes(id as ColumnId))
  const rest = COLUMNS.map(c => c.id).filter(id => !PINNED.includes(id) && !valid.includes(id))
  return [...PINNED, ...new Set(valid), ...rest]
}

// Wide tables add Epic and Created; Assignee and Estimate only when some row has one.
export function automaticColumns(tableWidth: number, anyAssigned: boolean, anyEstimate = false): ColumnId[] {
  const out: ColumnId[] = ['key', 'title', 'status', 'priority']
  if (tableWidth >= WIDE_TABLE) return [...out, ...(anyAssigned ? ['assignee' as const] : []), 'epic', ...(anyEstimate ? ['estimate' as const] : []), 'created', 'updated']
  if (tableWidth > 900 && anyAssigned) out.push('assignee')
  if (tableWidth > 740) out.push('updated')
  return out
}

export function widthOf(id: ColumnId, prefs: ListPrefs | null | undefined): number {
  const def = COLUMN_BY_ID.get(id)!
  const saved = prefs?.widths?.[id]
  return typeof saved === 'number' && Number.isFinite(saved) ? Math.max(def.min, Math.min(def.max, Math.round(saved))) : def.width
}

// The columns to show, in order. `customised` is true when a saved choice applies.
export function visibleColumns(tableWidth: number, options: { phone: boolean; anyAssigned: boolean; anyEstimate?: boolean; prefs?: ListPrefs | null }): { columns: ColumnDef[]; customised: boolean } {
  if (options.phone) return { columns: PHONE.map(id => COLUMN_BY_ID.get(id)!), customised: false }
  const prefs = options.prefs
  const customised = !!prefs?.visible
  const chosen = customised ? new Set<ColumnId>([...PINNED, ...prefs!.visible!]) : new Set(automaticColumns(tableWidth, options.anyAssigned, options.anyEstimate))
  let ids = orderOf(prefs).filter(id => chosen.has(id))
  // Whatever does not fit beside a readable title steps aside, least essential first.
  const title = COLUMN_BY_ID.get('title')!.min
  const total = () => ids.reduce((sum, id) => sum + (id === 'title' ? title : widthOf(id, prefs)), 0)
  for (const id of DROP_ORDER) {
    if (total() <= tableWidth || tableWidth <= 0) break
    ids = ids.filter(x => x !== id)
  }
  return { columns: ids.map(id => COLUMN_BY_ID.get(id)!), customised }
}

// Moving a column one step within the reorderable part.
export function moveColumn(order: ColumnId[], id: ColumnId, step: -1 | 1): ColumnId[] {
  const free = order.filter(x => !PINNED.includes(x))
  const index = free.indexOf(id)
  const to = index + step
  if (index === -1 || to < 0 || to >= free.length) return order
  const next = [...free]
  ;[next[index], next[to]] = [next[to], next[index]]
  return [...PINNED, ...next]
}
