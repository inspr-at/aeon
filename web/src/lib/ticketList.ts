// SPDX-License-Identifier: AGPL-3.0-only
// The ticket list's view state lives in the URL query so reloads, deep links
// and browser history keep it. These helpers translate between the query, the
// list API parameters and the grouped rows; they are pure for unit tests.
import type { Facets, ListItem, ListQuery } from './api.ts'
import { DEFAULT_SORT, KINDS, PRIORITIES, normaliseState, parseSort, serializeSort, statusMeta, statusOptions, type SortKey } from './work.ts'

export type Dimension = 'status' | 'priority' | 'assignee' | 'type'
export type GroupBy = 'none' | 'status' | 'epic'
export interface FacetOption { value: string; label: string; count?: number }
export interface ListFilters {
  q: string
  status: string[]
  priority: string[]
  assignee: string[]
  type: string[]
  showClosed: boolean
  sort: SortKey[]
  group: GroupBy
}
export const DIMENSIONS: { key: Dimension; title: string; facet: 'state' | 'priority' | 'assignee' | 'kind' }[] = [
  { key: 'status', title: 'Status', facet: 'state' },
  { key: 'priority', title: 'Priority', facet: 'priority' },
  { key: 'assignee', title: 'Assignee', facet: 'assignee' },
  { key: 'type', title: 'Type', facet: 'kind' },
]
export const WORK_KINDS = ['ticket', 'task', 'epic']
export const PAGE_SIZE = 200

function list(value: unknown): string[] {
  const raw = Array.isArray(value) ? value.join(',') : typeof value === 'string' ? value : ''
  return [...new Set(raw.split(',').map(part => part.trim()).filter(Boolean))]
}
export function filtersFromQuery(query: Record<string, unknown>): ListFilters {
  const group = query.group === 'status' || query.group === 'epic' ? query.group : 'none'
  return {
    q: typeof query.q === 'string' ? query.q : '',
    status: list(query.status),
    priority: list(query.priority),
    assignee: list(query.assignee),
    type: list(query.type).filter(kind => WORK_KINDS.includes(kind)),
    showClosed: query.closed === '1',
    sort: parseSort(typeof query.sort === 'string' ? query.sort : ''),
    group,
  }
}
export function filtersToQuery(filters: ListFilters): Record<string, string> {
  const out: Record<string, string> = {}
  if (filters.q.trim()) out.q = filters.q.trim()
  for (const key of ['status', 'priority', 'assignee', 'type'] as const) if (filters[key].length) out[key] = filters[key].join(',')
  if (filters.showClosed) out.closed = '1'
  if (filters.sort.length) out.sort = serializeSort(filters.sort)
  if (filters.group !== 'none') out.group = filters.group
  return out
}
export function activeDimensions(filters: ListFilters): Dimension[] {
  return DIMENSIONS.map(d => d.key).filter(key => filters[key].length > 0)
}
export function hasFilters(filters: ListFilters): boolean {
  return !!filters.q.trim() || activeDimensions(filters).length > 0
}

// The server matches states exactly; imported data spells in-progress with a hyphen.
function stateSpellings(states: string[]): string[] {
  const out = new Set<string>()
  for (const state of states) {
    out.add(state)
    const normal = normaliseState(state)
    if (normal === 'in_progress') { out.add('in_progress'); out.add('in-progress') }
  }
  return [...out]
}
export function effectiveSort(filters: ListFilters): SortKey[] {
  const keys = filters.sort.length ? [...filters.sort] : [...DEFAULT_SORT]
  if (filters.group === 'status' && keys[0]?.field !== 'state') {
    const index = keys.findIndex(key => key.field === 'state')
    keys.unshift(index >= 0 ? keys.splice(index, 1)[0] : { field: 'state', desc: false })
  }
  if (!keys.some(key => key.field === 'updated_at')) keys.push({ field: 'updated_at', desc: true })
  return keys
}
export function apiParams(within: string, filters: ListFilters, options: { omit?: Dimension; facets?: string[]; limit?: number; cursor?: string } = {}): ListQuery {
  const { omit } = options
  return {
    within,
    kind: omit !== 'type' && filters.type.length ? filters.type : WORK_KINDS,
    state: omit === 'status' ? [] : stateSpellings(filters.status),
    priority: omit === 'priority' ? [] : filters.priority,
    assignee: omit === 'assignee' ? [] : filters.assignee,
    q: filters.q.trim(),
    hide_closed: !filters.showClosed,
    facets: options.facets,
    sort: serializeSort(effectiveSort(filters)),
    limit: options.limit ?? PAGE_SIZE,
    cursor: options.cursor,
  }
}

export function totalFrom(facets: Facets | undefined): number | null {
  const kinds = facets?.kind
  return kinds ? Object.values(kinds).reduce((sum, count) => sum + count, 0) : null
}

