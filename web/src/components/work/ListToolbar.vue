<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { DIMENSIONS, activeDimensions, type Dimension, type FacetOption, type GroupBy, type ListFilters } from '../../lib/ticketList'
import { plural } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import FacetMenu from './FacetMenu.vue'
import FloatingPanel from './FloatingPanel.vue'
import ColumnPicker from './ColumnPicker.vue'
import type { ColumnId } from '../../lib/columns'

const props = defineProps<{
  filters: ListFilters
  options: (dimension: Dimension) => FacetOption[]
  total: number | null
  loading: boolean
  density: 'comfortable' | 'compact'
  stuck: boolean
  view: 'list' | 'outline' | 'journey' | 'knowledge'
  // The table's columns for the Display menu's picker.
  columns?: { order: ColumnId[]; visible: ColumnId[]; customised: boolean } | null
}>()
const emit = defineEmits<{
  search: [q: string]
  toggle: [dimension: Dimension, value: string]
  clear: [dimension: Dimension]
  clearAll: []
  showClosed: [value: boolean]
  group: [value: GroupBy]
  density: [value: 'comfortable' | 'compact']
  openSheet: []
  needNames: []
  create: []
  view: [value: 'list' | 'outline' | 'journey' | 'knowledge']
  expandAll: []
  collapseAll: []
  columns: [order: ColumnId[], visible: ColumnId[]]
  columnsReset: []
}>()

const draft = ref(props.filters.q)
const root = ref<HTMLElement>()
// A narrow toolbar (docked panel, small window) gets a placeholder that fits.
const narrow = ref(false)
let resize: ResizeObserver | undefined
onMounted(() => {
  if (!root.value) return
  resize = new ResizeObserver(([entry]) => { narrow.value = entry.contentRect.width < 940 })
  resize.observe(root.value)
})
const input = ref<HTMLInputElement>()
const open = ref<{ dimension: Dimension; anchor: HTMLElement } | null>(null)
const displayAnchor = ref<HTMLElement | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined
watch(() => props.filters.q, value => { if (value !== draft.value.trim()) draft.value = value })
watch(draft, value => {
  clearTimeout(timer)
  timer = setTimeout(() => { if (value.trim() !== props.filters.q) emit('search', value.trim()) }, 220)
})
onBeforeUnmount(() => { clearTimeout(timer); resize?.disconnect() })

const active = computed(() => activeDimensions(props.filters))
const filterCount = computed(() => active.value.reduce((sum, key) => sum + props.filters[key].length, 0) + (props.filters.q ? 1 : 0))
const groups: { value: GroupBy; label: string }[] = [{ value: 'none', label: 'None' }, { value: 'status', label: 'Status' }, { value: 'epic', label: 'Epic' }]
const displayLabel = computed(() => props.view === 'outline' || props.filters.group === 'none' ? 'Display' : `Grouped by ${props.filters.group}`)

function title(dimension: Dimension) { return DIMENSIONS.find(d => d.key === dimension)!.title }
function openMenu(dimension: Dimension, event: MouseEvent) {
  const anchor = event.currentTarget as HTMLElement
  if (open.value?.dimension === dimension) { open.value = null; return }
  open.value = { dimension, anchor }
  if (dimension === 'assignee') emit('needNames')
}
function closeMenu(restore: boolean) {
  const anchor = open.value?.anchor
  open.value = null
  if (restore) anchor?.focus()
}
function chipText(dimension: Dimension) {
  const labels = props.filters[dimension].map(value => props.options(dimension).find(option => option.value === value)?.label ?? value)
  return labels.length > 2 ? `${labels.slice(0, 2).join(', ')} +${labels.length - 2}` : labels.join(', ')
}
function closeDisplay(restore: boolean) {
  const anchor = displayAnchor.value
  displayAnchor.value = null
  if (restore) anchor?.focus()
}
function focusSearch() { input.value?.focus(); input.value?.select() }
function clearSearch() { draft.value = ''; emit('search', ''); input.value?.focus() }
function searchKey(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    if (draft.value) clearSearch()
    else input.value?.blur()
  }
}
defineExpose({ focusSearch, input })
</script>

