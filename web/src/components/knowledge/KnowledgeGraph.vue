<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../AppIcon.vue'
import KnowledgeGraphControls from './KnowledgeGraphControls.vue'
import KnowledgeGraphLegend from './KnowledgeGraphLegend.vue'
import { listNodes } from '../../lib/api'
import { toast } from '../../lib/toast'
import { entryPath, type KnowledgeType } from '../../lib/knowledge'
import { STATUS_VIEWS, type KnowledgeFilters } from '../../lib/useKnowledge'
import { fetchKnowledgeGraph, filterGraph, graphEntry, graphMatches, graphNeighbours, graphTypeLabel, graphTypeTokens, type GraphDimension, type GraphNode, type KnowledgeGraphData } from '../../lib/knowledgeGraph'
import { createKnowledgeGraphRenderer, type KnowledgeGraphRenderer } from '../../lib/knowledgeGraphRenderer'

// docked: the selected entry is open in the preview pane beside the graph, so the card would repeat it.
const props = defineProps<{ project: { id: string; routeKey: string; title: string }; filters: KnowledgeFilters; canWrite: boolean; docked?: boolean }>()
const emit = defineEmits<{ create: []; reset: []; list: [] }>()
const route = useRoute(), router = useRouter()
const root = ref<HTMLElement>(), host = ref<HTMLElement>()
const empty: KnowledgeGraphData = { nodes: [], edges: [], truncated: false }
const data = shallowRef<KnowledgeGraphData>(empty)
const loading = ref(true), error = ref(''), renderError = ref(''), ready = ref(false), fallback = ref(false)
const media = window.matchMedia('(prefers-reduced-motion: reduce)')
const schemeMedia = window.matchMedia('(prefers-color-scheme: dark)')
const reduced = ref(media.matches), dimension = ref<GraphDimension>(media.matches ? '2d' : '3d'), paused = ref(media.matches), tickets = ref(false)
const hovered = shallowRef<GraphNode | null>(null), ticketSelection = ref('')
const pointer = ref({ x: 0, y: 0 })
const visible = computed(() => filterGraph(data.value, props.filters.type))
const selected = computed(() => visible.value.nodes.find(n => n.kind === 'knowledge' ? graphEntry(n) === route.query.entry : n.id === ticketSelection.value) ?? null)
const query = computed(() => props.filters.q.trim())
const matches = computed(() => graphMatches(visible.value.nodes, query.value))
const neighbours = computed(() => selected.value ? graphNeighbours(visible.value.edges, selected.value.id) : new Set<string>())
const searchResults = computed(() => query.value ? visible.value.nodes.filter(n => matches.value.has(n.id)).slice(0, 5) : [])
const entries = computed(() => visible.value.nodes.filter(n => n.kind === 'knowledge').length)
const ticketCount = computed(() => visible.value.nodes.length - entries.value)
const summary = computed(() => `${entries.value} entries, ${visible.value.edges.length} ${visible.value.edges.length === 1 ? 'link' : 'links'}${ticketCount.value ? `, ${ticketCount.value} linked tickets` : ''}`)
let renderer: KnowledgeGraphRenderer | null = null
let renderController: AbortController | null = null, loadController: AbortController | null = null
let resize: ResizeObserver | null = null, theme: MutationObserver | null = null
let mounted = false, interacted = false

