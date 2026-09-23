// SPDX-License-Identifier: AGPL-3.0-only
import { reactive, ref, type Ref } from 'vue'
import { APIError, getNode, listNodes, updateNode, type Facets, type ListItem } from './api'
import { activeDimensions, apiParams, DIMENSIONS, WORK_KINDS, type Dimension, type ListFilters } from './ticketList'
import { toast } from './toast'
import { normaliseState, statusMeta } from './work'

const FACETS = DIMENSIONS.map(d => d.facet)
function message(error: unknown) { return error instanceof Error ? error.message : 'Something went wrong' }

// Loads one project's ticket list page by page (cursor paging, 200 rows),
// with facet counts. A dimension that is itself filtered gets its counts
// from a second, one-row request without that filter, so its menu still
// shows what else could be chosen.
export function useTicketList(projectId: Ref<string | null>, filters: Ref<ListFilters>) {
  const rows = ref<ListItem[]>([])
  const cursor = ref<string | null>(null)
  const loading = ref(false)
  const loadingMore = ref(false)
  const error = ref('')
  const moreError = ref('')
  const facets = ref<Facets>({})
  const dimensionFacets = ref<Partial<Record<Dimension, Record<string, number>>>>({})
  const names = reactive(new Map<string, string>())
  const loadedOnce = ref(false)
  let generation = 0

  function learn(items: ListItem[]) { for (const item of items) if (item.assignee) names.set(item.assignee.id, item.assignee.name) }

  // pageSize 1 fetches counts and facets only (the Outline builds its own rows then).
  let pageSize = 200
  async function load(options: { pageSize?: number } = {}) {
    const within = projectId.value
    if (!within) return
    pageSize = options.pageSize ?? 200
    const request = ++generation
    const current = filters.value
    loading.value = true; loadingMore.value = false; error.value = ''; moreError.value = ''
    const extras = activeDimensions(current).map(async dimension => {
      const facet = DIMENSIONS.find(d => d.key === dimension)!.facet
      const page = await listNodes(apiParams(within, current, { omit: dimension, facets: [facet], limit: 1 }))
      return [dimension, page.facets?.[facet] ?? {}] as const
    })
    try {
      const page = await listNodes(apiParams(within, current, { facets: FACETS, limit: pageSize }))
      if (request !== generation) return
      rows.value = page.items
      cursor.value = page.next_cursor
      facets.value = page.facets ?? {}
      learn(page.items)
      loadedOnce.value = true
    } catch (e) {
      if (request === generation) { error.value = message(e); rows.value = []; cursor.value = null }
    } finally {
      if (request === generation) loading.value = false
    }
    const settled = await Promise.allSettled(extras)
    if (request !== generation) return
    dimensionFacets.value = Object.fromEntries(settled.flatMap(result => result.status === 'fulfilled' ? [result.value] : []))
  }

  // One next-page request at a time; callers that need the page await the same promise.
  let moreRequest: Promise<void> | null = null
  function loadMore(): Promise<void> {
    if (loadingMore.value && moreRequest) return moreRequest
    moreRequest = fetchMore().finally(() => { moreRequest = null })
    return moreRequest
  }
  async function fetchMore() {
    const within = projectId.value
    if (!within || !cursor.value || loading.value) return
    const request = generation
    loadingMore.value = true; moreError.value = ''
    try {
      const page = await listNodes(apiParams(within, filters.value, { facets: FACETS, cursor: cursor.value, limit: pageSize }))
      if (request !== generation) return
      const seen = new Set(rows.value.map(row => row.id))
      rows.value = [...rows.value, ...page.items.filter(item => !seen.has(item.id))]
      cursor.value = page.next_cursor
      learn(page.items)
    } catch (e) {
      if (request === generation) moreError.value = message(e)
    } finally {
      if (request === generation) loadingMore.value = false
    }
  }

  function counts(dimension: Dimension): Record<string, number> {
    const facet = DIMENSIONS.find(d => d.key === dimension)!.facet
    return (filters.value[dimension].length ? dimensionFacets.value[dimension] : undefined) ?? facets.value[facet] ?? {}
  }

  async function resolveNames(ids: string[]) {
    const within = projectId.value
    if (!within) return
    await Promise.all(ids.filter(id => id !== 'none' && !names.has(id)).map(async id => {
      try {
        const page = await listNodes({ within, kind: WORK_KINDS, assignee: [id], limit: 1 })
        const person = page.items[0]?.assignee
        if (person) names.set(id, person.name)
      } catch { /* the menu shows "Unknown person" */ }
    }))
  }

  function shiftFacet(from: string, to: string) {
    const state = facets.value.state
    if (!state) return
    if (state[from] !== undefined) state[from] = Math.max(0, state[from] - 1)
    state[to] = (state[to] ?? 0) + 1
  }

  // Optimistic status change: the row updates at once and the server confirms.
  // The PATCH carries the row's updated_at as If-Unmodified-Since; a newer
  // server copy answers 412, nothing is written, and the latest version is shown.
  async function setStatus(row: ListItem, state: string, options: { undo?: boolean } = {}): Promise<boolean> {
    const target = rows.value.find(item => item.id === row.id) ?? row
    const before = { state: target.state, updated_at: target.updated_at }
    if (normaliseState(before.state) === normaliseState(state)) return false
    target.state = state
    shiftFacet(before.state, state)
    try {
      const saved = await updateNode(target.id, { state }, { ifUnmodifiedSince: before.updated_at })
      Object.assign(target, { state: saved.state, updated_at: saved.updated_at })
      if (!options.undo) {
        toast(`${target.key} is now ${statusMeta(saved.state).label}`, { action: { label: 'Undo', run: () => void setStatus(target, before.state, { undo: true }) } })
      }
      return true
    } catch (e) {
      if (e instanceof APIError && e.status === 412) {
        try {
          const latest = await getNode(target.id)
          shiftFacet(state, latest.state)
          Object.assign(target, { title: latest.title, body: latest.body, fields: latest.fields, state: latest.state, updated_at: latest.updated_at })
        } catch {
          shiftFacet(state, before.state)
          Object.assign(target, before)
        }
        toast(`${target.key} was changed elsewhere, so your status change was not saved. The latest version is shown.`, { tone: 'error' })
        return false
      }
      shiftFacet(state, before.state)
      Object.assign(target, before)
      toast(`${target.key} keeps its status: ${message(e)}`, { tone: 'error', action: { label: 'Retry', run: () => void setStatus(target, state, options) } })
      return false
    }
  }

  function invalidate() { generation++ }
  // Every page of the current query, up to a cap (the Outline needs the whole match set).
  async function loadAll(cap = 2000): Promise<boolean> {
    const request = generation
    while (cursor.value && rows.value.length < cap && request === generation && !moreError.value) await loadMore()
    return !cursor.value
  }
  // Created and deleted work shows up at once, before the next reload.
  function insertRow(item: ListItem) {
    if (rows.value.some(row => row.id === item.id)) return
    rows.value = [item, ...rows.value]
    const kind = facets.value.kind
    if (kind) kind[item.kind_slug] = (kind[item.kind_slug] ?? 0) + 1
    const state = facets.value.state
    if (state) state[item.state] = (state[item.state] ?? 0) + 1
  }
  function removeRow(id: string) {
    const row = rows.value.find(item => item.id === id)
    if (!row) return
    rows.value = rows.value.filter(item => item.id !== id)
    const kind = facets.value.kind
    if (kind?.[row.kind_slug]) kind[row.kind_slug]--
    const state = facets.value.state
    if (state?.[row.state]) state[row.state]--
  }

  return { rows, cursor, loading, loadingMore, error, moreError, facets, names, loadedOnce, load, loadMore, loadAll, counts, resolveNames, setStatus, invalidate, insertRow, removeRow }
}
