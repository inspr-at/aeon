<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listNodes, type ListItem } from '../lib/api'
import { density } from '../lib/prefs'
import { toast } from '../lib/toast'
import { apiParams, effectiveSort, facetOptions, filtersFromQuery, filtersToQuery, groupRows, hasFilters, orderByStatus, totalFrom, type Dimension, type EpicRef, type GroupBy, type ListFilters } from '../lib/ticketList'
import { useTicketList } from '../lib/useTicketList'
import { absoluteTime, cycleSort, relativeTime, stateBuckets, statusMeta, type SortField } from '../lib/work'
import { useProjects } from '../stores/projects'
import { useSession } from '../stores/session'
import AppIcon from '../components/AppIcon.vue'
import FilterSheet from '../components/work/FilterSheet.vue'
import ListToolbar from '../components/work/ListToolbar.vue'
import ShortcutSheet from '../components/work/ShortcutSheet.vue'
import StatusIcon from '../components/work/StatusIcon.vue'
import StatusMenu from '../components/work/StatusMenu.vue'
import TicketPanel from '../components/work/TicketPanel.vue'
import TicketTable from '../components/work/TicketTable.vue'

const route = useRoute()
const router = useRouter()
const projects = useProjects()
const session = useSession()

const projectKey = computed(() => String(route.params.projectKey ?? ''))
const ticketKey = computed(() => typeof route.params.ticketKey === 'string' ? route.params.ticketKey : '')
const project = computed(() => projects.byRouteKey(projectKey.value))
const projectId = computed(() => project.value?.id ?? null)
const routeKey = computed(() => project.value?.routeKey ?? projectKey.value)
const filters = computed(() => filtersFromQuery(route.query))
const list = useTicketList(projectId, filters)
const now = ref(Date.now())

const toolbarWrap = ref<HTMLElement>()
const stickMark = ref<HTMLElement>()
const toolbar = ref<InstanceType<typeof ListToolbar>>()
const table = ref<InstanceType<typeof TicketTable>>()
const panel = ref<InstanceType<typeof TicketPanel>>()
const shortcuts = ref<InstanceType<typeof ShortcutSheet>>()
const filterSheet = ref<InstanceType<typeof FilterSheet>>()
const scrollRoot = ref<HTMLElement | null>(null)
const toolbarHeight = ref(52)
const stuck = ref(false)
const cursorId = ref<string | null>(null)
const collapsed = ref(new Set<string>())
const statusMenu = ref<{ row: ListItem; anchor: HTMLElement; from: 'list' | 'panel' } | null>(null)

// ---------- Rows, groups and keyboard order ----------
const displayRows = computed(() => {
  const primary = effectiveSort(filters.value)[0]
  return primary?.field === 'state' ? orderByStatus(list.rows.value, primary.desc) : list.rows.value
})
const rowsById = computed(() => new Map(list.rows.value.map(row => [row.id, row])))
const groups = computed(() => groupRows(displayRows.value, filters.value.group, list.facets.value.state))
const sequence = computed(() => {
  const out: ListItem[] = []
  for (const group of groups.value) {
    const epicRow = group.epic ? rowsById.value.get(group.epic.id) : undefined
    if (epicRow) out.push(epicRow)
    if (!collapsed.value.has(group.key)) out.push(...group.rows)
  }
  return out
})
const total = computed(() => totalFrom(list.facets.value))
const showAssignee = computed(() => list.rows.value.some(row => row.assignee))

// Header counts come from the project's own state facet, so every spelling of a
// status lands in the right bucket whatever the summary endpoint reports.
const stateCounts = ref<Record<string, number> | null>(null)
let countGeneration = 0
async function loadCounts() {
  const within = projectId.value
  if (!within) return
  const request = ++countGeneration
  try {
    const page = await listNodes({ within, facets: ['state'], limit: 1 })
    if (request === countGeneration) stateCounts.value = page.facets?.state ?? null
  } catch { /* the summary counts stay in place */ }
}
watch(projectId, () => { stateCounts.value = null; void loadCounts() }, { immediate: true })
const counts = computed(() => {
  if (stateCounts.value) return stateBuckets(stateCounts.value)
  const p = project.value
  return p ? { open: p.open, progress: p.in_progress, done: p.done, total: p.total } : null
})
const knownStates = computed(() => Object.keys(list.facets.value.state ?? {}))
const filtered = computed(() => hasFilters(filters.value))

