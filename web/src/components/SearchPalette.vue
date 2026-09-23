<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { searchNodes, type SearchHit } from '../lib/api'
import AppIcon from './AppIcon.vue'
const emit = defineEmits<{ select: [id: string] }>()
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
  opener = document.activeElement as HTMLElement
  dialog.value?.showModal()
  await nextTick()
  input.value?.focus()
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
function choose(id: string) { close(); emit('select', id) }
function keydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    active.value = Math.max(0, Math.min(hits.value.length - 1, active.value + (event.key === 'ArrowDown' ? 1 : -1)))
    document.getElementById(`search-hit-${active.value}`)?.scrollIntoView({ block: 'nearest' })
  } else if (event.key === 'Enter') {
    event.preventDefault()
    const hit = hits.value[active.value]
    if (hit) choose(hit.node.id)
  }
}
onBeforeUnmount(() => { clearTimeout(timer); generation++ })
function refresh() { if (dialog.value?.open && term.value.trim()) void search() }
defineExpose({ open, refresh })
</script>
<template>
  <dialog ref="dialog" class="search-palette" aria-labelledby="search-title" @cancel.prevent="close">
    <header><h2 id="search-title">Search all work</h2><button class="icon-button" aria-label="Close search" @click="close"><AppIcon name="close" /></button></header>
    <label class="search-input"><AppIcon name="search" /><input ref="input" v-model="term" aria-label="Search all work" placeholder="Title, key, or words in a document…" role="combobox" aria-controls="search-results" :aria-expanded="true" :aria-activedescendant="hits.length ? `search-hit-${active}` : undefined" autocomplete="off" @keydown="keydown" /></label>
    <p v-if="error" role="alert">{{ error }} <button class="quiet" @click="search()">Retry search</button></p>
    <p v-else-if="busy" role="status">Searching…</p>
    <p v-else-if="!term.trim()">Search across every kind and branch. Use arrow keys and Enter to open.</p>
    <p v-else-if="!hits.length" role="status">No matching work.</p>
    <div id="search-results" role="listbox" aria-label="Search results">
      <button v-for="(hit, index) in hits" :id="`search-hit-${index}`" :key="hit.node.id" class="search-result" role="option" :aria-selected="index === active" @click="choose(hit.node.id)"><span class="node-key">{{ hit.node.key }}</span><strong>{{ hit.node.title }}</strong><span>{{ hit.node.state }}</span></button>
    </div>
    <button v-if="cursor" class="quiet" :disabled="busy" @click="search(true)">More results</button>
  </dialog>
</template>
<style scoped>
.search-palette { width: min(640px, calc(100vw - 32px)); max-height: 80dvh; margin: 10dvh auto auto; padding: 22px; }
header { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 16px; }
h2 { font-size: 20px; font-weight: 500; }
.search-input { display: flex; align-items: center; gap: 12px; }
.search-input input { flex: 1; min-width: 0; }
p { margin: 16px 0; }
.search-result { display: flex; gap: 12px; text-align: left; width: 100%; padding: 12px; background: transparent; border: 0; border-bottom: 1px solid var(--line); }
.search-result strong { flex: 1; overflow-wrap: anywhere; }
.search-result[aria-selected="true"] { background: var(--aqua-3); }
#search-results { margin-top: 16px; }
</style>