<template>
  <div ref="root" class="toolbar" :class="{ stuck, knowledge: view === 'knowledge' }" role="toolbar" :aria-label="view === 'knowledge' ? 'Knowledge controls' : 'Ticket list controls'">
    <div class="seg view-seg" role="radiogroup" aria-label="View">
      <button type="button" role="radio" :aria-checked="view === 'list'" aria-label="List view" data-tip="List view · flat, sortable, groupable" @click="emit('view', 'list')"><AppIcon name="list" :size="14" /><span class="view-label">List</span></button>
      <button type="button" role="radio" :aria-checked="view === 'outline'" aria-label="Outline view" data-tip="Outline view · epics, tickets and tasks as a tree" @click="emit('view', 'outline')"><AppIcon name="outline" :size="14" /><span class="view-label">Outline</span></button>
      <button type="button" role="radio" :aria-checked="view === 'journey'" aria-label="Journey view" data-tip="Journey · from the first conversation to live, with the next step" @click="emit('view', 'journey')"><AppIcon name="journey" :size="14" /><span class="view-label">Journey</span></button>
      <button type="button" role="radio" :aria-checked="view === 'knowledge'" aria-label="Knowledge" data-tip="Knowledge · runbooks, guidelines and memory agents read" @click="emit('view', 'knowledge')"><AppIcon name="book" :size="14" /><span class="view-label">Knowledge</span></button>
    </div>
    <template v-if="view === 'list' || view === 'outline'">
    <label class="search-field list-search">
      <AppIcon name="search" :size="14" />
      <input ref="input" v-model="draft" class="field" type="search" :placeholder="narrow ? 'Search' : 'Search this list'" aria-label="Search tickets in this project" aria-keyshortcuts="/" autocomplete="off" spellcheck="false" @keydown="searchKey" />
      <kbd v-if="!draft && !narrow" class="keycap slash" aria-hidden="true">/</kbd>
      <button v-if="draft" type="button" class="clear-q" aria-label="Clear search" @click="clearSearch"><AppIcon name="close" :size="12" /></button>
    </label>

    <div class="facets">
      <button
        v-for="dimension in DIMENSIONS" :key="dimension.key" type="button" class="btn sm facet-btn"
        :class="{ on: filters[dimension.key].length }" :aria-expanded="open?.dimension === dimension.key" aria-haspopup="dialog" @click="openMenu(dimension.key, $event)"
      >
        {{ dimension.title }}
        <span class="facet-end">
          <span v-if="filters[dimension.key].length" class="facet-count mono">{{ filters[dimension.key].length }}</span>
          <AppIcon v-else name="chevron" :size="12" class="facet-chevron" />
        </span>
      </button>
    </div>

    <div v-if="active.length" class="chips" aria-label="Applied filters">
      <span v-for="dimension in active" :key="dimension" class="filter-chip">
        <button type="button" class="chip-body" :aria-label="`Edit ${title(dimension)} filter`" @click="openMenu(dimension, $event)"><span class="chip-dim">{{ title(dimension) }}</span>{{ chipText(dimension) }}</button>
        <button type="button" class="chip-x" :aria-label="`Remove ${title(dimension)} filter`" @click="emit('clear', dimension)"><AppIcon name="close" :size="11" /></button>
      </span>
      <button v-if="active.length > 1" type="button" class="btn sm ghost clear-all" @click="emit('clearAll')">Clear all</button>
    </div>

    <span class="spacer" />

    <span class="count mono" role="status" aria-live="polite"><span v-if="total === null && loading" class="skeleton count-skeleton" aria-label="Counting tickets" /><template v-else-if="total !== null">{{ plural(total, 'ticket') }}</template></span>
    <label class="switch closed-switch">
      <input type="checkbox" :checked="!filters.showClosed" @change="emit('showClosed', !($event.target as HTMLInputElement).checked)" />
      <span>Hide closed</span>
    </label>
    <button
      type="button" class="btn sm closed-pill" :class="{ on: !filters.showClosed }" :aria-pressed="!filters.showClosed" aria-label="Hide closed tickets"
      :data-tip="filters.showClosed ? 'Closed tickets are shown\nClick to hide them' : 'Closed tickets are hidden\nClick to show them'" @click="emit('showClosed', !filters.showClosed)"
    ><AppIcon :name="filters.showClosed ? 'eye' : 'eye-off'" :size="14" />Closed</button>
    <button type="button" class="btn sm display-btn" :class="{ on: view === 'list' && filters.group !== 'none' }" aria-haspopup="dialog" :aria-expanded="!!displayAnchor" :aria-label="`Display: ${displayLabel}`" data-tip="Grouping, row height and columns" @click="displayAnchor = displayAnchor ? null : ($event.currentTarget as HTMLElement)">
      <AppIcon name="layers" :size="13" /><span class="display-label">{{ displayLabel }}</span><AppIcon name="chevron" :size="12" class="facet-chevron" />
    </button>

    <button type="button" class="btn primary new-btn" aria-label="New ticket" aria-keyshortcuts="n" data-tip="New ticket · n" @click="emit('create')"><AppIcon name="plus" :size="14" /><span class="new-label">New</span></button>
    <button type="button" class="btn filters-btn" :class="{ on: filterCount }" aria-label="Filters" @click="emit('openSheet')">
      <AppIcon name="sliders" :size="14" /><span class="filters-label">Filters</span><span v-if="filterCount" class="facet-count mono">{{ filterCount }}</span>
    </button>

    </template>
    <!-- The Knowledge tab teleports its own controls here (KnowledgeTab.vue). -->
    <div v-else-if="view === 'knowledge'" id="knowledge-controls" class="knowledge-controls" />
    <span v-else class="spacer" />
    <slot name="journey" />

    <FacetMenu
      v-if="open" :anchor="open.anchor" :dimension="open.dimension" :title="title(open.dimension)" :options="options(open.dimension)" :selected="filters[open.dimension]"
      @toggle="value => emit('toggle', open!.dimension, value)" @clear="emit('clear', open!.dimension)" @close="closeMenu"
    />
    <FloatingPanel v-if="displayAnchor" :anchor="displayAnchor" :width="300" :tallest="720" align="end" label="Display options" @close="closeDisplay">
      <div class="display-panel">
        <template v-if="view === 'list'">
          <p class="eyebrow">Group by</p>
          <div class="seg wide" role="radiogroup" aria-label="Group by">
            <button v-for="option in groups" :key="option.value" type="button" role="radio" :aria-checked="filters.group === option.value" :data-autofocus="filters.group === option.value ? '' : undefined" @click="emit('group', option.value)">{{ option.label }}</button>
          </div>
        </template>
        <template v-else>
          <p class="eyebrow">Outline</p>
          <div class="outline-actions">
            <button type="button" class="btn sm" data-autofocus @click="emit('expandAll'); closeDisplay(false)"><AppIcon name="expand-all" :size="13" />Expand all</button>
            <button type="button" class="btn sm" @click="emit('collapseAll'); closeDisplay(false)"><AppIcon name="collapse-all" :size="13" />Collapse all</button>
          </div>
        </template>
        <p class="eyebrow">Row height</p>
        <div class="seg wide" role="radiogroup" aria-label="Row height">
          <button type="button" role="radio" :aria-checked="density === 'comfortable'" @click="emit('density', 'comfortable')"><AppIcon name="rows-comfortable" :size="14" />Comfortable</button>
          <button type="button" role="radio" :aria-checked="density === 'compact'" @click="emit('density', 'compact')"><AppIcon name="rows-compact" :size="14" />Compact</button>
        </div>
        <ColumnPicker v-if="columns" class="column-picker" :order="columns.order" :visible="columns.visible" :customised="columns.customised" @change="(order, visible) => emit('columns', order, visible)" @reset="emit('columnsReset')" />
      </div>
    </FloatingPanel>
  </div>