function options(dimension: Dimension) {
  return facetOptions(dimension, list.counts(dimension), filters.value[dimension], list.names, session.identity?.principal.id)
}

// ---------- URL state ----------
function update(patch: Partial<ListFilters>) {
  void router.replace({ path: route.path, query: filtersToQuery({ ...filters.value, ...patch }) })
}
function toggleValue(dimension: Dimension, value: string) {
  const current = filters.value[dimension]
  const adding = !current.includes(value)
  const patch: Partial<ListFilters> = { [dimension]: adding ? [...current, value] : current.filter(item => item !== value) }
  // Choosing a closed status while closed tickets are hidden would show nothing.
  if (dimension === 'status' && adding && statusMeta(value).closed && !filters.value.showClosed) patch.showClosed = true
  update(patch)
}
function clearFilters() { update({ q: '', status: [], priority: [], assignee: [], type: [] }) }
function sortBy(field: SortField, additive: boolean) { update({ sort: cycleSort(filters.value.sort, field, additive) }) }
function setGroup(group: GroupBy) { collapsed.value = new Set(); update({ group }) }
function toggleGroup(key: string) {
  const next = new Set(collapsed.value)
  if (next.has(key)) next.delete(key); else next.add(key)
  collapsed.value = next
}

const queryKey = computed(() => projectId.value ? JSON.stringify(apiParams(projectId.value, filters.value)) : '')
watch(queryKey, (value, old) => {
  if (!value) return
  void list.load()
  // A new query starts at its first row, with the project header still out of view.
  if (old && scrollRoot.value && toolbarWrap.value && scrollRoot.value.scrollTop > toolbarWrap.value.offsetTop) {
    scrollRoot.value.scrollTop = toolbarWrap.value.offsetTop
  }
}, { immediate: true })

// ---------- Side panel ----------
const fetched = ref<ListItem | null>(null)
const panelError = ref('')
const panelLoading = ref(false)
let panelGeneration = 0
const panelItem = computed(() => {
  const key = ticketKey.value.toLowerCase()
  if (!key) return null
  return list.rows.value.find(row => row.key.toLowerCase() === key) ?? (fetched.value?.key.toLowerCase() === key ? fetched.value : null)
})
const panelPosition = computed(() => {
  const item = panelItem.value
  if (!item) return null
  const index = sequence.value.findIndex(row => row.id === item.id)
  return index === -1 ? null : { index, count: sequence.value.length }
})
async function resolvePanel() {
  const key = ticketKey.value
  const within = projectId.value
  panelError.value = ''
  if (!key || !within) return
  if (list.rows.value.some(row => row.key.toLowerCase() === key.toLowerCase()) || fetched.value?.key.toLowerCase() === key.toLowerCase()) return
  const request = ++panelGeneration
  panelLoading.value = true
  try {
    const result = await listNodes({ within, q: key, sort: 'key', limit: 50 })
    if (request !== panelGeneration) return
    const hit = result.items.find(item => item.key.toLowerCase() === key.toLowerCase())
    if (hit) fetched.value = hit
    else if (!list.rows.value.some(row => row.key.toLowerCase() === key.toLowerCase())) panelError.value = `${key.toUpperCase()} is not part of ${project.value?.title ?? 'this project'}.`
  } catch (e) {
    if (request === panelGeneration) panelError.value = e instanceof Error ? e.message : 'The ticket could not be loaded.'
  } finally {
    if (request === panelGeneration) panelLoading.value = false
  }
}
watch([ticketKey, projectId], resolvePanel, { immediate: true })
watch(panelItem, (item, old) => {
  if (!item) return
  if (sequence.value.some(row => row.id === item.id)) {
    cursorId.value = item.id
    void nextTick(() => table.value?.scrollToRow(item.id))
  }
  if (item.id !== old?.id) panel.value?.resetScroll()
})