async function load() {
  loadController?.abort()
  const controller = loadController = new AbortController()
  loading.value = true; error.value = ''
  try {
    const statuses = STATUS_VIEWS.find(v => v.value === props.filters.status)?.statuses ?? []
    const result = await fetchKnowledgeGraph(props.project.id, statuses, tickets.value, controller.signal)
    if (!controller.signal.aborted) data.value = result
  } catch (e) { if (!controller.signal.aborted) { data.value = empty; error.value = e instanceof Error ? e.message : 'The graph could not be loaded.' } }
  finally { if (!controller.signal.aborted) loading.value = false }
}
function emphasis() {
  renderer?.emphasis({ selected: selected.value?.id ?? '', neighbours: neighbours.value, matches: matches.value, searching: !!query.value, hovered: hovered.value?.id ?? '' })
}
async function startRenderer() {
  renderController?.abort(); renderer?.dispose(); renderer = null; ready.value = false
  const controller = renderController = new AbortController()
  await nextTick()
  if (!host.value || !mounted || controller.signal.aborted) return
  renderError.value = ''
  try {
    const next = await createKnowledgeGraphRenderer(host.value, dimension.value, {
      reduced: reduced.value, signal: controller.signal, select, open, hover: node => { hovered.value = node }, clear,
    })
    if (controller.signal.aborted || !mounted) { next?.dispose(); return }
    if (!next) return
    renderer = next
    if (dimension.value === '3d' && next.dimension === '2d') { fallback.value = true; dimension.value = '2d' }
    renderer.resize(host.value.clientWidth, host.value.clientHeight)
    renderer.motion(paused.value); renderer.data(visible.value); emphasis(); ready.value = true
    if (interacted || selected.value) renderer.interact()
    if (selected.value) renderer.focus(selected.value.id)
  } catch { if (!controller.signal.aborted) renderError.value = 'The graph could not start. You can still explore every entry in the list.' }
}
function fit() { interact(); renderer?.fit() }
function interact() { interacted = true; renderer?.interact() }
function retheme() { renderer?.theme() }
function setDimension(value: GraphDimension) {
  if (value === dimension.value) return
  interact(); dimension.value = value; hovered.value = null; void startRenderer()
}
function select(node: GraphNode) {
  if (!mounted) return
  interact(); ticketSelection.value = node.kind === 'ticket' ? node.id : ''
  void router.replace({ query: { ...route.query, entry: graphEntry(node) || undefined } })
  renderer?.focus(node.id); host.value?.focus({ preventScroll: true })
}
function clear() {
  if (!mounted) return
  interact(); ticketSelection.value = ''; hovered.value = null
  if (route.query.entry) void router.replace({ query: { ...route.query, entry: undefined } })
}
async function open(node: GraphNode) {
  if (!mounted) return
  if (node.kind === 'knowledge') {
    await router.push({ path: entryPath(props.project.routeKey, node.type as KnowledgeType, node.slug), query: { ...route.query, entry: undefined } })
    return
  }
  // A satellite can belong to another project. Resolve its project only when
  // opening the full ticket; graph loading never requests ticket bodies.
  try {
    const page = await listNodes({ q: node.key, kind: ['ticket'], sort: 'key', limit: 100 })
    if (!mounted) return
    const project = page.items.find(item => item.id === node.id)?.project
    if (!project) throw new Error('Ticket project unavailable')
    await router.push({ path: `/p/${encodeURIComponent(project.key)}/${encodeURIComponent(node.key)}` })
  } catch { toast('The ticket could not be opened. Please try again.', { tone: 'error' }) }
}
function keydown(event: KeyboardEvent) {
  if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.altKey || (event.target instanceof HTMLElement && event.target.closest('input, textarea, select, [contenteditable="true"], dialog'))) return
  if (event.key === 'Escape' && selected.value) { event.preventDefault(); clear(); host.value?.focus({ preventScroll: true }) }
  if (event.key === 'Enter' && selected.value && event.target === host.value) { event.preventDefault(); open(selected.value) }
  if (event.target === host.value && ['ArrowRight', 'ArrowLeft'].includes(event.key)) {
    const list = visible.value.nodes, at = list.findIndex(n => n.id === selected.value?.id)
    const next = (at + (event.key === 'ArrowRight' ? 1 : -1) + list.length) % list.length
    if (list[next]) { event.preventDefault(); select(list[next]) }
  }
}
function move(event: PointerEvent) {
  const rect = root.value?.getBoundingClientRect(); if (!rect) return
  pointer.value = { x: Math.max(8, Math.min(event.clientX - rect.left + 18, rect.width - 280)), y: Math.max(70, Math.min(event.clientY - rect.top + 18, rect.height - 150)) }
  interact()
}
function toggleMotion() { interact(); paused.value = !paused.value; renderer?.motion(paused.value) }
function onReduced(event: MediaQueryListEvent) {
  reduced.value = event.matches; paused.value = event.matches
  if (event.matches) dimension.value = '2d'
  void startRenderer()
}
watch([() => props.project.id, () => props.filters.status, tickets], () => { if (mounted) void load() })
watch(visible, value => { hovered.value = null; renderer?.data(value); emphasis() })
watch([selected, matches, hovered], emphasis)
watch(() => route.query.entry, () => { if (selected.value) { interact(); renderer?.focus(selected.value.id) } })
onMounted(() => {
  mounted = true
  resize = new ResizeObserver(() => { if (host.value) renderer?.resize(host.value.clientWidth, host.value.clientHeight) })
  if (host.value) resize.observe(host.value)
  theme = new MutationObserver(() => renderer?.theme())
  theme.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'style', 'class'] })
  media.addEventListener('change', onReduced); schemeMedia.addEventListener('change', retheme)
  window.addEventListener('keydown', keydown); window.addEventListener('pointerdown', interact, { passive: true }); window.addEventListener('wheel', interact, { passive: true })
  void load(); void startRenderer()
})
defineExpose({ focus: () => host.value?.focus({ preventScroll: true }) })
onBeforeUnmount(() => {
  mounted = false; loadController?.abort(); renderController?.abort(); resize?.disconnect(); theme?.disconnect()
  media.removeEventListener('change', onReduced); schemeMedia.removeEventListener('change', retheme)
  window.removeEventListener('keydown', keydown); window.removeEventListener('pointerdown', interact); window.removeEventListener('wheel', interact)
  renderer?.dispose(); renderer = null
})
</script>

