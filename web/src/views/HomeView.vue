<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useSession } from '../stores/session'
import { createNode, getKinds, getNodes, getTree, getViews, subscribeWorkspace, type Kind, type NodeFilters, type SavedView, type SortField, type TreeEntry, type WorkNode } from '../lib/api'
import AppIcon from '../components/AppIcon.vue'
import NodeSidebar from '../components/NodeSidebar.vue'
import SearchPalette from '../components/SearchPalette.vue'
import SavedViews from '../components/SavedViews.vue'
import '../styles/workspace.css'

const session = useSession()
const mode = ref<'tree' | 'list'>('tree')
const kinds = ref<Kind[]>([])
const views = ref<SavedView[]>([])
const entries = ref<TreeEntry[]>([])
const nodes = ref<WorkNode[]>([])
const cursor = ref<string | null>(null)
const filters = reactive({ kind_id: '', state: '', parent_id: '', include_descendants: false })
const sort = ref<SortField>('position')
const direction = ref<'asc' | 'desc'>('asc')
const columns = ref(['key', 'kind', 'state'])
const selected = ref('')
const collapsed = ref(new Set<string>())
const busy = ref(false)
const error = ref('')
const metadataError = ref('')
const live = ref(false)
const sidebar = ref<InstanceType<typeof NodeSidebar>>()
const palette = ref<InstanceType<typeof SearchPalette>>()
const createDialog = ref<HTMLDialogElement>()
const createTitle = ref<HTMLInputElement>()
const newNode = reactive({ title: '', kind_id: '', parent_id: '', body: '', fields: '{}' })
const creating = ref(false)
const createError = ref('')
let loadedPages = 1
let generation = 0
let metadataGeneration = 0
let stopStream: (() => void) | undefined
let refreshTimer: ReturnType<typeof setTimeout> | undefined
let lastFocus: HTMLElement | null = null
const configuration = computed(() => ({ filters: filterValues(), sort: { field: sort.value, direction: direction.value }, columns: [...columns.value] }))
function filterValues(): NodeFilters {
  return { kind_id: filters.kind_id || undefined, state: filters.state.trim() || undefined, parent_id: filters.parent_id.trim() || undefined, include_descendants: !!filters.parent_id && filters.include_descendants }
}
const visibleEntries = computed(() => {
  let hiddenBelow: number | null = null
  return entries.value.filter(entry => {
    if (hiddenBelow !== null && entry.depth > hiddenBelow) return false
    hiddenBelow = collapsed.value.has(entry.node.id) ? entry.depth : null
    return true
  })
})
const rows = computed(() => mode.value === 'tree' ? visibleEntries.value : nodes.value.map(node => ({ node, depth: 0 })))
const allowedKinds = computed(() => {
  const parent = entries.value.find(entry => entry.node.id === newNode.parent_id)?.node ?? nodes.value.find(node => node.id === newNode.parent_id)
  const allowed = kinds.value.find(kind => kind.id === parent?.kind_id)?.allowed_child_kinds
  return allowed == null ? kinds.value : kinds.value.filter(kind => allowed.includes(kind.slug))
})
function kindLabel(id: string) { return kinds.value.find(kind => kind.id === id)?.label ?? 'Unknown kind' }
function toggle(id: string) { if (collapsed.value.has(id)) collapsed.value.delete(id); else collapsed.value.add(id) }
async function metadata() {
  const request = ++metadataGeneration
  const results = await Promise.allSettled([getKinds(), getViews()])
  if (request !== metadataGeneration) return
  const [kindResult, viewResult] = results
  if (kindResult.status === 'fulfilled') kinds.value = kindResult.value.items
  if (viewResult.status === 'fulfilled') views.value = viewResult.value.items
  metadataError.value = results.some(result => result.status === 'rejected') ? 'Kinds or saved views could not be loaded.' : ''
}
async function load(more = false) {
  const request = ++generation
  const currentMode = mode.value
  busy.value = true; error.value = ''
  const next = more ? cursor.value ?? undefined : undefined
  try {
    const pageCount = more ? 1 : loadedPages
    if (currentMode === 'tree') {
      let items: TreeEntry[] = [], nextCursor = next
      let resultCursor: string | null = null
      for (let pageIndex = 0; pageIndex < pageCount; pageIndex++) {
        const page = await getTree({ cursor: nextCursor })
        if (request !== generation) return
        items = [...items, ...page.items]; resultCursor = page.next_cursor
        if (!resultCursor) break
        nextCursor = resultCursor
      }
      entries.value = more ? [...entries.value, ...items] : items
      cursor.value = resultCursor
    } else {
      let items: WorkNode[] = [], nextCursor = next
      let resultCursor: string | null = null
      const params = { ...filterValues(), sort: sort.value, direction: direction.value }
      for (let pageIndex = 0; pageIndex < pageCount; pageIndex++) {
        const page = await getNodes({ ...params, cursor: nextCursor })
        if (request !== generation) return
        items = [...items, ...page.items]; resultCursor = page.next_cursor
        if (!resultCursor) break
        nextCursor = resultCursor
      }
      nodes.value = more ? [...nodes.value, ...items] : items
      cursor.value = resultCursor
    }
    if (more) loadedPages++
  } catch (e) { if (request === generation) error.value = e instanceof Error ? e.message : 'Could not load work' }
  finally { if (request === generation) busy.value = false }
}
function changed() {
  if (refreshTimer !== undefined) return
  refreshTimer = setTimeout(() => { refreshTimer = undefined; void load(); void metadata(); void sidebar.value?.refresh(); palette.value?.refresh() }, 150)
}
async function select(id: string) {
  if (id === selected.value) return
  if (sidebar.value && !sidebar.value.canLeave()) return
  lastFocus = document.activeElement as HTMLElement
  selected.value = id
  await nextTick()
  document.querySelector<HTMLButtonElement>('[aria-label="Close node details"]')?.focus()
}
async function closeDetails() { selected.value = ''; await nextTick(); lastFocus?.focus() }
function applyView(view: SavedView) {
  Object.assign(filters, { kind_id: '', state: '', parent_id: '', include_descendants: false }, view.filters)
  sort.value = view.sort.field; direction.value = view.sort.direction; columns.value = [...view.columns]
  mode.value = 'list'
}
async function openCreate() {
  newNode.title = ''; newNode.body = ''; newNode.fields = '{}'; newNode.parent_id = mode.value === 'tree' ? selected.value : filters.parent_id
  newNode.kind_id = allowedKinds.value[0]?.id ?? ''
  createError.value = ''
  createDialog.value?.showModal()
  await nextTick(); createTitle.value?.focus()
}
function closeCreate() { if (!creating.value) createDialog.value?.close() }
async function create() {
  creating.value = true; createError.value = ''
  try {
    const fields: unknown = JSON.parse(newNode.fields)
    if (!fields || typeof fields !== 'object' || Array.isArray(fields)) throw new Error('Fields must be a JSON object.')
    const value = await createNode({ title: newNode.title.trim(), kind_id: newNode.kind_id, body: newNode.body, parent_id: newNode.parent_id || null, fields: fields as Record<string, unknown> })
    createDialog.value?.close()
    await load(); await select(value.id)
  } catch (e) { createError.value = e instanceof Error ? e.message : 'Could not create node' }
  finally { creating.value = false }
}
function shortcut(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); palette.value?.open() }
}
watch([mode, filters, sort, direction], () => { loadedPages = 1; cursor.value = null; nodes.value = []; void load() }, { deep: true })
onBeforeRouteLeave(() => !session.identity || (sidebar.value?.canLeave() ?? true))
onMounted(() => { void metadata(); void load(); stopStream = subscribeWorkspace(changed, value => { live.value = value }); window.addEventListener('keydown', shortcut) })
onBeforeUnmount(() => { generation++; metadataGeneration++; clearTimeout(refreshTimer); stopStream?.(); window.removeEventListener('keydown', shortcut) })
</script>

