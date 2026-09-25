// SPDX-License-Identifier: AGPL-3.0-only
import { computed, ref, watch, type Ref } from 'vue'
import { countBy, filterItems, groupItems, isKnowledgeType, listKnowledge, type KnowledgeItem, type KnowledgeStatus, type KnowledgeType, type SortBy } from './knowledge'

// One project's knowledge for its tab and its entry pages: every entry is read
// once (knowledge is small), filters and sort apply in place, and a search asks
// the server, which also reads the bodies and cuts an excerpt around the match.
export type StatusView = 'current' | 'proposed' | 'archived' | 'all'
export const STATUS_VIEWS: readonly { value: StatusView; label: string; statuses: KnowledgeStatus[] }[] = [
  { value: 'current', label: 'Current', statuses: ['active', 'proposed'] },
  { value: 'proposed', label: 'Proposed', statuses: ['proposed'] },
  { value: 'archived', label: 'Archived', statuses: ['archived'] },
  { value: 'all', label: 'All', statuses: [] },
]
export interface KnowledgeFilters { q: string; type: KnowledgeType | ''; status: StatusView; sort: SortBy | '' }
const text = (value: unknown) => typeof value === 'string' ? value : ''
export function filtersFromQuery(query: Record<string, unknown>): KnowledgeFilters {
  const status = text(query.status)
  const sort = text(query.sort)
  return {
    q: text(query.q),
    type: isKnowledgeType(query.type) ? query.type : '',
    status: STATUS_VIEWS.some(view => view.value === status) ? status as StatusView : 'current',
    sort: ['updated', 'title', 'slug', 'relevance'].includes(sort) ? sort as SortBy : '',
  }
}
export function filtersToQuery(filters: KnowledgeFilters): Record<string, string> {
  const out: Record<string, string> = {}
  if (filters.q.trim()) out.q = filters.q.trim()
  if (filters.type) out.type = filters.type
  if (filters.status !== 'current') out.status = filters.status
  if (filters.sort) out.sort = filters.sort
  return out
}

export function useKnowledge(projectId: Ref<string | null>, filters: Ref<KnowledgeFilters>, active: Ref<boolean>) {
  const items = ref<KnowledgeItem[]>([])
  const loaded = ref(false)
  const loading = ref(false)
  const error = ref('')
  const truncated = ref(false)
  const hits = ref<KnowledgeItem[] | null>(null)
  const searched = ref('')
  const searching = ref(false)
  let loadedFor = ''
  let generation = 0
  let searchController: AbortController | null = null
  let timer: ReturnType<typeof setTimeout> | undefined

  async function load() {
    const id = projectId.value
    if (!id) return
    const request = ++generation
    loading.value = true; error.value = ''
    try {
      const page = await listKnowledge({ project_id: id, limit: 1000 })
      if (request !== generation) return
      items.value = page.items
      truncated.value = page.truncated
      loaded.value = true
      loadedFor = id
    } catch (e) {
      if (request === generation) error.value = e instanceof Error ? e.message : 'Knowledge could not be loaded.'
    } finally {
      if (request === generation) loading.value = false
    }
  }
  watch([projectId, active], ([id, on]) => {
    if (!on || !id) return
    if (id !== loadedFor) { items.value = []; loaded.value = false; hits.value = null; void load() }
  }, { immediate: true })

  async function search() {
    const id = projectId.value, q = filters.value.q.trim()
    searchController?.abort()
    if (!id || !q) { hits.value = null; searched.value = ''; searching.value = false; return }
    const controller = searchController = new AbortController()
    searching.value = true
    try {
      const page = await listKnowledge({ project_id: id, q, limit: 1000 }, controller.signal)
      if (controller.signal.aborted) return
      hits.value = page.items
      searched.value = q
    } catch (e) {
      if (controller.signal.aborted) return
      // A failed search falls back to the titles and slugs already here.
      const needle = q.toLowerCase()
      hits.value = items.value.filter(item => `${item.title} ${item.slug} ${item.key}`.toLowerCase().includes(needle))
      searched.value = q
      error.value = e instanceof Error ? `Search is not answering (${e.message}); showing title matches.` : ''
    } finally {
      if (!controller.signal.aborted) searching.value = false
    }
  }
  watch(() => [filters.value.q, projectId.value, active.value] as const, ([q, , on]) => {
    clearTimeout(timer)
    if (!on) return
    if (!q.trim()) { searchController?.abort(); hits.value = null; searched.value = ''; searching.value = false; return }
    searching.value = true
    timer = setTimeout(() => void search(), 180)
  }, { immediate: true })

  // While a search is on its way, the last answer (or the plain list) stays: no flashing.
  const source = computed(() => filters.value.q.trim() && hits.value ? hits.value : items.value)
  const statuses = computed(() => STATUS_VIEWS.find(view => view.value === filters.value.status)?.statuses ?? [])
  const types = computed<KnowledgeType[]>(() => filters.value.type ? [filters.value.type] : [])
  const sortBy = computed<SortBy>(() => filters.value.sort || (filters.value.q.trim() && hits.value ? 'relevance' : 'updated'))
  const visible = computed(() => filterItems(source.value, types.value, statuses.value))
  const groups = computed(() => groupItems(visible.value, sortBy.value))
  const sequence = computed(() => groups.value.flatMap(group => group.items))
  const typeCounts = computed(() => countBy(filterItems(source.value, [], statuses.value)).type)
  const statusCounts = computed(() => countBy(filterItems(source.value, types.value, [])).status)
  const total = computed(() => filterItems(source.value, [], statuses.value).length)
  const slugs = (type: KnowledgeType) => items.value.filter(item => item.type === type).map(item => item.slug)

  // After a write: the entry page hands back what the server returned.
  function upsert(item: KnowledgeItem) {
    const next = { ...item }
    const at = items.value.findIndex(existing => existing.id === item.id)
    items.value = at === -1 ? [next, ...items.value] : items.value.map(existing => existing.id === item.id ? next : existing)
    if (hits.value) hits.value = hits.value.map(existing => existing.id === item.id ? { ...next, excerpt: existing.excerpt } : existing)
  }
  function remove(id: string) {
    items.value = items.value.filter(item => item.id !== id)
    if (hits.value) hits.value = hits.value.filter(item => item.id !== id)
  }
  function stop() { clearTimeout(timer); searchController?.abort() }

  return { items, loaded, loading, error, truncated, searching, searched, visible, groups, sequence, typeCounts, statusCounts, total, sortBy, load, upsert, remove, slugs, stop }
}
export type KnowledgeState = ReturnType<typeof useKnowledge>