let openedFromList = false
let openedQuery = ''
function ticketPath(key: string) { return `/p/${encodeURIComponent(routeKey.value)}/${encodeURIComponent(key)}` }
function openKey(key: string) {
  const location = { path: ticketPath(key), query: route.query }
  if (ticketKey.value) { void router.replace(location); return }
  openedFromList = true
  openedQuery = JSON.stringify(route.query)
  void router.push(location)
}
function openRow(row: ListItem) { cursorId.value = row.id; openKey(row.key) }
function closePanel() {
  if (!ticketKey.value) return
  const back = openedFromList && JSON.stringify(route.query) === openedQuery && typeof window.history.state?.back === 'string'
  openedFromList = false
  if (back) router.back()
  else void router.replace({ path: `/p/${encodeURIComponent(routeKey.value)}`, query: route.query })
  void nextTick(() => table.value?.focusGrid())
}
watch(ticketKey, key => { if (!key) openedFromList = false })

// ---------- Actions ----------
function copyKey(key: string) {
  navigator.clipboard.writeText(key).then(() => toast(`Copied ${key}`), () => toast(`${key} could not be copied`, { tone: 'error' }))
}
function newTab(key: string) { window.open(ticketPath(key), '_blank', 'noopener') }
function openStatus(row: ListItem, anchor: HTMLElement, from: 'list' | 'panel') {
  statusMenu.value = statusMenu.value?.row.id === row.id && statusMenu.value.from === from ? null : { row, anchor, from }
}
function closeStatus(restore: boolean) {
  const menu = statusMenu.value
  statusMenu.value = null
  if (restore) menu?.anchor.focus()
}
function chooseStatus(state: string) {
  const menu = statusMenu.value
  statusMenu.value = null
  if (!menu) return
  void list.setStatus(menu.row, state).then(loadCounts)
  if (menu.from === 'panel') menu.anchor.focus()
  else table.value?.focusGrid()
}
function openEpic(epic: EpicRef) { openKey(epic.key) }
function setDensity(value: 'comfortable' | 'compact') { density.value = value }
// Tabbing into the table lands on a visible row, not on an invisible container.
function focusFirst() { if (!cursorId.value && sequence.value.length) cursorId.value = sequence.value[0].id }

// ---------- Keyboard ----------
function typing(target: EventTarget | null) {
  return target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
}
function move(step: number) {
  const rows = sequence.value
  if (!rows.length) return
  const index = rows.findIndex(row => row.id === cursorId.value)
  const next = index === -1 ? (step > 0 ? 0 : rows.length - 1) : Math.max(0, Math.min(rows.length - 1, index + step))
  const row = rows[next]
  cursorId.value = row.id
  void nextTick(() => table.value?.scrollToRow(row.id))
  if (ticketKey.value) openKey(row.key)
  if (!panel.value?.el?.contains(document.activeElement)) table.value?.focusGrid()
}
function keydown(event: KeyboardEvent) {
  if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return
  if (document.querySelector('dialog[open]')) return
  const target = event.target as HTMLElement | null
  if (target?.closest?.('.floating') || document.querySelector('.floating')) return
  if (typing(target)) {
    if (event.key === 'ArrowDown' && target === toolbar.value?.input) { event.preventDefault(); target.blur(); move(cursorId.value ? 0 : 1) }
    return
  }
  const row = sequence.value.find(item => item.id === cursorId.value)
  switch (event.key) {
    case 'j': case 'ArrowDown': event.preventDefault(); move(1); break
    case 'k': case 'ArrowUp': event.preventDefault(); move(-1); break
    case 'Enter': case 'o':
      if (event.key === 'Enter' && target?.closest('button, a, summary')) return
      if (row) { event.preventDefault(); openRow(row) }
      break
    case 'Escape':
      if (ticketKey.value) { event.preventDefault(); closePanel() }
      break
    case '/': event.preventDefault(); toolbar.value?.focusSearch(); break
    case '?': event.preventDefault(); shortcuts.value?.open(); break
  }
}