<template>
  <section class="workspace" aria-labelledby="home-title">
    <header class="workspace-heading">
      <div><p class="eyebrow">Your workspace</p><h1 id="home-title">{{ session.identity?.tenant.name }}</h1></div>
      <button class="quiet search-trigger" @click="palette?.open()"><AppIcon name="search" /><span>Search all work</span><kbd>Ctrl / Cmd K</kbd></button>
    </header>
    <div class="workspace-grid glass-card" :class="{ 'has-sidebar': selected }">
      <div class="work-content">
        <div class="work-toolbar">
          <div class="view-switch" aria-label="Work display"><button class="quiet" :aria-pressed="mode === 'tree'" @click="mode = 'tree'"><AppIcon name="tree" />Tree</button><button class="quiet" :aria-pressed="mode === 'list'" @click="mode = 'list'"><AppIcon name="list" />List</button></div>
          <RouterLink class="quiet" to="/agents">Agents</RouterLink>
          <span class="connection" role="status">{{ live ? 'Live' : 'Reconnecting…' }}</span>
          <button class="quiet" :disabled="busy" aria-label="Refresh work" @click="changed">Refresh</button>
          <button class="button" :disabled="!kinds.length" @click="openCreate"><AppIcon name="plus" />New node</button>
        </div>
        <form v-if="mode === 'list'" class="filters" @submit.prevent="load()">
          <label>Kind<select v-model="filters.kind_id" aria-label="Kind"><option value="">All kinds</option><option v-for="kind in kinds" :key="kind.id" :value="kind.id">{{ kind.label }}</option></select></label>
          <label>State<input v-model="filters.state" placeholder="Any state" /></label>
          <label>Parent ID<input v-model="filters.parent_id" placeholder="Any parent" /></label>
          <label class="check"><input v-model="filters.include_descendants" type="checkbox" :disabled="!filters.parent_id" />Include descendants</label>
          <label>Sort<select v-model="sort" aria-label="Sort"><option value="position">Position</option><option value="updated_at">Updated</option><option value="created_at">Created</option><option value="key">Key</option><option value="title">Title</option></select></label>
          <label>Direction<select v-model="direction" aria-label="Direction"><option value="asc">Ascending</option><option value="desc">Descending</option></select></label>
          <fieldset><legend>Columns</legend><label v-for="column in ['key', 'kind', 'state', 'updated_at', 'created_at']" :key="column" class="check"><input v-model="columns" type="checkbox" :value="column" />{{ column.replace('_at', '') }}</label></fieldset>
        </form>
        <SavedViews :views="views" :configuration="configuration" @apply="applyView" @changed="metadata" />
        <p v-if="metadataError" class="work-message error" role="alert">{{ metadataError }} <button class="quiet" @click="metadata">Retry workspace settings</button></p>
        <p v-if="error" class="work-message error" role="alert">{{ error }} <button class="quiet" @click="load()">Retry work</button></p>
        <div class="work-scroll" :aria-busy="busy">
          <p v-if="busy && !rows.length" class="work-message" role="status">Loading work…</p>
          <div v-else-if="!rows.length && !error" class="work-empty"><AppIcon name="tree" /><h2>{{ mode === 'tree' ? 'Room to grow.' : 'No matching work.' }}</h2><p>{{ mode === 'tree' ? 'Create a node to begin your work tree.' : 'Try another kind, state, or parent.' }}</p></div>
          <div v-else class="node-rows" role="list" :aria-label="mode === 'tree' ? 'Work tree' : 'Work list'">
            <div v-for="entry in rows" :key="entry.node.id" class="node-row" role="listitem" :class="{ selected: selected === entry.node.id }" :style="{ '--depth': mode === 'tree' ? Math.min(entry.depth, 12) : 0 }">
              <button v-if="mode === 'tree'" class="branch-toggle" :aria-label="`${collapsed.has(entry.node.id) ? 'Expand' : 'Collapse'} ${entry.node.key}`" :aria-expanded="!collapsed.has(entry.node.id)" @click="toggle(entry.node.id)"><AppIcon name="chevron" /></button>
              <button class="node-open" :aria-label="`Open ${entry.node.key}: ${entry.node.title}`" @click="select(entry.node.id)"><span v-if="mode === 'tree' || columns.includes('key')" class="node-key">{{ entry.node.key }}</span><span class="node-title">{{ entry.node.title }}</span><span v-if="mode === 'tree' || columns.includes('kind')" class="kind-label">{{ kindLabel(entry.node.kind_id) }}</span><span v-if="mode === 'tree' || columns.includes('state')" class="state-label">{{ entry.node.state }}</span><time v-if="mode === 'list' && columns.includes('updated_at')" :datetime="entry.node.updated_at">{{ new Date(entry.node.updated_at).toLocaleDateString() }}</time><time v-if="mode === 'list' && columns.includes('created_at')" :datetime="entry.node.created_at">{{ new Date(entry.node.created_at).toLocaleDateString() }}</time></button>
            </div>
          </div>
          <button v-if="cursor" class="quiet load-more" :disabled="busy" @click="load(true)">{{ busy ? 'Loading…' : 'Load more work' }}</button>
        </div>
      </div>
      <NodeSidebar v-if="selected" ref="sidebar" :node-id="selected" @close="closeDetails" @changed="changed" />
    </div>
    <SearchPalette ref="palette" @select="select" />
    <dialog ref="createDialog" class="create-dialog" aria-labelledby="create-title" @cancel.prevent="closeCreate">
      <header><h2 id="create-title">Create a node</h2><button class="icon-button" aria-label="Close create node" :disabled="creating" @click="closeCreate"><AppIcon name="close" /></button></header>
      <form @submit.prevent="create">
        <label>Title<input ref="createTitle" v-model="newNode.title" required :disabled="creating" /></label>
        <label>Parent node ID<input v-model="newNode.parent_id" placeholder="Leave empty for a root node" :disabled="creating" @input="newNode.kind_id = ''" /></label>
        <label>Kind<select v-model="newNode.kind_id" aria-label="Kind" required :disabled="creating"><option value="" disabled>Choose a kind</option><option v-for="kind in allowedKinds" :key="kind.id" :value="kind.id">{{ kind.label }}</option></select></label>
        <label>Markdown<textarea v-model="newNode.body" rows="5" :disabled="creating" /></label>
        <details><summary>Custom fields (JSON)</summary><label>Fields<textarea v-model="newNode.fields" rows="4" :disabled="creating" /></label><p>The selected kind’s schema is validated when you create the node.</p><pre>{{ JSON.stringify(kinds.find(kind => kind.id === newNode.kind_id)?.field_schema, null, 2) }}</pre></details>
        <p v-if="createError" class="error" role="alert">{{ createError }}</p>
        <button class="button" :disabled="creating || !newNode.title.trim() || !newNode.kind_id">{{ creating ? 'Creating…' : 'Create node' }}</button>
      </form>
    </dialog>
  </section>
</template>
