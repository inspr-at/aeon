// SPDX-License-Identifier: AGPL-3.0-only
// Ticket list columns: which show at a given table width, in which order and how
// wide. Without a saved choice the table fills its width: narrow tables drop
// columns, wide ones (from ~1500px of table, about an 1800px screen) add Assignee,
// Epic, Release and Tags (when some row has them), Estimate and Created. A saved
// choice fixes order and visibility; columns that cannot fit still step aside from
// the end. On wide tables Title stops at ~960px and the spare width goes to the
// text columns, so the metadata stays near the title. Free of Vue for unit tests.
import type { SortField } from './work.ts'

export type ColumnId = 'key' | 'title' | 'status' | 'priority' | 'assignee' | 'epic' | 'release' | 'tags' | 'cost' | 'estimate' | 'created' | 'updated'
export interface ColumnDef { id: ColumnId; label: string; sort: SortField | null; width: number; min: number; max: number; end?: boolean }
// defaultView: the saved view this person opens the project with.
export interface ListPrefs { order?: ColumnId[]; visible?: ColumnId[]; widths?: Partial<Record<ColumnId, number>>; defaultView?: string | null }

export const COLUMNS: ColumnDef[] = [
  { id: 'key', label: 'Key', sort: 'key', width: 118, min: 84, max: 220 },
  { id: 'title', label: 'Title', sort: 'title', width: 0, min: 240, max: 4000 },
  { id: 'status', label: 'Status', sort: 'state', width: 138, min: 84, max: 260 },
  { id: 'priority', label: 'Priority', sort: 'priority', width: 112, min: 72, max: 200 },
  { id: 'assignee', label: 'Assignee', sort: 'assignee', width: 156, min: 96, max: 320 },
  { id: 'epic', label: 'Epic', sort: null, width: 220, min: 110, max: 480 },
  { id: 'release', label: 'Release', sort: null, width: 132, min: 84, max: 260 },
  { id: 'tags', label: 'Tags', sort: null, width: 180, min: 96, max: 420 },
  { id: 'cost', label: 'Cost unit', sort: null, width: 150, min: 96, max: 320 },
  { id: 'estimate', label: 'Estimate', sort: null, width: 96, min: 72, max: 180, end: true },
  { id: 'created', label: 'Created', sort: 'created_at', width: 104, min: 80, max: 200, end: true },
  { id: 'updated', label: 'Updated', sort: 'updated_at', width: 104, min: 80, max: 200, end: true },
]
export const COLUMN_BY_ID = new Map(COLUMNS.map(column => [column.id, column]))
// Key and Title always lead; the rest can be hidden and reordered.
export const PINNED: ColumnId[] = ['key', 'title']
export const WIDE_TABLE = 1500
// Title's automatic width on wide tables; a width the person gives Title wins.
export const TITLE_TARGET = 960
// Automatic wide columns keep Title at least this wide; otherwise they step aside.
const TITLE_ROOM = 420
const PHONE: ColumnId[] = ['key', 'title', 'status', 'priority', 'updated']
// The order columns leave in when space runs out: the least essential first.
const DROP_ORDER: ColumnId[] = ['estimate', 'cost', 'tags', 'release', 'created', 'epic', 'assignee', 'updated', 'priority', 'status']
// Columns only wide tables add on their own.
const WIDE_EXTRAS: ColumnId[] = ['estimate', 'tags', 'release', 'created', 'epic', 'assignee']
// The text columns that take spare width on wide tables (their text gets room).
const GROWS: ColumnId[] = ['epic', 'tags', 'assignee', 'release', 'cost']
// Which optional values any loaded row has.
export interface Present { assigned?: boolean; estimate?: boolean; release?: boolean; tags?: boolean }

export function orderOf(prefs: ListPrefs | null | undefined): ColumnId[] {
  const valid = (prefs?.order ?? []).filter((id): id is ColumnId => COLUMN_BY_ID.has(id as ColumnId) && !PINNED.includes(id as ColumnId))
  const rest = COLUMNS.map(c => c.id).filter(id => !PINNED.includes(id) && !valid.includes(id))
  return [...PINNED, ...new Set(valid), ...rest]
}

// Wide tables add Assignee, Epic and Created, and Release, Tags and Estimate when
// some row has one; narrower ones show Assignee only when someone is assigned.
export function automaticColumns(tableWidth: number, present: Present = {}): ColumnId[] {
  const out: ColumnId[] = ['key', 'title', 'status', 'priority']
  if (tableWidth >= WIDE_TABLE) {
    const optional: ColumnId[] = [...(present.release ? ['release' as const] : []), ...(present.tags ? ['tags' as const] : []), ...(present.estimate ? ['estimate' as const] : [])]
    return [...out, 'assignee', 'epic', ...optional, 'created', 'updated']
  }
  if (tableWidth > 900 && present.assigned) out.push('assignee')
  if (tableWidth > 740) out.push('updated')
  return out
}