// ---------- Layout: sticky toolbar height and stuck state ----------
let resize: ResizeObserver | undefined
let stick: IntersectionObserver | undefined
let clock: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  scrollRoot.value = document.getElementById('main')
  void projects.load()
  window.addEventListener('keydown', keydown)
  clock = setInterval(() => { now.value = Date.now() }, 60_000)
})
// The toolbar only exists once the project is known, so observe it when it appears.
watch(toolbarWrap, element => {
  resize?.disconnect()
  if (!element) return
  resize = new ResizeObserver(() => { toolbarHeight.value = element.offsetHeight })
  resize.observe(element)
}, { flush: 'post' })
watch([stickMark, scrollRoot], ([element, root]) => {
  stick?.disconnect()
  if (!element || !root) return
  stick = new IntersectionObserver(([entry]) => { stuck.value = !entry.isIntersecting }, { root, threshold: 0 })
  stick.observe(element)
}, { flush: 'post' })
onBeforeUnmount(() => {
  window.removeEventListener('keydown', keydown)
  clearInterval(clock)
  resize?.disconnect()
  stick?.disconnect()
  list.invalidate()
})

// ---------- Document title ----------
watch([project, panelItem], ([current, item]) => {
  if (!current) return
  document.title = item ? `${item.key} ${item.title} · PAIMOS AEON` : `${current.routeKey} ${current.title} · PAIMOS AEON`
}, { immediate: true })

const progress = computed(() => counts.value && counts.value.total ? Math.round((counts.value.done / counts.value.total) * 100) : 0)
</script>

