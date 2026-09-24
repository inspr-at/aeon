<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { setPageTitle } from '../lib/brand'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { createNode, listNodes, type ListItem } from '../lib/api'
import { canWrite } from '../lib/activity'
import { confirmAction } from '../lib/confirm'
import { asListItem, guardedMove, keyPrefix, kinds } from '../lib/useTicket'
import { useOutline } from '../lib/useOutline'
import { density } from '../lib/prefs'
import { orderOf, type ColumnId, type ListPrefs } from '../lib/columns'
import { usePreference } from '../lib/preferences'
import { toast } from '../lib/toast'
import { command, consume, run } from '../lib/commands'
import { remember } from '../lib/recents'
import { apiParams, effectiveSort, facetOptions, filtersFromQuery, filtersToQuery, groupRows, hasFilters, orderByStatus, totalFrom, WORK_KINDS, type Dimension, type EpicRef, type GroupBy, type ListFilters } from '../lib/ticketList'
import { useTicketList } from '../lib/useTicketList'
import { absoluteTime, cycleSort, relativeTime, statusMeta, type SortField } from '../lib/work'
import { useProjects } from '../stores/projects'
import { useSession } from '../stores/session'
import AppIcon from '../components/AppIcon.vue'
import PanelSplitter from '../components/PanelSplitter.vue'
import FilterSheet from '../components/work/FilterSheet.vue'
import ListToolbar from '../components/work/ListToolbar.vue'
import StatusIcon from '../components/work/StatusIcon.vue'
import StatusMenu from '../components/work/StatusMenu.vue'
import TicketTable from '../components/work/TicketTable.vue'
import TicketWorkspace from '../components/work/TicketWorkspace.vue'
import JourneyChip from '../components/journey/JourneyChip.vue'
import JourneyView from '../components/journey/JourneyView.vue'
import type { Stage } from '../lib/journey'
import type { QuickDraft } from '../components/work/QuickCreateRow.vue'

const route = useRoute()
const router = useRouter()
const projects = useProjects()
const session = useSession()

const projectKey = computed(() => String(route.params.projectKey ?? ''))
const ticketKey = computed(() => typeof route.params.ticketKey === 'string' ? route.params.ticketKey : '')
const project = computed(() => projects.byRouteKey(projectKey.value))
const projectId = computed(() => project.value?.id ?? null)
const routeKey = computed(() => project.value?.routeKey ?? projectKey.value)

// ---------- Columns: the person's order, visibility and widths for this project ----------
const listPref = computed(() => projectId.value ? usePreference<ListPrefs>(`list:${projectId.value}`) : null)
const listPrefs = computed(() => listPref.value?.value.value ?? null)
const tableLayout = ref<{ visible: ColumnId[]; customised: boolean }>({ visible: [], customised: false })
const toolbarColumns = computed(() => ({ order: orderOf(listPrefs.value), visible: tableLayout.value.visible, customised: tableLayout.value.customised }))
function saveColumns(order: ColumnId[], visible: ColumnId[]) { listPref.value?.save({ ...(listPrefs.value ?? {}), order, visible }, 0) }
function resetColumns() { const { order: _order, visible: _visible, ...rest } = listPrefs.value ?? {}; listPref.value?.save(rest, 0) }
function saveWidths(widths: Partial<Record<ColumnId, number>>) { listPref.value?.save({ ...(listPrefs.value ?? {}), widths }) }
const filters = computed(() => filtersFromQuery(route.query))
const list = useTicketList(projectId, filters)
const now = ref(Date.now())
// List, Outline or Journey. The full page keeps whichever the ticket was opened from.
type ViewMode = 'list' | 'outline' | 'journey'
const modeOf = (view: unknown): ViewMode => view === 'outline' ? 'outline' : view === 'journey' ? 'journey' : 'list'
const fullViewQuery = computed(() => !!ticketKey.value && route.query.view === 'full')
const lastListMode = ref<ViewMode>(modeOf(route.query.view))
watch(() => route.query.view, view => { if (view !== 'full') lastListMode.value = modeOf(view) })
const viewMode = computed<ViewMode>(() => fullViewQuery.value ? lastListMode.value : modeOf(route.query.view))
const journeyActive = computed(() => viewMode.value === 'journey')
// The journey's own place: the stage looked at, a chosen release and the walker's ticket.
const JOURNEY_KEYS = ['stage', 'release', 'walk'] as const
const queryText = (value: unknown) => typeof value === 'string' && value ? value : null
const journeyStage = computed(() => queryText(route.query.stage))
const journeyRelease = computed(() => queryText(route.query.release))
const journeyWalk = computed(() => queryText(route.query.walk))
function journeyQuery(patch: Partial<Record<typeof JOURNEY_KEYS[number], string | null>> = {}) {
  const query: Record<string, string> = { view: 'journey' }
  for (const key of JOURNEY_KEYS) {
    const value = key in patch ? patch[key] : queryText(route.query[key])
    if (value) query[key] = value
  }
  return query
}
function journeyStageTo(stage: Stage) { void router.push({ path: `/p/${encodeURIComponent(routeKey.value)}`, query: journeyQuery({ stage, walk: null }) }) }
function journeyReleaseTo(key: string | null) { void router.replace({ path: route.path, query: journeyQuery({ release: key }) }) }
function journeyWalkTo(key: string | null, mode: 'open' | 'move' | 'close') {
  if (mode === 'open') { void router.push({ path: route.path, query: journeyQuery({ walk: key }) }); return }
  if (mode === 'move') { void router.replace({ path: route.path, query: journeyQuery({ walk: key }) }); return }
  if (typeof window.history.state?.back === 'string' && window.history.state.back.includes('view=journey') && !window.history.state.back.includes('walk=')) router.back()
  else void router.replace({ path: route.path, query: journeyQuery({ walk: null }) })
}
const outlineActive = computed(() => viewMode.value === 'outline')
const outline = useOutline(projectId, filters, outlineActive, list)