export function widthOf(id: ColumnId, prefs: ListPrefs | null | undefined): number {
  const def = COLUMN_BY_ID.get(id)!
  const saved = prefs?.widths?.[id]
  return typeof saved === 'number' && Number.isFinite(saved) ? Math.max(def.min, Math.min(def.max, Math.round(saved))) : def.width
}

// The columns to show, in order. `customised` is true when a saved choice applies.
export function visibleColumns(tableWidth: number, options: { phone: boolean; present?: Present; prefs?: ListPrefs | null }): { columns: ColumnDef[]; customised: boolean } {
  if (options.phone) return { columns: PHONE.map(id => COLUMN_BY_ID.get(id)!), customised: false }
  const prefs = options.prefs
  const customised = !!prefs?.visible
  const chosen = customised ? new Set<ColumnId>([...PINNED, ...prefs!.visible!]) : new Set(automaticColumns(tableWidth, options.present))
  let ids = orderOf(prefs).filter(id => chosen.has(id))
  const total = (title: number) => ids.reduce((sum, id) => sum + (id === 'title' ? title : widthOf(id, prefs)), 0)
  // Columns a wide table adds on its own leave first when Title would get cramped.
  if (!customised) for (const id of WIDE_EXTRAS) {
    if (total(TITLE_ROOM) <= tableWidth || tableWidth <= 0) break
    ids = ids.filter(x => x !== id)
  }
  // Whatever does not fit beside a readable title steps aside, least essential first.
  const title = COLUMN_BY_ID.get('title')!.min
  for (const id of DROP_ORDER) {
    if (total(title) <= tableWidth || tableWidth <= 0) break
    ids = ids.filter(x => x !== id)
  }
  return { columns: ids.map(id => COLUMN_BY_ID.get(id)!), customised }
}

// Widths for every visible column except Title (which takes the rest). Title aims
// for TITLE_TARGET, or the width the person gave it; spare width beyond that widens
// Epic, Tags, Assignee and Release (not ones the person sized) up to their maximum,
// in proportion to their normal width. What is still left goes back to Title.
export function layoutWidths(ids: ColumnId[], tableWidth: number, prefs?: ListPrefs | null, live: Partial<Record<ColumnId, number>> = {}): Partial<Record<ColumnId, number>> {
  const out: Partial<Record<ColumnId, number>> = {}
  for (const id of ids) if (id !== 'title') out[id] = live[id] ?? widthOf(id, prefs)
  const sized = (id: ColumnId) => live[id] !== undefined || typeof prefs?.widths?.[id] === 'number'
  const titleTarget = sized('title') ? live.title ?? widthOf('title', prefs) : TITLE_TARGET
  let spare = tableWidth - Object.values(out).reduce((sum, w) => sum + (w ?? 0), 0) - titleTarget
  const growing = GROWS.filter(id => ids.includes(id) && !sized(id))
  while (spare >= 1 && growing.length) {
    const weight = growing.reduce((sum, id) => sum + COLUMN_BY_ID.get(id)!.width, 0)
    let used = 0
    for (const id of [...growing]) {
      const def = COLUMN_BY_ID.get(id)!
      const add = Math.min(def.max - out[id]!, spare * def.width / weight)
      out[id] = out[id]! + add
      used += add
      if (out[id]! >= def.max - 0.5) growing.splice(growing.indexOf(id), 1)
    }
    spare -= used
    if (used < 0.5) break
  }
  for (const id of Object.keys(out) as ColumnId[]) out[id] = Math.floor(out[id]!)
  return out
}

// The release label and tags classic imports keep in the ticket's fields.
export function releaseLabel(fields: Record<string, unknown> | null | undefined): string {
  const value = fields?.release
  if (typeof value === 'string') return value.trim()
  if (value && typeof value === 'object') { const label = (value as { label?: unknown; name?: unknown }).label ?? (value as { name?: unknown }).name; return typeof label === 'string' ? label.trim() : '' }
  return ''
}
// The cost unit a ticket books to: its own, else the one classic PPM gave it.
export function costUnitLabel(fields: Record<string, unknown> | null | undefined): string {
  const label = (value: unknown): string => {
    if (typeof value === 'string') return value.trim()
    if (value && typeof value === 'object') { const v = (value as { label?: unknown; name?: unknown }).label ?? (value as { name?: unknown }).name; return typeof v === 'string' ? v.trim() : '' }
    return ''
  }
  if (fields && 'cost_unit' in fields) return label(fields.cost_unit)
  const classic = fields?.classic
  return classic && typeof classic === 'object' ? label((classic as Record<string, unknown>).cost_unit) : ''
}
export interface TagRef { name: string; color: string }
export function tagList(fields: Record<string, unknown> | null | undefined): TagRef[] {
  const value = fields?.tags
  if (!Array.isArray(value)) return []
  return value.flatMap(tag => {
    if (typeof tag === 'string') return tag.trim() ? [{ name: tag.trim(), color: '' }] : []
    if (tag && typeof tag === 'object' && typeof (tag as { name?: unknown }).name === 'string') {
      const { name, color } = tag as { name: string; color?: unknown }
      return name.trim() ? [{ name: name.trim(), color: typeof color === 'string' ? color : '' }] : []
    }
    return []
  })
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