</template>

<style scoped>
.toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 10px; min-height: 52px; padding: 9px 0; }
.list-search { width: 240px; flex-shrink: 0; }
.list-search .field { height: 32px; padding-right: 30px; font-size: 13.5px; }
.list-search .field::-webkit-search-cancel-button { display: none; }
.slash { position: absolute; right: 8px; pointer-events: none; }
@media (hover: none) { .slash { display: none; } }
.clear-q { position: absolute; right: 5px; display: grid; place-items: center; width: 22px; height: 22px; padding: 0; border: 0; border-radius: 50%; background: var(--chip-bg); color: var(--ink-2); }
.clear-q:hover { color: var(--ink); background: var(--row-selected); }
.clear-q:focus-visible { box-shadow: var(--focus-ring); }
.facets { display: flex; gap: 6px; }
.facet-btn { gap: 6px; padding: 0 9px 0 12px; font-weight: 600; color: var(--ink-2); }
.facet-btn:hover, .facet-btn[aria-expanded="true"] { color: var(--ink); }
.facet-btn.on { color: var(--teal-ink); }
/* Chevron and count badge share one slot so buttons never change width. */
.facet-end { display: inline-grid; place-items: center; width: 18px; }
.facet-chevron { color: var(--ink-3); }
.facet-count { display: inline-grid; place-items: center; min-width: 17px; height: 17px; padding: 0 5px; border-radius: 999px; background: linear-gradient(180deg, #1a8683, #0e6f6c); color: #fff; font-size: 10.5px; font-weight: 700; }
.chips { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; min-width: 0; }
.filter-chip { display: inline-flex; align-items: center; height: 28px; border-radius: 999px; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); font-size: 12.5px; max-width: 320px; }
.chip-body { display: inline-flex; align-items: center; gap: 6px; min-width: 0; height: 100%; padding: 0 4px 0 11px; border: 0; border-radius: 999px 0 0 999px; background: transparent; color: inherit; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-weight: 600; }
.chip-body:hover { background: rgba(14, 111, 108, .06); }
.chip-dim { font: 500 10px/1 var(--mono); letter-spacing: .1em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.chip-x { display: grid; place-items: center; flex-shrink: 0; width: 24px; height: 24px; margin-right: 2px; padding: 0; border: 0; border-radius: 50%; background: transparent; color: var(--teal-ink); }
.chip-x:hover { background: rgba(14, 111, 108, .12); }
.chip-x:active { background: rgba(14, 111, 108, .2); }
.chip-body:focus-visible, .chip-x:focus-visible { box-shadow: var(--focus-ring); }
.clear-all { padding: 0 8px; }
.spacer { flex: 1; }
/* The count keeps its width while numbers change, so the controls beside it never shift. */
.count { display: inline-block; min-width: 13ch; text-align: right; font-size: 12px; color: var(--ink-2); white-space: nowrap; }
.count-skeleton { display: inline-block; width: 64px; height: 8px; vertical-align: middle; }
.closed-switch { font-size: 12.5px; }
.display-btn { gap: 6px; color: var(--ink-2); }
.display-btn.on { color: var(--teal-ink); }
.filters-btn { display: none; }
.new-btn { height: 32px; padding: 0 14px 0 11px; gap: 6px; }
.display-panel { display: grid; gap: 8px; padding: 6px 8px 8px; }
.display-panel .eyebrow + .seg, .outline-actions { margin-bottom: 6px; }
.column-picker { margin-top: 8px; padding-top: 10px; border-top: 1px solid var(--line); }
.outline-actions { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; }
.view-seg { flex-shrink: 0; }
.knowledge-controls { display: contents; }
.view-seg button { height: 26px; padding: 0 11px; }
.seg.wide { display: grid; grid-auto-flow: column; grid-auto-columns: 1fr; }
.seg.wide button { height: 30px; }
/* Narrow list (docked panel or small window): tighter search, no count. */
@container toolbar (max-width: 1180px) { .view-label { display: none; } .view-seg button { padding: 0 8px; } }
@container toolbar (max-width: 1000px) { .list-search { width: 190px; } .count { display: none; } .new-btn { width: 32px; padding: 0; } .new-label { display: none; } }
@container toolbar (max-width: 920px) { .list-search { width: 150px; } .facet-btn { padding: 0 11px; } .facet-btn:not(.on) .facet-end { display: none; } }
@container toolbar (max-width: 820px) { .list-search { width: 112px; } .list-search .field { padding-right: 10px; } .facet-btn { padding: 0 10px; } .facet-btn:not(.on) .facet-end { display: none; } .view-seg button { padding: 0 7px; } }
@container toolbar (max-width: 900px) { .display-label { display: none; } .display-btn { padding: 0 9px; } }
/* Narrowest docked width: a labelled pill replaces the switch and its longer label. */
.closed-pill { display: none; gap: 6px; padding: 0 11px 0 9px; color: var(--ink-2); }
.closed-pill.on { color: var(--teal-ink); }
@container toolbar (max-width: 800px) { .closed-switch { display: none; } .closed-pill { display: inline-flex; } }
@media (max-width: 900px) { .facets, .chips, .display-btn, .closed-switch, .closed-pill, .spacer { display: none; } .list-search { flex: 1; width: auto; } .filters-btn { display: inline-flex; } }
@media (max-width: 600px) {
  .toolbar { flex-wrap: nowrap; gap: 8px; padding: 8px 0; }
  .list-search .field { height: 44px; font-size: 16px; }
  .filters-btn { height: 44px; width: 44px; padding: 0; position: relative; }
  .filters-label { display: none; }
  .filters-btn .facet-count { position: absolute; top: -2px; right: -2px; }
  .view-seg { padding: 2px; }
  .view-seg button { width: 40px; height: 40px; padding: 0; }
  .new-btn { order: 3; width: 44px; height: 44px; padding: 0; }
  .new-label { display: none; }
  .count { display: none; }
  /* Knowledge on a phone: the view switch on its own line, search and filters below. */
  .toolbar.knowledge { flex-wrap: wrap; }
  .knowledge-controls { display: flex; flex: 1 1 100%; align-items: center; gap: 8px; min-width: 0; }
}
</style>