const toolbarWrap = ref<HTMLElement>()
const stickMark = ref<HTMLElement>()
const toolbar = ref<InstanceType<typeof ListToolbar>>()
const table = ref<InstanceType<typeof TicketTable>>()
const panel = ref<InstanceType<typeof TicketWorkspace>>()
const filterSheet = ref<InstanceType<typeof FilterSheet>>()
const scrollRoot = ref<HTMLElement | null>(null)
const toolbarHeight = ref(52)
const stuck = ref(false)
const cursorId = ref<string | null>(null)
const collapsed = ref(new Set<string>())
const statusMenu = ref<{ row: ListItem; anchor: HTMLElement; from: 'list' | 'panel' } | null>(null)
const creating = ref(false)
// Full page: the same ticket workspace in a two-column page instead of the side panel.
const fullView = fullViewQuery
const me = computed(() => session.identity ? { id: session.identity.principal.id, name: session.identity.principal.name } : null)
const writable = computed(() => canWrite(session.identity?.principal.roles))

// ---------- Rows, groups and keyboard order ----------
const displayRows = computed(() => {
  const primary = effectiveSort(filters.value)[0]
  return primary?.field === 'state' ? orderByStatus(list.rows.value, primary.desc) : list.rows.value
})
const rowsById = computed(() => new Map(list.rows.value.map(row => [row.id, row])))
const groups = computed(() => groupRows(displayRows.value, filters.value.group, list.facets.value.state))
const sequence = computed(() => {
  if (outlineActive.value) return outline.rows.value
  const out: ListItem[] = []
  for (const group of groups.value) {
    const epicRow = group.epic ? rowsById.value.get(group.epic.id) : undefined
    if (epicRow) out.push(epicRow)
    if (!collapsed.value.has(group.key)) out.push(...group.rows)
  }
  return out
})
const total = computed(() => totalFrom(list.facets.value))
const showAssignee = computed(() => (outlineActive.value ? outline.rows.value : list.rows.value).some(row => row.assignee))

// One source for the header: the project summary (work counts). It arrives with
// the project itself and refreshes after a status change.
const counts = computed(() => {
  const p = project.value
  return p ? { open: p.open, progress: p.in_progress, done: p.done, cancelled: p.cancelled, total: p.total, percent: p.percent } : null
})
// The skeleton holds about the height the first page will take, so the hint and
// footer below the table do not jump when rows arrive.
const expectedRows = computed(() => {
  const c = counts.value
  if (!c || filtered.value) return 8
  if (outlineActive.value) return 12
  return filters.value.showClosed ? c.total : c.open + c.progress
})
const knownStates = computed(() => Object.keys(list.facets.value.state ?? {}))
const filtered = computed(() => hasFilters(filters.value))

function options(dimension: Dimension) {
  return facetOptions(dimension, list.counts(dimension), filters.value[dimension], list.names, session.identity?.principal.id)
}