<template>
  <section ref="root" class="kg" aria-label="Knowledge graph" @pointerdown="interact" @wheel.passive="interact" @keydown.capture="interact">
    <header class="kg-header">
      <div><h2>Knowledge, connected</h2><p class="kg-count" role="status">{{ loading ? 'Finding the connections…' : summary }}</p></div>
      <KnowledgeGraphControls :dimension="dimension" :paused="paused" :tickets="tickets" :fallback="fallback" :reduced="reduced" @fit="fit" @dimension="setDimension" @pause="toggleMotion" @tickets="tickets = !tickets" />
    </header>
    <div class="kg-stage" @pointermove="move" @pointerleave="hovered = null">
      <div ref="host" class="kg-canvas" role="img" tabindex="0" :aria-label="`Knowledge graph: ${summary}. Use Entries for accessible reading. Left and right arrows select entries; Enter opens; Escape clears.`" :data-dimension="dimension" :data-ready="ready" :data-motion="reduced || paused ? 'still' : 'on'" />
      <div v-if="loading || error || renderError || !visible.nodes.length" class="kg-state">
        <span v-if="loading" class="kg-loading" aria-hidden="true"><AppIcon name="link" :size="24" /></span>
        <template v-if="error || renderError"><AppIcon name="alert" :size="24" /><h3>Connections are taking a moment</h3><p role="alert">{{ error || renderError }}</p><button class="btn" type="button" @click="error ? load() : startRenderer()">Try again</button></template>
        <template v-else-if="!loading"><AppIcon name="book" :size="26" /><h3>{{ data.nodes.length ? 'No entries of this kind' : 'A place for connections to grow' }}</h3><p>Link entries with <code>[[slug]]</code> mentions or relations.</p><button v-if="data.nodes.length" class="btn" type="button" @click="emit('reset')">Show all knowledge</button><button v-else-if="canWrite" class="btn primary" type="button" @click="emit('create')"><AppIcon name="plus" :size="14" />Write the first entry</button><button v-else class="btn" type="button" @click="emit('list')">Show the entries</button></template>
      </div>
      <div v-if="!loading && !error && query" class="kg-results">
        <p>{{ matches.size }} {{ matches.size === 1 ? 'match' : 'matches' }} <span>in this graph</span></p>
        <button v-for="node in searchResults" :key="node.id" type="button" :aria-pressed="selected?.id === node.id" @click="select(node)"><span class="kg-dot" :style="{ background: `var(${graphTypeTokens[node.type]})` }" /><span>{{ node.title }}</span><AppIcon name="arrow" :size="12" /></button>
        <button v-if="!matches.size" type="button" @click="emit('reset')">Clear the filters<AppIcon name="close" :size="12" /></button>
      </div>
      <div v-if="!loading && visible.nodes.length && (!visible.edges.length || visible.nodes.length < 5) && !selected && !query" class="kg-sparse"><AppIcon name="link" :size="14" /><span>Add <code>[[slug]]</code> mentions or relations to connect these entries.</span><button class="btn sm" type="button" @click="open(visible.nodes[0])">Open an entry<AppIcon name="arrow" :size="12" /></button></div>
      <div v-if="selected && !(docked && selected.kind === 'knowledge')" class="kg-selection" aria-live="polite">
        <div class="kg-selection-meta"><span class="kg-dot" :style="{ background: `var(${graphTypeTokens[selected.type]})` }" />{{ graphTypeLabel(selected) }}<span class="mono">{{ selected.degree }} {{ selected.degree === 1 ? 'link' : 'links' }}</span><button type="button" class="icon-btn sm flat" aria-label="Clear graph selection" @click="clear"><AppIcon name="close" :size="12" /></button></div>
        <h3>{{ selected.title }}</h3><p class="mono">{{ selected.slug || selected.key }}</p>
        <button type="button" class="btn sm" @click="open(selected)">Open {{ selected.kind === 'ticket' ? 'ticket' : 'entry' }}<AppIcon name="external" :size="12" /></button><span class="kg-enter"><kbd class="keycap">Enter</kbd></span>
      </div>
      <div v-if="hovered && hovered.id !== selected?.id" class="kg-tooltip" role="tooltip" :style="{ left: `${pointer.x}px`, top: `${pointer.y - 64}px` }"><span>{{ graphTypeLabel(hovered) }} · {{ hovered.degree }} {{ hovered.degree === 1 ? 'link' : 'links' }}</span><strong>{{ hovered.title }}</strong><code>{{ hovered.slug || hovered.key }}</code></div>
      <span v-if="!loading && visible.nodes.length && !query && !selected" class="kg-navigation">{{ dimension === '3d' ? 'Drag to orbit' : 'Drag to pan' }}<span>Scroll to zoom</span><span>Select to explore</span></span>
    </div>
    <footer class="kg-footer"><KnowledgeGraphLegend :nodes="visible.nodes" /><p v-if="data.truncated" role="status">Showing up to 2,000 entries and tickets, and 8,000 links. Choose a status to narrow the graph.</p><p v-if="fallback">3D is unavailable here. The 2D graph has the same entries and controls.</p></footer>
  </section>