<template>
  <section class="project-page" :class="{ 'panel-open': !!ticketKey }" :style="{ '--toolbar-h': `${toolbarHeight}px` }" :aria-labelledby="project ? 'project-title' : undefined">
    <template v-if="project">
      <header class="project-head">
        <div class="head-main">
          <div class="title-line">
            <span class="key-badge big">{{ project.routeKey }}</span>
            <h1 id="project-title">{{ project.title }}</h1>
            <span v-if="project.frozen" class="chip state-chip">Frozen</span>
            <span v-else-if="project.archived" class="chip state-chip">Archived</span>
          </div>
          <p class="description" :data-tip="project.description.length > 120 ? project.description : undefined">{{ project.description || 'No description yet.' }}</p>
        </div>
        <div v-if="counts" class="head-stats" :aria-label="`${counts.open} open, ${counts.progress} in progress, ${counts.done} done of ${counts.total}`">
          <div class="stat-line">
            <span class="stat"><StatusIcon state="new" :size="11" /><b>{{ counts.open.toLocaleString('en-GB') }}</b> open</span>
            <span class="stat"><StatusIcon state="in_progress" :size="11" /><b>{{ counts.progress.toLocaleString('en-GB') }}</b> in progress</span>
            <span class="stat"><StatusIcon state="done" :size="11" /><b>{{ counts.done.toLocaleString('en-GB') }}</b> done</span>
          </div>
          <div class="progress-line">
            <span class="bar"><i :style="{ width: `${progress}%` }" /></span>
            <span class="mono pct">{{ progress }}%</span>
          </div>
          <p class="activity">Active <time :datetime="project.last_activity" :data-tip="absoluteTime(project.last_activity)">{{ relativeTime(project.last_activity, { now, long: true }) }}</time></p>
        </div>
      </header>

      <div ref="stickMark" class="stick-mark" aria-hidden="true" />
      <div ref="toolbarWrap" class="toolbar-wrap" :class="{ stuck }">
        <ListToolbar
          ref="toolbar" :filters="filters" :options="options" :total="total" :loading="list.loading.value" :density="density" :stuck="stuck"
          @search="q => update({ q })" @toggle="toggleValue" @clear="dimension => update({ [dimension]: [] })" @clear-all="clearFilters"
          @show-closed="value => update({ showClosed: value })" @group="setGroup" @density="setDensity"
          @open-sheet="filterSheet?.open()" @need-names="list.resolveNames(options('assignee').map(o => o.value))"
        />
      </div>

      <TicketTable
        ref="table" :groups="groups" :group="filters.group" :rows-by-id="rowsById" :cursor-id="cursorId" :open-id="panelItem?.id ?? null"
        :query="filters.q" :sort="filters.sort" :density="density" :loading="list.loading.value" :loading-more="list.loadingMore.value"
        :error="list.error.value" :more-error="list.moreError.value" :has-more="!!list.cursor.value" :filtered="filtered" :hiding-closed="!filters.showClosed"
        :collapsed="collapsed" :total="total" :project-key="routeKey" :scroll-root="scrollRoot" :now="now" :show-assignee="showAssignee"
        @open="openRow" @cursor="id => cursorId = id" @sort="sortBy" @status="(row, anchor) => openStatus(row, anchor, 'list')"
        @copy="row => copyKey(row.key)" @new-tab="row => newTab(row.key)" @toggle-group="toggleGroup" @open-epic="openEpic"
        @retry="list.load()" @more="list.loadMore()" @grid-focus="focusFirst" @clear-filters="clearFilters" @show-closed="update({ showClosed: true })"
      />

      <p class="hint">
        <kbd class="keycap">j</kbd><kbd class="keycap">k</kbd> move · <kbd class="keycap"><AppIcon name="enter" /></kbd> open · <kbd class="keycap">/</kbd> search ·
        <button type="button" class="hint-link" @click="shortcuts?.open()"><kbd class="keycap">?</kbd> all shortcuts</button>
      </p>

      <TicketPanel
        v-if="ticketKey" ref="panel" :item="panelItem" :ticket-key="ticketKey.toUpperCase()" :loading="panelLoading" :error="panelError" :position="panelPosition" :now="now"
        @close="closePanel" @prev="move(-1)" @next="move(1)" @new-tab="newTab(panelItem?.key ?? ticketKey)" @copy="copyKey(panelItem?.key ?? ticketKey.toUpperCase())"
        @status="anchor => panelItem && openStatus(panelItem, anchor, 'panel')" @open-parent="openKey" @retry="resolvePanel"
      />
      <StatusMenu v-if="statusMenu" :anchor="statusMenu.anchor" :current="statusMenu.row.state" :known-states="knownStates" :ticket-key="statusMenu.row.key" @choose="chooseStatus" @close="closeStatus" />
      <ShortcutSheet ref="shortcuts" />
      <FilterSheet
        ref="filterSheet" :filters="filters" :options="options" :total="total"
        @toggle="toggleValue" @clear-all="clearFilters" @show-closed="value => update({ showClosed: value })" @group="setGroup"
        @opened="list.resolveNames(options('assignee').map(o => o.value))"
      />
    </template>

    <div v-else-if="projects.error && !projects.loaded" class="page-state" role="alert">
      <AppIcon name="alert" :size="20" />
      <h1>Projects could not be loaded</h1>
      <p>{{ projects.error }}</p>
      <button type="button" class="btn" @click="projects.load(true)">Try again</button>
    </div>
    <div v-else-if="projects.loaded" class="page-state">
      <AppIcon name="folder" :size="20" />
      <h1>No project called {{ projectKey.toUpperCase() }}</h1>
      <p>It may have been renamed or archived.</p>
      <RouterLink class="btn" to="/"><AppIcon name="arrow" :size="14" />All projects</RouterLink>
    </div>
    <div v-else class="head-skeleton" role="status" aria-label="Loading project">
      <span class="skeleton sk-a" /><span class="skeleton sk-b" /><span class="skeleton sk-c" />
    </div>
  </section>