// ---------- URL state ----------
function update(patch: Partial<ListFilters>) {
  const view = route.query.view === 'outline' || route.query.view === 'full' || route.query.view === 'journey' ? { view: route.query.view } : {}
  void router.replace({ path: route.path, query: { ...filtersToQuery({ ...filters.value, ...patch }), ...view } })
}
function setView(mode: ViewMode) {
  if (mode === viewMode.value) return
  creating.value = false
  const { view: _view, stage: _stage, release: _release, walk: _walk, ...query } = route.query
  if (mode === 'journey') { void router.push({ path: `/p/${encodeURIComponent(routeKey.value)}`, query: { view: 'journey' } }); return }
  const path = viewMode.value === 'journey' ? `/p/${encodeURIComponent(routeKey.value)}` : route.path
  void router.replace({ path, query: mode === 'outline' ? { ...query, view: 'outline' } : query })
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
// The Outline without filters loads its own levels; the list query then only supplies
// counts. With filters or Hide closed, the Outline needs the list's whole match set.
const listLoadMode = computed(() => journeyActive.value ? 'counts' : !outlineActive.value ? 'list' : outline.matchMode.value ? 'all' : 'counts')
watch([queryKey, listLoadMode], async ([value, mode], old) => {
  if (!value) return
  // Switching views on the same query reuses the rows already loaded.
  const sameQuery = !!old && old[0] === value
  if (sameQuery && old![1] !== 'counts' && mode !== 'counts') { if (mode === 'all') void list.loadAll(); return }
  const load = list.load({ pageSize: mode === 'counts' ? 1 : 200 })
  if (mode === 'all') void load.then(() => list.loadAll())
  if (sameQuery) return
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
  return list.rows.value.find(row => row.key.toLowerCase() === key) ?? outline.findByKey(key) ?? (fetched.value?.key.toLowerCase() === key ? fetched.value : null)
})
const panelPosition = computed(() => {
  const item = panelItem.value
  if (!item) return null
  const index = sequence.value.findIndex(row => row.id === item.id)
  // While more pages exist, count the whole list (its total), not just what is loaded.
  const count = list.cursor.value && total.value ? Math.max(total.value, sequence.value.length) : sequence.value.length
  return index === -1 ? null : { index, count }
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
watch(panelItem, item => {
  if (!item) return
  if (outlineActive.value) outline.reveal(item.id)
  if (sequence.value.some(row => row.id === item.id)) {
    cursorId.value = item.id
    if (!fullView.value) void nextTick(() => table.value?.scrollToRow(item.id))
  }
})

// People who can be assigned: everyone assigned somewhere in this project, and you.
const projectPeople = ref<string[]>([])
watch(projectId, async id => {
  projectPeople.value = []
  if (!id) return
  try {
    const page = await listNodes({ within: id, kind: WORK_KINDS, facets: ['assignee'], limit: 1 })
    const ids = Object.keys(page.facets?.assignee ?? {}).filter(value => value !== 'none')
    await list.resolveNames(ids)
    if (projectId.value === id) projectPeople.value = ids
  } catch { /* the menu still offers you and Unassigned */ }
}, { immediate: true })
const people = computed(() => projectPeople.value.map(id => ({ id, name: list.names.get(id) ?? 'Someone' })))

let openedFromList = false
let openedQuery = ''
function ticketPath(key: string) { return `/p/${encodeURIComponent(routeKey.value)}/${encodeURIComponent(key)}` }
// List navigation (open from the list, j/k, next/previous) replaces the open ticket
// and clears the back trail; following a link inside the panel pushes a step.
function openKey(key: string) {
  const location = { path: ticketPath(key), query: route.query, state: { trail: [] } }
  if (ticketKey.value) { void router.replace(location); return }
  openedFromList = true
  openedQuery = JSON.stringify(route.query)
  void router.push(location)
}
function openRow(row: ListItem) { cursorId.value = row.id; openKey(row.key) }
function listQuery() {
  const { view, ...query } = route.query
  if (view === 'journey' || (view === 'full' && lastListMode.value === 'journey')) return journeyQuery()
  return view === 'outline' || (view === 'full' && lastListMode.value === 'outline') ? { ...query, view: 'outline' } : query
}
function closePanel() {
  if (!ticketKey.value) return
  const back = !fullView.value && openedFromList && JSON.stringify(route.query) === openedQuery && typeof window.history.state?.back === 'string'
  openedFromList = false
  // Back past every followed link to the list entry the panel was opened from.
  if (back) router.go(-(trail.value.length + 1))
  else void router.replace({ path: `/p/${encodeURIComponent(routeKey.value)}`, query: listQuery() })
  void nextTick(() => table.value?.focusGrid())
}
let expandedFromPanel = false
let listScroll = 0
function expand() {
  if (!ticketKey.value || fullView.value) return
  expandedFromPanel = true
  void router.push({ path: route.path, query: { ...route.query, view: 'full' } })
}
// The full page starts at its top; going back returns to the same place in the list.
watch(fullView, async (full, was) => {
  const root = scrollRoot.value
  if (!root) return
  if (full && !was) { listScroll = root.scrollTop; await nextTick(); root.scrollTop = 0 }
  else if (!full && was) { await nextTick(); root.scrollTop = listScroll; if (panelItem.value) table.value?.scrollToRow(panelItem.value.id) }
})
function collapse() {
  if (!fullView.value) return
  if (expandedFromPanel && typeof window.history.state?.back === 'string') router.back()
  else void router.replace({ path: route.path, query: listQuery() })
  expandedFromPanel = false
}
// ---------- Back trail: links followed inside the panel ----------
const trail = ref<string[]>([])
function readTrail() {
  const saved = window.history.state?.trail
  trail.value = Array.isArray(saved) ? saved.filter((key): key is string => typeof key === 'string') : []
}
watch(() => route.fullPath, readTrail, { immediate: true })
function follow(path: string) {
  const current = panelItem.value?.key ?? ticketKey.value.toUpperCase()
  void router.push({ path, query: route.query, state: { trail: [...trail.value, current] } })
}
function trailBack(steps = 1) { if (trail.value.length) router.go(-Math.min(steps, trail.value.length)) }
// Related tickets can live in another project: open them where they belong.
async function openRelated(key: string, newTabRequested = false) {
  if (newTabRequested) { newTab(key); return }
  const here = key.split('-')[0] === routeKey.value || list.rows.value.some(row => row.key === key)
  if (!here) {
    try {
      const page = await listNodes({ q: key, limit: 25 })
      const owner = page.items.find(item => item.key === key)?.project
      const target = owner ? projects.byId(owner.id) : undefined
      if (target && target.id !== projectId.value) { follow(`/p/${encodeURIComponent(target.routeKey)}/${encodeURIComponent(key)}`); return }
    } catch { /* fall back to this project, where the panel explains */ }
  }
  if (ticketKey.value) follow(ticketPath(key)); else openKey(key)
}
watch(ticketKey, key => { if (!key) openedFromList = false })

// ---------- Actions ----------
function copyKey(key: string) {
  navigator.clipboard.writeText(key).then(() => toast(`Copied ${key}`), () => toast(`${key} could not be copied`, { tone: 'error' }))
}
function newTab(key: string) {
  const project = projects.projects.find(p => key.startsWith(`${p.routeKey}-`))
  window.open(project ? `/p/${encodeURIComponent(project.routeKey)}/${encodeURIComponent(key)}` : ticketPath(key), '_blank', 'noopener')
}
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
  void list.setStatus(menu.row, state).then(changed => { if (changed) { void projects.load(true); outline.refreshStatsFor(menu.row.id) } })
  if (menu.from === 'panel') menu.anchor.focus()
  else table.value?.focusGrid()
}
function openEpic(epic: EpicRef) { openKey(epic.key) }

// ---------- Create and remove ----------
async function startCreate(under: ListItem | null = null) {
  if (fullView.value) collapse()
  if (under && outlineActive.value) {
    creating.value = false
    outline.startCreateUnder(under.id)
    await nextTick(); table.value?.focusCreate()
    return
  }
  outline.startCreateUnder(null)
  creating.value = true
  if (scrollRoot.value && toolbarWrap.value && scrollRoot.value.scrollTop > toolbarWrap.value.offsetTop) scrollRoot.value.scrollTop = toolbarWrap.value.offsetTop
  await nextTick(); table.value?.focusCreate()
}
async function quickCreate(draft: QuickDraft): Promise<boolean> {
  const current = project.value
  if (!current) return false
  try {
    const kind = (await kinds()).find(candidate => candidate.slug === draft.kind)
    if (!kind) throw new Error('this workspace has no such type')
    const node = await createNode({
      kind_id: kind.id, title: draft.title, state: draft.state, fields: draft.priority ? { priority: draft.priority } : {},
      parent_id: draft.epic?.id ?? current.id, key_prefix: keyPrefix(current.routeKey),
    })
    const parent = draft.epic ? { ...draft.epic, kind_slug: 'epic' } : { id: current.id, key: current.key, title: current.title, kind_slug: 'project' }
    const created = asListItem(node, kind, parent, { id: current.id, key: current.key, title: current.title })
    list.insertRow(created)
    outline.insert(created)
    cursorId.value = created.id
    toast(`Created ${node.key}`, { action: { label: 'Open', run: () => openKey(node.key) } })
    void projects.load(true)
    return true
  } catch (e) {
    toast(`The ticket was not created: ${e instanceof Error ? e.message : 'unknown error'}`, { tone: 'error' })
    return false
  }
}
function childCreated(item: ListItem) { list.insertRow(item); outline.insert(item); void projects.load(true) }
function childMoved(item: ListItem, fromParent: string | null) {
  outline.relocate(item, fromParent, fromParent && outline.node(fromParent)?.kind_slug === 'epic' ? fromParent : null)
}
function closeCreate() { creating.value = false; outline.startCreateUnder(null) }
// Drag and drop in the Outline: the same guarded move as the workspace's "Move to another epic".
async function moveRow(row: ListItem, epic: ListItem | null) {
  const current = project.value
  if (!current) return
  const parent = epic ? { id: epic.id, key: epic.key, title: epic.title, kind_slug: 'epic' } : { id: current.id, key: current.key, title: current.title, kind_slug: 'project' }
  if (await guardedMove(row, parent, childMoved) === 'ok') {
    if (epic) outline.setExpanded(epic.id, true)
    cursorId.value = row.id
  }
}
let skipGuard = false
function removed(item: ListItem) {
  outline.remove(item)
  list.removeRow(item.id)
  void projects.load(true)
  skipGuard = true
  void router.replace({ path: `/p/${encodeURIComponent(routeKey.value)}`, query: listQuery() }).finally(() => { skipGuard = false })
}

// Commands from the palette: New ticket in this project.
watch(command, value => {
  const next = value?.command
  if (next?.name !== 'new-ticket' || !project.value || next.projectKey !== project.value.routeKey) return
  consume()
  if (ticketKey.value) closePanel()
  void nextTick(() => startCreate())
}, { immediate: true })
// Recently opened work and projects, for the palette.
watch(panelItem, item => {
  if (item && project.value) remember({ type: 'ticket', key: item.key, title: item.title, state: item.state, kind: item.kind_slug, projectKey: project.value.routeKey })
})
watch(project, current => { if (current) remember({ type: 'project', key: current.routeKey, title: current.title }) }, { immediate: true })

// ---------- Unsaved changes ----------
function dirty() { return !!panel.value?.isDirty() }
async function confirmDiscard() {
  if (skipGuard || !dirty()) return true
  return confirmAction({ title: 'Discard unsaved changes?', body: 'You have edits in this ticket that are not saved yet.', confirmLabel: 'Discard changes', danger: true })
}
onBeforeRouteUpdate(async (to, from) => {
  if (to.params.ticketKey !== from.params.ticketKey || to.params.projectKey !== from.params.projectKey) return confirmDiscard()
})
onBeforeRouteLeave(async () => (await confirmDiscard()) && (!table.value?.createDirty() || skipGuard || confirmAction({ title: 'Discard the new ticket?', body: 'Its title has not been created yet.', confirmLabel: 'Discard', danger: true })))
function beforeUnload(event: BeforeUnloadEvent) { if (dirty() || table.value?.createDirty()) { event.preventDefault(); event.returnValue = '' } }
function setDensity(value: 'comfortable' | 'compact') { density.value = value }
// Tabbing into the table lands on a visible row, not on an invisible container.
function focusFirst() { if (!cursorId.value && sequence.value.length) cursorId.value = sequence.value[0].id }

// ---------- Keyboard ----------
function typing(target: EventTarget | null) {
  return target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
}
async function move(step: number) {
  let rows = sequence.value
  if (!rows.length) return
  // At the end of what is loaded, fetch the next page first so j and Next keep going.
  const at = rows.findIndex(row => row.id === (ticketKey.value && panelItem.value ? panelItem.value.id : cursorId.value))
  if (step > 0 && at === rows.length - 1 && list.cursor.value && !outlineActive.value) {
    await list.loadMore()
    rows = sequence.value
  }
  // With a ticket open, move from that ticket; the cursor follows only once the move happens
  // (an unsaved-changes guard may keep the current ticket open).
  const from = ticketKey.value && panelItem.value ? panelItem.value.id : cursorId.value
  const index = rows.findIndex(row => row.id === from)
  const next = index === -1 ? (step > 0 ? 0 : rows.length - 1) : Math.max(0, Math.min(rows.length - 1, index + step))
  const row = rows[next]
  if (ticketKey.value) {
    openKey(row.key)
    if (!fullView.value && !panel.value?.el?.contains(document.activeElement)) table.value?.focusGrid()
    return
  }
  cursorId.value = row.id
  void nextTick(() => table.value?.scrollToRow(row.id))
  table.value?.focusGrid()
}
function keydown(event: KeyboardEvent) {
  if (event.altKey && event.key === 'ArrowLeft' && ticketKey.value && trail.value.length && !typing(event.target as HTMLElement | null)) { event.preventDefault(); trailBack(); return }
  if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return
  if (document.querySelector('dialog[open]')) return
  const target = event.target as HTMLElement | null
  if (target?.closest?.('.floating') || document.querySelector('.floating')) return
  if (typing(target)) {
    if (event.key === 'ArrowDown' && target === toolbar.value?.input) { event.preventDefault(); target.blur(); void move(cursorId.value ? 0 : 1) }
    return
  }
  // The journey has its own keys; with a ticket open, the panel's keys still work.
  if (journeyActive.value && !ticketKey.value) return
  if (journeyActive.value && ['j', 'k', 'ArrowDown', 'ArrowUp', 'Enter', 'o', '/', 'n'].includes(event.key)) return
  const row = sequence.value.find(item => item.id === cursorId.value)
  if (outlineActive.value && !fullView.value && outlineKey(event, row)) return
  switch (event.key) {
    case 'j': case 'ArrowDown': event.preventDefault(); void move(1); break
    case 'k': case 'ArrowUp': event.preventDefault(); void move(-1); break
    case 'Enter': case 'o':
      if (event.key === 'Enter' && target?.closest('button, a, summary')) return
      if (row) { event.preventDefault(); openRow(row) }
      break
    case 'Escape':
      if (creating.value || outline.createUnder.value) { event.preventDefault(); closeCreate() }
      else if (ticketKey.value) { event.preventDefault(); closePanel() }
      break
    case '/': if (!fullView.value) { event.preventDefault(); toolbar.value?.focusSearch() } break
    case 'n': event.preventDefault(); void startCreate(outlineActive.value && !ticketKey.value && row?.kind_slug === 'epic' ? row : null); break
    case 'e': if (ticketKey.value) { event.preventDefault(); void panel.value?.startEdit() } break
    case 's':
      if (ticketKey.value) { event.preventDefault(); panel.value?.openStatus() }
      else if (row) { const anchor = document.querySelector<HTMLElement>(`#row-${row.id} .status-btn`); if (anchor) { event.preventDefault(); openStatus(row, anchor, 'list') } }
      break
    case 'p': if (ticketKey.value) { event.preventDefault(); panel.value?.openPriority() } break
    case 'a': if (ticketKey.value) { event.preventDefault(); panel.value?.openAssignee() } break
    case 'c': if (ticketKey.value) { event.preventDefault(); panel.value?.focusComposer() } break
    case 'f': if (ticketKey.value) { event.preventDefault(); if (fullView.value) collapse(); else expand() } break
  }
}

// Outline keys: right opens or steps into, left closes or steps out, Space toggles.
function outlineKey(event: KeyboardEvent, row: ListItem | undefined): boolean {
  if (!['ArrowRight', 'ArrowLeft', ' '].includes(event.key) || !row) return false
  if (ticketKey.value && panel.value?.el?.contains(document.activeElement)) return false
  event.preventDefault()
  const open = outline.isExpanded(row.id), children = outline.hasChildren(row.id)
  const focusRow = (id: string | null) => {
    if (!id || !sequence.value.some(item => item.id === id)) return
    cursorId.value = id
    void nextTick(() => table.value?.scrollToRow(id))
  }
  if (event.key === ' ') { if (children) outline.toggle(row.id) }
  else if (event.key === 'ArrowRight') {
    if (children && !open) outline.setExpanded(row.id, true)
    else if (children) { const index = sequence.value.findIndex(item => item.id === row.id); const next = sequence.value[index + 1]; if (next && next.parent_id === row.id) focusRow(next.id) }
  } else {
    if (children && open) outline.setExpanded(row.id, false)
    else focusRow(outline.parentOf(row.id))
  }
  table.value?.focusGrid()
  return true
}

// ---------- Layout: sticky toolbar height and stuck state ----------
let resize: ResizeObserver | undefined
let stick: IntersectionObserver | undefined
let clock: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  scrollRoot.value = document.getElementById('main')
  void projects.load()
  window.addEventListener('keydown', keydown)
  window.addEventListener('beforeunload', beforeUnload)
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
  window.removeEventListener('beforeunload', beforeUnload)
  clearInterval(clock)
  resize?.disconnect()
  stick?.disconnect()
  list.invalidate()
})