export function facetOptions(dimension: Dimension, counts: Record<string, number> = {}, selected: string[] = [], names: Map<string, string> = new Map(), me?: string): FacetOption[] {
  if (dimension === 'status') {
    const byKey = new Map<string, FacetOption>()
    const seen = new Set<string>()
    const add = (value: string) => {
      if (seen.has(value)) return
      seen.add(value)
      const meta = statusMeta(value)
      const key = meta.key === 'other' ? normaliseState(value) : meta.key
      const existing = byKey.get(key)
      const count = counts[value] ?? 0
      // Spellings of one status (in_progress, in-progress) share one option; the
      // option keeps a selected spelling, else the one the data uses.
      if (existing) {
        existing.count = (existing.count ?? 0) + count
        if (counts[value] !== undefined && !selected.includes(existing.value)) existing.value = value
        return
      }
      byKey.set(key, { value, label: meta.label, count })
    }
    for (const option of statusOptions(Object.keys(counts))) add(option.value)
    for (const value of [...Object.keys(counts), ...selected]) add(value)
    return [...byKey.values()].sort((a, b) => statusMeta(a.value).order - statusMeta(b.value).order)
  }
  if (dimension === 'priority') {
    return [...PRIORITIES.map(p => ({ value: p.value as string, label: p.label as string })), { value: 'none', label: 'No priority' }]
      .map(option => ({ ...option, count: counts[option.value] ?? 0 }))
  }
  if (dimension === 'type') return KINDS.map(kind => ({ value: kind.value, label: kind.label, count: counts[kind.value] ?? 0 }))
  const ids = [...new Set([...Object.keys(counts), ...selected])].filter(id => id !== 'none')
  ids.sort((a, b) => (a === me ? -1 : b === me ? 1 : (counts[b] ?? 0) - (counts[a] ?? 0)))
  return [
    { value: 'none', label: 'Unassigned', count: counts.none ?? 0 },
    ...ids.map(id => ({ value: id, label: names.get(id) ?? 'Unknown person', count: counts[id] ?? 0 })),
  ]
}

// Client-side ordering with the same keys as the list API, for siblings in the Outline
// that come from different requests (matches and their ancestors).
const PRIORITY_RANK: Record<string, number> = { high: 0, medium: 1, low: 2 }
export function compareRows(keys: SortKey[]): (a: ListItem, b: ListItem) => number {
  const value = (row: ListItem, field: SortKey['field']): string | number => {
    switch (field) {
      case 'state': return statusMeta(row.state).order
      case 'priority': return PRIORITY_RANK[row.priority ?? ''] ?? 3
      case 'updated_at': return Date.parse(row.updated_at) || 0
      case 'key': { const [prefix, number] = row.key.split('-'); return `${prefix}-${number.padStart(9, '0')}` }
      case 'title': return row.title.toLowerCase()
      case 'kind': return row.kind_slug
    }
  }
  return (a, b) => {
    for (const key of keys) {
      const x = value(a, key.field), y = value(b, key.field)
      if (x !== y) return (x < y ? -1 : 1) * (key.desc ? -1 : 1)
    }
    return a.id < b.id ? -1 : 1
  }
}

// Stable status ordering by workflow (the server orders unknown spellings last).
export function orderByStatus(rows: ListItem[], desc = false): ListItem[] {
  return rows
    .map((row, index) => ({ row, index, order: statusMeta(row.state).order }))
    .sort((a, b) => (desc ? b.order - a.order : a.order - b.order) || a.index - b.index)
    .map(entry => entry.row)
}

export interface EpicRef { id: string; key: string; title: string }
export function epicOf(row: ListItem, byId: Map<string, ListItem>): EpicRef | null {
  if (row.kind_slug === 'epic') return { id: row.id, key: row.key, title: row.title }
  let parent = row.parent
  for (let depth = 0; parent && depth < 8; depth++) {
    if (parent.kind_slug === 'epic') return { id: parent.id, key: parent.key, title: parent.title }
    const loaded = byId.get(parent.id)
    parent = loaded?.parent ?? null
  }
  return null
}

export interface RowGroup { key: string; label: string; state?: string; epic?: EpicRef; rows: ListItem[]; total: number }
export function groupRows(rows: ListItem[], group: GroupBy, stateCounts: Record<string, number> = {}): RowGroup[] {
  if (group === 'none') return [{ key: 'all', label: '', rows, total: rows.length }]
  if (group === 'status') {
    const groups = new Map<string, RowGroup>()
    for (const row of rows) {
      const meta = statusMeta(row.state)
      const key = meta.key === 'other' ? normaliseState(row.state) : meta.key
      if (!groups.has(key)) groups.set(key, { key, label: meta.label, state: row.state, rows: [], total: 0 })
      groups.get(key)!.rows.push(row)
    }
    for (const entry of groups.values()) {
      const counted = Object.entries(stateCounts).filter(([state]) => {
        const meta = statusMeta(state)
        return (meta.key === 'other' ? normaliseState(state) : meta.key) === entry.key
      }).reduce((sum, [, count]) => sum + count, 0)
      entry.total = Math.max(counted, entry.rows.length)
    }
    return [...groups.values()].sort((a, b) => statusMeta(a.state!).order - statusMeta(b.state!).order)
  }
  const byId = new Map(rows.map(row => [row.id, row]))
  const groups = new Map<string, RowGroup>()
  const none: RowGroup = { key: 'none', label: 'No epic', rows: [], total: 0 }
  for (const row of rows) {
    const epic = epicOf(row, byId)
    if (!epic) { none.rows.push(row); continue }
    if (!groups.has(epic.id)) groups.set(epic.id, { key: epic.id, label: epic.title, epic, rows: [], total: 0 })
    if (row.kind_slug !== 'epic') groups.get(epic.id)!.rows.push(row)
  }
  const out = [...groups.values(), ...(none.rows.length ? [none] : [])]
  for (const entry of out) entry.total = entry.rows.length
  return out
}
