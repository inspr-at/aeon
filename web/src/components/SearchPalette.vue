<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { searchNodes, type SearchHit, type WorkNode } from '../lib/api'
import { highlight } from '../lib/work'
import AppIcon from './AppIcon.vue'
import StatusIcon from './work/StatusIcon.vue'
const emit = defineEmits<{ select: [node: WorkNode] }>()
const dialog = ref<HTMLDialogElement>()
const input = ref<HTMLInputElement>()
const term = ref('')
const hits = ref<SearchHit[]>([])
const cursor = ref<string | null>(null)
const busy = ref(false)
const error = ref('')
const active = ref(0)
let generation = 0
let timer: ReturnType<typeof setTimeout> | undefined
let opener: HTMLElement | null = null
async function open() {
  if (dialog.value?.open) { input.value?.select(); return }
  opener = document.activeElement as HTMLElement
  dialog.value?.showModal()
  await nextTick()
  input.value?.focus()
  input.value?.select()
}
function close() { dialog.value?.close(); opener?.focus() }
async function search(more = false) {
  const request = ++generation
  const q = term.value.trim()
  if (!q) { hits.value = []; cursor.value = null; busy.value = false; return }
  busy.value = true
  error.value = ''
  try {
    const page = await searchNodes(q, { cursor: more ? cursor.value ?? undefined : undefined })
    if (request !== generation) return
    hits.value = more ? [...hits.value, ...page.items] : page.items
    cursor.value = page.next_cursor
    if (!more) active.value = 0
  } catch (e) { if (request === generation) error.value = e instanceof Error ? e.message : 'Search unavailable' }
  finally { if (request === generation) busy.value = false }
}
watch(term, () => {
  generation++
  clearTimeout(timer)
  hits.value = []; cursor.value = null; error.value = ''; active.value = 0
  busy.value = !!term.value.trim()
  timer = setTimeout(() => void search(), 180)
})
function choose(node: WorkNode) { close(); emit('select', node) }
function keydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    active.value = Math.max(0, Math.min(hits.value.length - 1, active.value + (event.key === 'ArrowDown' ? 1 : -1)))
    document.getElementById(`search-hit-${active.value}`)?.scrollIntoView({ block: 'nearest' })
  } else if (event.key === 'Enter') {
    event.preventDefault()
    const hit = hits.value[active.value]
    if (hit) choose(hit.node)
  }
}
function backdrop(event: MouseEvent) { if (event.target === dialog.value) close() }
onBeforeUnmount(() => { clearTimeout(timer); generation++ })
function refresh() { if (dialog.value?.open && term.value.trim()) void search() }
defineExpose({ open, refresh })
</script>
<template>
  <dialog ref="dialog" class="search-palette" aria-labelledby="search-title" @cancel.prevent="close" @click="backdrop">
    <div class="palette-card">
      <h2 id="search-title" class="sr-only">Search all work</h2>
      <label class="palette-input">
        <AppIcon name="search" :size="18" />
        <input ref="input" v-model="term" aria-label="Search all work" placeholder="Search tickets, epics and projects by title or key…" role="combobox" aria-controls="search-results" :aria-expanded="true" :aria-activedescendant="hits.length ? `search-hit-${active}` : undefined" autocomplete="off" spellcheck="false" @keydown="keydown" />
        <button class="icon-btn sm flat" type="button" aria-label="Close search" @click="close"><AppIcon name="close" :size="14" /></button>
      </label>
      <div class="palette-body">
        <p v-if="error" class="palette-note error" role="alert">{{ error }} <button class="btn sm" type="button" @click="search()">Retry search</button></p>
        <p v-else-if="busy && !hits.length" class="palette-note" role="status">Searching…</p>
        <p v-else-if="!term.trim()" class="palette-note">Search everything in your workspace. Open a result with <kbd class="keycap"><AppIcon name="enter" /></kbd>.</p>
        <p v-else-if="!hits.length" class="palette-note" role="status">No matching work.</p>
        <div id="search-results" role="listbox" aria-label="Search results">
          <button v-for="(hit, index) in hits" :id="`search-hit-${index}`" :key="hit.node.id" class="search-result" type="button" role="option" :aria-selected="index === active" @click="choose(hit.node)" @pointermove="active = index">
            <StatusIcon :state="hit.node.state" />
            <span class="key">{{ hit.node.key }}</span>
            <strong><template v-for="(part, i) in highlight(hit.node.title, term)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></strong>
            <AppIcon v-if="index === active" name="enter" :size="14" class="enter" />
          </button>
        </div>
        <button v-if="cursor" class="btn sm more" type="button" :disabled="busy" @click="search(true)">More results</button>
      </div>
      <footer class="palette-foot">
        <span><kbd class="keycap"><AppIcon name="arrow-up" /></kbd><kbd class="keycap"><AppIcon name="arrow-down" /></kbd> move</span>
        <span><kbd class="keycap"><AppIcon name="enter" /></kbd> open</span>
        <span><kbd class="keycap">esc</kbd> close</span>
      </footer>
    </div>
  </dialog>
</template>
<style scoped>
.search-palette { width: min(640px, calc(100vw - 24px)); max-width: none; max-height: min(560px, 80dvh); margin: 12dvh auto auto; padding: 0; border: 0; background: transparent; color: var(--ink); overflow: visible; }
.search-palette::backdrop { background: var(--scrim); backdrop-filter: blur(2px); }
.palette-card { display: flex; flex-direction: column; max-height: min(560px, 80dvh); border-radius: var(--radius); background: var(--surface-raised); border: 1px solid var(--glass-edge); box-shadow: var(--shadow-pop); overflow: hidden; }
.palette-input { display: flex; flex-shrink: 0; align-items: center; gap: 12px; padding: 0 12px 0 18px; height: 56px; border-bottom: 1px solid var(--line); color: var(--ink-3); }
.palette-input input { flex: 1; min-width: 0; height: 100%; border: 0; background: transparent; color: var(--ink); font-size: 16px; }
.palette-input input:focus { box-shadow: none; }
.palette-input input::placeholder { color: var(--ink-3); }
.palette-body { flex: 1; min-height: 0; overflow: auto; padding: 6px; }
.palette-note { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; padding: 14px 12px; font-size: 13.5px; color: var(--ink-2); }
.search-result { display: flex; align-items: center; gap: 12px; width: 100%; min-height: 40px; padding: 0 12px; border: 0; border-radius: 8px; background: transparent; text-align: left; font-size: 14px; }
.search-result .key { width: 104px; flex-shrink: 0; overflow: hidden; text-overflow: ellipsis; }
.search-result strong { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 500; }
.search-result[aria-selected="true"] { background: var(--row-selected); }
.search-result .enter { color: var(--ink-3); }
.more { margin: 8px 12px 6px; }
.palette-foot { display: flex; flex-shrink: 0; gap: 18px; padding: 10px 18px; border-top: 1px solid var(--line); font-size: 12px; color: var(--ink-3); }
.palette-foot span { display: inline-flex; align-items: center; gap: 4px; }
.palette-foot .keycap + .keycap { margin-left: 2px; }
@media (max-width: 600px) {
  .search-palette { margin-top: 8px; }
  .search-result .key { width: 84px; }
  .palette-foot { display: none; }
}
</style>