// ---------- Document title ----------
watch([project, panelItem], ([current, item]) => {
  if (!current) return
  setPageTitle(item ? `${item.key} ${item.title}` : `${current.routeKey} ${current.title}`)
}, { immediate: true })


</script>

<template>
  <section class="project-page" :class="{ 'panel-open': !!ticketKey && !fullView, 'full-view': fullView }" :style="{ '--toolbar-h': `${toolbarHeight}px` }" :aria-labelledby="project ? 'project-title' : undefined">
    <template v-if="project">
      <div v-show="!fullView" class="list-view">
      <header class="project-head">
        <div class="head-main">
          <div class="title-line">
            <span class="key-badge big">{{ project.routeKey }}</span>
            <h1 id="project-title">{{ project.title }}</h1>
            <span v-if="project.frozen" class="chip state-chip">Frozen</span>
            <span v-else-if="project.archived" class="chip state-chip">Archived</span>
          </div>
          <p class="description" :data-tip="project.description.length > 120 ? project.description : undefined">{{ project.description || 'No description yet.' }}</p>
          <JourneyChip :project-id="project.id" :active="journeyActive" @go="journeyActive ? journeyStageTo(journeyStage as Stage ?? 'inspire') : setView('journey')" />
        </div>
        <div v-if="counts" class="head-stats" :aria-label="`${counts.open} open, ${counts.progress} in progress, ${counts.done} done of ${counts.total}`">
          <div class="stat-line">
            <span class="stat"><StatusIcon state="new" :size="11" /><b>{{ counts.open.toLocaleString('en-GB') }}</b> open</span>
            <span class="stat"><StatusIcon state="in_progress" :size="11" /><b>{{ counts.progress.toLocaleString('en-GB') }}</b> in progress</span>
            <span class="stat"><StatusIcon state="done" :size="11" /><b>{{ counts.done.toLocaleString('en-GB') }}</b> done</span>
          </div>
          <div class="progress-line" :data-tip="`${counts.done.toLocaleString('en-GB')} of ${(counts.total - counts.cancelled).toLocaleString('en-GB')} done${counts.cancelled ? ` · ${counts.cancelled} cancelled` : ''}`">
            <span class="bar"><i :style="{ width: `${counts.percent}%` }" /></span>
            <span class="mono pct">{{ counts.percent }}%</span>
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
          @open-sheet="filterSheet?.open()" @need-names="list.resolveNames(options('assignee').map(o => o.value))" @create="startCreate()"
          :view="viewMode" @view="setView" @expand-all="outline.expandAll()" @collapse-all="outline.collapseAll()"
          :columns="toolbarColumns" @columns="saveColumns" @columns-reset="resetColumns"
        />
      </div>

      <JourneyView
        v-if="journeyActive" :project="{ id: project.id, routeKey: project.routeKey, title: project.title }" :stage="journeyStage" :release-key="journeyRelease" :walk-key="journeyWalk"
        :can-write="writable" :person="session.identity?.principal.kind !== 'agent'" :me="me?.id ?? null"
        @stage="journeyStageTo" @release="journeyReleaseTo" @walk="journeyWalkTo" @open="openKey"
      />
      <TicketTable
        v-else ref="table" :expected-rows="expectedRows" :groups="groups" :group="filters.group" :rows-by-id="rowsById" :cursor-id="cursorId" :open-id="panelItem?.id ?? null"
        :query="filters.q" :sort="filters.sort" :density="density"
        :loading="outlineActive ? outline.loading.value : list.loading.value" :loading-more="outlineActive ? outline.loadingMoreRoot.value : list.loadingMore.value"
        :error="outlineActive ? outline.error.value || list.error.value : list.error.value" :more-error="list.moreError.value"
        :has-more="outlineActive ? outline.hasMoreRoot.value : !!list.cursor.value" :filtered="filtered" :hiding-closed="!filters.showClosed"
        :collapsed="collapsed" :total="total" :project-key="routeKey" :scroll-root="scrollRoot" :now="now" :show-assignee="showAssignee"
        :creating="creating" :project-id="project.id" :known-states="knownStates" :create="quickCreate" @close-create="closeCreate"
        :outline="outlineActive ? outline.entries.value : null" :can-drag="outlineActive && writable" :prefs="listPrefs"
        @layout="(visible, customised) => tableLayout = { visible, customised }" @widths="saveWidths"
        @toggle-row="outline.toggle" @toggle-no-epic="outline.noEpicCollapsed.value = !outline.noEpicCollapsed.value"
        @more-children="id => id === project!.id ? outline.loadMoreRoot() : outline.loadChildren(id, true)" @move="moveRow"
        @open="openRow" @cursor="id => cursorId = id" @sort="sortBy" @status="(row, anchor) => openStatus(row, anchor, 'list')"
        @copy="row => copyKey(row.key)" @new-tab="row => newTab(row.key)" @toggle-group="toggleGroup" @open-epic="openEpic"
        @retry="outlineActive ? outline.reload() : list.load()" @more="outlineActive ? outline.loadMoreRoot() : list.loadMore()" @grid-focus="focusFirst" @clear-filters="clearFilters" @show-closed="update({ showClosed: true })"
      />

      <p v-if="!journeyActive" class="hint">
        <kbd class="keycap">j</kbd><kbd class="keycap">k</kbd> move · <kbd class="keycap"><AppIcon name="enter" /></kbd> open · <kbd class="keycap">/</kbd> search ·
        <button type="button" class="hint-link" @click="run({ name: 'shortcuts' })"><kbd class="keycap">?</kbd> all shortcuts</button>
      </p>
      </div>

      <PanelSplitter v-if="ticketKey && !fullView" />
      <TicketWorkspace
        v-if="ticketKey" ref="panel" :item="panelItem" :ticket-key="ticketKey.toUpperCase()" :resolving="panelLoading" :resolve-error="panelError"
        :position="panelPosition" :now="now" :mode="fullView ? 'full' : 'panel'" :project="{ id: project.id, routeKey: project.routeKey }"
        :names="list.names" :me="me" :can-write="writable" :people="people"
        @close="closePanel" @prev="move(-1)" @next="move(1)" @expand="expand" @collapse="collapse" @new-tab="newTab(panelItem?.key ?? ticketKey)"
        @status="anchor => panelItem && openStatus(panelItem, anchor, 'panel')" @open-key="openRelated" :trail="trail" @trail-back="trailBack" @removed="removed" @created="childCreated" @moved="childMoved" @retry="resolvePanel"
      />
      <StatusMenu v-if="statusMenu" :anchor="statusMenu.anchor" :current="statusMenu.row.state" :known-states="knownStates" :ticket-key="statusMenu.row.key" @choose="chooseStatus" @close="closeStatus" />
      <FilterSheet
        ref="filterSheet" :filters="filters" :options="options" :total="total" :view="viewMode === 'journey' ? 'list' : viewMode" @expand-all="outline.expandAll()" @collapse-all="outline.collapseAll()"
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
/* Lists use the full width; the gutter grows with the screen. */
.project-page { width: 100%; margin: 0; padding: 22px var(--gutter) 12px; }
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
.toolbar-wrap { position: sticky; top: 0; z-index: 5; margin: 0 calc(-1 * var(--gutter)); padding: 0 var(--gutter); container: toolbar / inline-size; }
.toolbar-wrap.stuck { background: var(--glass); box-shadow: 0 1px 0 var(--line), 0 12px 24px -20px rgba(16, 35, 39, .35); backdrop-filter: blur(18px) saturate(1.2); -webkit-backdrop-filter: blur(18px) saturate(1.2); }
.hint { display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 5px; padding: 16px 0 6px; font-size: 12px; color: var(--ink-3); }
/* The hint waits for the rows, like the footer, so it never jumps while they load. */
.project-page:has(.skeleton-body) .hint { visibility: hidden; }
.hint .keycap + .keycap { margin-left: 2px; }
.hint-link { display: inline-flex; align-items: center; gap: 5px; padding: 0; border: 0; background: transparent; color: var(--ink-3); font-size: 12px; }
.hint-link:hover { color: var(--teal-ink); }
.project-page.full-view { padding-top: 12px; }
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
