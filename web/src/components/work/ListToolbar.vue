<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { DIMENSIONS, activeDimensions, type Dimension, type FacetOption, type GroupBy, type ListFilters } from '../../lib/ticketList'
import { plural } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import FacetMenu from './FacetMenu.vue'
import FloatingPanel from './FloatingPanel.vue'

const props = defineProps<{
  filters: ListFilters
  options: (dimension: Dimension) => FacetOption[]
  total: number | null
  loading: boolean
  density: 'comfortable' | 'compact'
  stuck: boolean
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
}>()

const draft = ref(props.filters.q)
const input = ref<HTMLInputElement>()
const open = ref<{ dimension: Dimension; anchor: HTMLElement } | null>(null)
const groupAnchor = ref<HTMLElement | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined
watch(() => props.filters.q, value => { if (value !== draft.value.trim()) draft.value = value })
watch(draft, value => {
  clearTimeout(timer)
  timer = setTimeout(() => { if (value.trim() !== props.filters.q) emit('search', value.trim()) }, 220)
})
onBeforeUnmount(() => clearTimeout(timer))

const active = computed(() => activeDimensions(props.filters))
const filterCount = computed(() => active.value.reduce((sum, key) => sum + props.filters[key].length, 0) + (props.filters.q ? 1 : 0))
const groups: { value: GroupBy; label: string }[] = [{ value: 'none', label: 'No grouping' }, { value: 'status', label: 'Status' }, { value: 'epic', label: 'Epic' }]
const groupLabel = computed(() => props.filters.group === 'none' ? 'Group' : `Group: ${groups.find(g => g.value === props.filters.group)!.label}`)

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
function chooseGroup(value: GroupBy) { emit('group', value); groupAnchor.value = null }
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
  <div class="toolbar" :class="{ stuck }" role="toolbar" aria-label="Ticket list controls">
    <label class="search-field list-search">
      <AppIcon name="search" :size="14" />
      <input ref="input" v-model="draft" class="field" type="search" placeholder="Search this list" aria-label="Search tickets in this project" aria-keyshortcuts="/" autocomplete="off" spellcheck="false" @keydown="searchKey" />
      <kbd v-if="!draft" class="keycap slash" aria-hidden="true">/</kbd>
      <button v-else type="button" class="clear-q" aria-label="Clear search" @click="clearSearch"><AppIcon name="close" :size="12" /></button>
    </label>

    <div class="facets">
      <button
        v-for="dimension in DIMENSIONS" :key="dimension.key" type="button" class="btn sm facet-btn"
        :class="{ on: filters[dimension.key].length }" :aria-expanded="open?.dimension === dimension.key" aria-haspopup="dialog" @click="openMenu(dimension.key, $event)"
      >
        {{ dimension.title }}
        <span v-if="filters[dimension.key].length" class="facet-count mono">{{ filters[dimension.key].length }}</span>
        <AppIcon v-else name="chevron" :size="12" class="facet-chevron" />
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
    <button type="button" class="btn sm group-btn" :class="{ on: filters.group !== 'none' }" aria-haspopup="menu" :aria-expanded="!!groupAnchor" @click="groupAnchor = groupAnchor ? null : ($event.currentTarget as HTMLElement)">
      <AppIcon name="layers" :size="13" />{{ groupLabel }}
    </button>
    <div class="seg icons density" role="radiogroup" aria-label="Row density">
      <button type="button" role="radio" :aria-checked="density === 'comfortable'" aria-label="Comfortable rows" data-tip="Comfortable rows" @click="emit('density', 'comfortable')"><AppIcon name="rows-comfortable" :size="14" /></button>
      <button type="button" role="radio" :aria-checked="density === 'compact'" aria-label="Compact rows" data-tip="Compact rows" @click="emit('density', 'compact')"><AppIcon name="rows-compact" :size="14" /></button>
    </div>

    <button type="button" class="btn filters-btn" :class="{ on: filterCount }" @click="emit('openSheet')">
      <AppIcon name="sliders" :size="14" />Filters<span v-if="filterCount" class="facet-count mono">{{ filterCount }}</span>
    </button>

    <FacetMenu
      v-if="open" :anchor="open.anchor" :dimension="open.dimension" :title="title(open.dimension)" :options="options(open.dimension)" :selected="filters[open.dimension]"
      @toggle="value => emit('toggle', open!.dimension, value)" @clear="emit('clear', open!.dimension)" @close="closeMenu"
    />
    <FloatingPanel v-if="groupAnchor" :anchor="groupAnchor" :width="190" align="end" label="Group tickets" @close="restore => { const a = groupAnchor; groupAnchor = null; if (restore) a?.focus() }">
      <p class="eyebrow menu-title">Group by</p>
      <div role="menu" aria-label="Group by">
        <button v-for="option in groups" :key="option.value" type="button" role="menuitemradio" class="menu-item" :aria-checked="filters.group === option.value" :data-autofocus="filters.group === option.value ? '' : undefined" @click="chooseGroup(option.value)">
          <span>{{ option.label }}</span><AppIcon v-if="filters.group === option.value" name="check" :size="14" class="tick" />
        </button>
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
.facets { display: flex; gap: 6px; }
.facet-btn { gap: 6px; padding: 0 10px 0 12px; font-weight: 600; color: var(--ink-2); }
.facet-btn:hover, .facet-btn[aria-expanded="true"] { color: var(--ink); }
.facet-btn.on { color: var(--teal-ink); }
.facet-chevron { color: var(--ink-3); }
.facet-count { display: inline-grid; place-items: center; min-width: 17px; height: 17px; padding: 0 5px; border-radius: 999px; background: linear-gradient(180deg, #1a8683, #0e6f6c); color: #fff; font-size: 10.5px; font-weight: 700; }
.chips { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; min-width: 0; }
.filter-chip { display: inline-flex; align-items: center; height: 28px; border-radius: 999px; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); font-size: 12.5px; max-width: 320px; }
.chip-body { display: inline-flex; align-items: center; gap: 6px; min-width: 0; height: 100%; padding: 0 4px 0 11px; border: 0; border-radius: 999px 0 0 999px; background: transparent; color: inherit; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-weight: 600; }
.chip-dim { font: 500 10px/1 var(--mono); letter-spacing: .1em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.chip-x { display: grid; place-items: center; flex-shrink: 0; width: 24px; height: 24px; margin-right: 2px; padding: 0; border: 0; border-radius: 50%; background: transparent; color: var(--teal-ink); }
.chip-x:hover { background: rgba(14, 111, 108, .12); }
.clear-all { padding: 0 8px; }
.spacer { flex: 1; }
.count { font-size: 12px; color: var(--ink-2); white-space: nowrap; }
.count-skeleton { display: inline-block; width: 64px; height: 8px; vertical-align: middle; }
.closed-switch { font-size: 12.5px; }
.group-btn { color: var(--ink-2); }
.group-btn.on { color: var(--teal-ink); }
.density button { height: 24px; min-width: 26px; }
.filters-btn { display: none; }
.menu-title { padding: 6px 10px 4px; }
.menu-item { display: flex; align-items: center; justify-content: space-between; gap: 10px; width: 100%; height: 32px; padding: 0 10px; border: 0; border-radius: 8px; background: transparent; color: var(--ink); font-size: 13.5px; text-align: left; }
.menu-item:hover, .menu-item:focus-visible { background: var(--row-selected); box-shadow: none; }
.tick { color: var(--teal); }
@media (max-width: 1240px) { .count { display: none; } }
@media (max-width: 1080px) { .density { display: none; } .list-search { width: 200px; } }
@media (max-width: 900px) { .facets, .chips, .group-btn, .closed-switch, .spacer { display: none; } .list-search { flex: 1; width: auto; } .filters-btn { display: inline-flex; } .count { display: inline; } }
@media (max-width: 600px) {
  .toolbar { flex-wrap: nowrap; gap: 8px; padding: 8px 0; }
  .list-search .field { height: 44px; font-size: 16px; }
  .filters-btn { height: 44px; padding: 0 14px; }
  .count { display: none; }
}
</style>