</template>

<style scoped>
.project-page { width: min(1480px, 100%); margin: 0 auto; padding: 22px 28px 12px; }
.project-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 32px; padding: 4px 0 14px; }
.head-main { min-width: 0; flex: 1; }
.title-line { display: flex; align-items: center; gap: 12px; min-width: 0; }
.key-badge.big { height: 26px; padding: 0 10px; font-size: 12px; border-radius: 7px; }
.title-line h1 { font-size: 30px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.state-chip { height: 20px; font-size: 10px; text-transform: uppercase; letter-spacing: .08em; }
.description { margin-top: 6px; max-width: 820px; font-size: 13.5px; color: var(--ink-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.head-stats { display: grid; justify-items: end; gap: 7px; flex-shrink: 0; }
.stat-line { display: flex; gap: 16px; font-size: 12.5px; color: var(--ink-2); }
.stat { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
.stat b { font: 600 13px/1 var(--mono); color: var(--ink); font-variant-numeric: tabular-nums; font-variant-ligatures: none; }
.progress-line { display: flex; align-items: center; gap: 10px; width: 280px; }
.progress-line .bar { flex: 1; }
.pct { width: 34px; font-size: 12px; color: var(--ink-2); text-align: right; }
.activity { font-size: 12px; color: var(--ink-3); }
.activity time { color: var(--ink-2); }
.stick-mark { height: 1px; margin-bottom: -1px; }
.toolbar-wrap { position: sticky; top: 0; z-index: 5; margin: 0 -28px; padding: 0 28px; container: toolbar / inline-size; }
.toolbar-wrap.stuck { background: var(--glass); box-shadow: 0 1px 0 var(--line), 0 12px 24px -20px rgba(16, 35, 39, .35); backdrop-filter: blur(18px) saturate(1.2); -webkit-backdrop-filter: blur(18px) saturate(1.2); }
.hint { display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 5px; padding: 16px 0 6px; font-size: 12px; color: var(--ink-3); }
.hint .keycap + .keycap { margin-left: 2px; }
.hint-link { display: inline-flex; align-items: center; gap: 5px; padding: 0; border: 0; background: transparent; color: var(--ink-3); font-size: 12px; }
.hint-link:hover { color: var(--teal-ink); }
/* Wide screens dock the ticket panel: the list reflows beside it instead of under it. */
@media (min-width: 1100px) {
  .project-page.panel-open { width: 100%; margin: 0; padding-right: calc(var(--panel-w) + 22px); }
  .project-page.panel-open .toolbar-wrap { margin-right: 0; padding-right: 0; }
  .project-page.panel-open .description { max-width: 100%; }
}
.page-state { display: grid; justify-items: center; gap: 10px; padding: 96px 24px; text-align: center; }
.page-state > svg { color: var(--teal); }
.page-state h1 { font-size: 26px; }
.page-state .btn { margin-top: 8px; }
.head-skeleton { display: grid; gap: 12px; padding: 12px 0; }
.sk-a { width: 320px; height: 26px; border-radius: 8px; } .sk-b { width: 520px; } .sk-c { width: 100%; height: 44px; border-radius: 12px; margin-top: 18px; }
@media (max-width: 1080px) { .progress-line { width: 200px; } .stat-line { gap: 12px; } }
@media (max-width: 900px) {
  .project-head { flex-direction: column; align-items: stretch; gap: 12px; }
  .head-stats { justify-items: start; }
  .progress-line { width: 100%; }
}
@media (max-width: 720px) {
  .project-page { padding: 14px 12px 8px; }
  .toolbar-wrap { margin: 0 -12px; padding: 0 12px; }
  .title-line { gap: 10px; }
  .title-line h1 { font-size: 24px; }
  .description { white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
  .stat-line { flex-wrap: wrap; gap: 4px 14px; }
  .activity { display: none; }
  .hint { display: none; }
}
</style>