</template>

<style scoped>
.kg { position: relative; min-width: 0; overflow: hidden; border-radius: var(--radius); background: var(--canvas); box-shadow: var(--shadow); align-self: start; }
.kg-header { display: flex; justify-content: space-between; flex-wrap: wrap; align-items: center; gap: 16px; padding: 22px 24px 16px; position: relative; z-index: 1; }
.kg-header h2 { font-size: 17px; font-weight: 580; letter-spacing: -.025em; }
.kg-count { font: 11px/1.6 var(--mono); color: var(--ink-3); margin-top: 5px; }
.kg-stage { position: relative; height: clamp(330px, calc(100dvh - 452px), 700px); min-width: 0; }
.kg-canvas { width: 100%; height: 100%; outline: none; }
.kg-canvas:focus-visible { box-shadow: inset 0 0 0 2px var(--teal); border-radius: 8px; }
.kg-canvas :deep(canvas) { display: block; }
.kg-state { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 15px; padding: 30px; text-align: center; background: var(--canvas); color: var(--ink-2); }
.kg-state h3 { font-size: 20px; font-weight: 550; color: var(--ink); letter-spacing: -.025em; }
.kg-state p { font-size: 13px; max-width: 360px; }
.kg-loading { display: grid; place-items: center; width: 64px; height: 64px; border-radius: 50%; background: var(--surface-2); color: var(--teal); }
.kg-footer { padding: 16px 24px 20px; }
.kg-footer > p { font-size: 11.5px; color: var(--ink-3); margin-top: 10px; }
.kg-navigation { position: absolute; bottom: 12px; left: 0; right: 0; display: flex; justify-content: center; flex-wrap: wrap; gap: 16px; pointer-events: none; font-size: 11px; color: var(--ink-3); }
.kg-navigation span { opacity: .8; }
.kg-dot { width: 7px; height: 7px; flex: 0 0 auto; border-radius: 50%; }
.kg-selection, .kg-results, .kg-tooltip { background: var(--surface-raised-2); border-radius: 12px; box-shadow: var(--shadow-pop); -webkit-backdrop-filter: blur(14px); backdrop-filter: blur(14px); color: var(--ink); }
.kg-selection { position: absolute; bottom: 18px; left: 22px; width: min(310px, calc(100% - 44px)); padding: 14px 16px; }
.kg-selection-meta { display: flex; align-items: center; gap: 7px; font-size: 11px; color: var(--ink-2); }
.kg-selection-meta .mono { margin-left: auto; font-size: 10px; }
.kg-selection-meta .icon-btn { margin: -5px -8px -5px 0; }
.kg-selection h3 { font-size: 15px; line-height: 1.4; margin: 8px 0 4px; font-weight: 600; overflow-wrap: anywhere; }
.kg-selection > p { font-size: 10px; color: var(--ink-3); margin-bottom: 13px; overflow-wrap: anywhere; }
.kg-enter { margin-left: 10px; color: var(--ink-3); font-size: 10px; }
.kg-tooltip { position: absolute; pointer-events: none; padding: 12px 14px; width: 260px; z-index: 3; display: grid; gap: 5px; }
.kg-tooltip > span { font-size: 10px; color: var(--ink-3); }
.kg-tooltip strong { font-size: 13px; line-height: 1.4; font-weight: 550; overflow-wrap: anywhere; }
.kg-tooltip code { font-size: 10px; color: var(--ink-3); overflow-wrap: anywhere; }
.kg-results { position: absolute; top: 8px; left: 22px; padding: 12px; width: min(310px, calc(100% - 44px)); }
.kg-results > p { font-size: 11px; font-weight: 600; padding: 0 6px 8px; }
.kg-results > p span { color: var(--ink-3); font-weight: 400; }
.kg-results > button { width: 100%; display: flex; align-items: center; gap: 9px; text-align: left; border: 0; background: transparent; color: var(--ink); padding: 8px 6px; font-size: 12px; border-radius: 7px; }
.kg-results > button > span:nth-child(2) { flex: 1; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.kg-results > button:hover, .kg-results > button[aria-pressed="true"] { background: var(--row-selected); }
.kg-results > button:focus-visible { box-shadow: var(--focus-ring); }
.kg-sparse { position: absolute; bottom: 52px; left: 22px; right: 22px; display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 10px; color: var(--ink-2); font-size: 12px; }
@media (max-width: 1100px) { .kg-header { padding: 18px 18px 10px; gap: 12px; } .kg-footer { padding: 14px 18px; } }
@media (max-width: 600px) { .kg-stage { height: 470px; } .kg-header h2 { font-size: 16px; } .kg-tooltip { display: none; } .kg-navigation { gap: 10px; font-size: 10px; } }
</style>
