<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import QuoteAcceptance from './QuoteAcceptance.vue'
import QuoteCover from './QuoteCover.vue'
import QuotePositions from './QuotePositions.vue'
import QuoteProse from './QuoteProse.vue'
import QuoteText from './QuoteText.vue'
import { QuoteEditor } from '../../../lib/quotes/editor'
import { fitWholeBlocks, type PaginationResult, type PagePlan } from '../../../lib/quotes/layout'
import type { QuoteDocumentData, DocumentSettings, ListMode, MarkName, NumberingOptions, OffsetPatch, QuoteMarker, SectionSettingsPatch } from '../../../lib/quotes/types'
const props = withDefaults(defineProps<{ document: QuoteDocumentData; offerNo?: string; editable?: boolean; accepted?: { name: string; company?: string; at: string; digest: string } | null }>(), { editable: false, offerNo: '' })
const emit = defineEmits<{ 'update:document': [document: QuoteDocumentData]; change: [document: QuoteDocumentData]; 'render-state': [state: PaginationResult]; overflow: [message: string | null] }>()
const editor = new QuoteEditor(props.document)
const state = shallowRef(editor.document)
let lastEmitted: QuoteDocumentData | null = null
editor.onChange = document => { state.value = document; lastEmitted = document; emit('update:document', document); emit('change', document); schedule() }
watch(() => props.document, document => { if (document !== lastEmitted) { editor.replaceDocument(document); lastEmitted = editor.document; schedule() } })
const pages = ref<PagePlan[]>([{ kind: 'cover', sectionIds: [], positionIds: [], acceptance: false }, { kind: 'positions', sectionIds: [], positionIds: [], acceptance: true }])
const renderState = ref<PaginationResult>({ ready: false, overflow: null, pages: pages.value })
const measureRoot = ref<HTMLElement>()
const heightProbe = ref<HTMLElement>()
let resize: ResizeObserver | null = null
let scheduled = false
let generation = 0
const sections = computed(() => state.value.sections)
const byId = (id: string) => sections.value.find(s => s.id === id)!
function sectionIndex(id: string) { return sections.value.findIndex(s => s.id === id) }
async function measure() {
  const current = ++generation
  await nextTick()
  const root = measureRoot.value, probe = heightProbe.value
  if (!root || !probe) return
  await document.fonts.ready
  if (current !== generation) return
  const rect = (selector: string) => root.querySelector<HTMLElement>(selector)?.getBoundingClientRect().height ?? 0
  const available = probe.getBoundingClientRect().height
  if (available <= 0) return
  const cover = rect('[data-measure-cover]')
  const sectionHeights = state.value.sections.map(section => ({ id: section.id, px: rect(`[data-measure-section="${section.id}"]`) }))
  const positionHeights = state.value.positions.map(position => ({ id: position.id, px: rect(`[data-measure-position="${position.id}"] tbody`) }))
  const next = fitWholeBlocks(cover, sectionHeights, positionHeights, rect('[data-measure-acceptance]'), available)
  pages.value = next.pages
  renderState.value = next
  emit('render-state', next)
  emit('overflow', next.overflow)
}
function schedule() {
  renderState.value = { ...renderState.value, ready: false, overflow: null }
  if (scheduled) return
  scheduled = true
  void nextTick().then(() => { scheduled = false; void measure() })
}
watch(state, schedule)
onMounted(() => { resize = new ResizeObserver(schedule); if (measureRoot.value) resize.observe(measureRoot.value); window.addEventListener('resize', schedule); schedule() })
onBeforeUnmount(() => { resize?.disconnect(); window.removeEventListener('resize', schedule) })
function moveSection(id: string, direction: number) { const index = sectionIndex(id); editor.moveSection(id, index + direction) }
const menu = ref<string | null>(null)
const announcement = ref('')
const dragId = ref<string | null>(null)
function removeSection(id: string) { editor.deleteSection(id); menu.value = null; announcement.value = 'Section deleted. Undo is available.' }
function dropSection(id: string) { if (dragId.value && dragId.value !== id) editor.moveSection(dragId.value, sectionIndex(id)); dragId.value = null }
function whenReady(): Promise<PaginationResult> {
  return new Promise(resolve => {
    const poll = () => { if (renderState.value.ready || renderState.value.overflow) resolve(renderState.value); else requestAnimationFrame(poll) }
    poll()
  })
}
// Typed P4/P5 integration surface; this component owns all document mutation and history.
defineExpose({
  editor, renderState, whenReady, select: editor.select.bind(editor), undo: editor.undo.bind(editor), redo: editor.redo.bind(editor),
  setMarks: (mark: MarkName | 'normal', active?: boolean) => editor.setMarks(mark, active),
  setListMode: (mode: ListMode, bullet?: Exclude<QuoteMarker, 'decimal'>) => editor.setListMode(mode, bullet),
  indent: () => editor.indent(), outdent: () => editor.outdent(),
  setNumbering: (options: NumberingOptions) => editor.setNumbering(options),
  setOffsets: (patch: OffsetPatch) => editor.setOffsets(patch),
  insertSection: (afterId?: string) => editor.insertSection(afterId),
  moveSection: (id: string, targetIndex: number) => editor.moveSection(id, targetIndex),
  deleteSection: (id: string) => editor.deleteSection(id),
  setSectionSettings: (id: string, patch: SectionSettingsPatch) => editor.setSectionSettings(id, patch),
  setDocumentSettings: (patch: DocumentSettings) => editor.setDocumentSettings(patch),
})
</script>
<template>
  <div class="quote-document" :data-quote-ready="renderState.ready" :data-quote-overflow="renderState.overflow ?? undefined" :data-page-count="pages.length">
    <div v-if="renderState.overflow" class="quote-overflow" role="alert">{{ renderState.overflow }}</div>
    <div v-if="announcement" class="quote-announcement" role="status">{{ announcement }} <button v-if="editable" type="button" @click="editor.undo(); announcement = ''">Undo</button></div>
    <article v-for="(page, pageIndex) in pages" :key="pageIndex" class="quote-page" :data-page="pageIndex + 1">
      <div class="quote-page-content">
        <QuoteCover v-if="page.kind === 'cover'" :document="state" :editor="editor" :offer-no="offerNo" :editable="editable" />
        <div v-for="id in page.sectionIds" :key="id" class="quote-section" :data-section-id="id" @dragover.prevent @drop.prevent="dropSection(id)">
          <div class="quote-section-heading"><span class="quote-section-number">{{ sectionIndex(id) + 1 }}.</span><QuoteText tag="h2" :model-value="byId(id).heading" :label="`Heading section ${sectionIndex(id) + 1}`" :editable="editable" @focus="editor.select({ sectionId: id })" @update:model-value="editor.editSection(id, { heading: $event })" />
            <button v-if="editable" type="button" class="quote-section-menu-button" draggable="true" :aria-label="`Section ${sectionIndex(id) + 1} actions and drag handle`" :aria-expanded="menu === id" @dragstart="dragId = id" @click="menu = menu === id ? null : id"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="5" cy="12" r="1.5"/><circle cx="12" cy="12" r="1.5"/><circle cx="19" cy="12" r="1.5"/></svg></button>
          </div>
          <div v-if="menu === id" class="quote-section-menu"><button type="button" @click="editor.insertSection(id); menu = null">Add section below</button><button type="button" :disabled="sectionIndex(id) === 0" @click="moveSection(id, -1); menu = null">Move up</button><button type="button" :disabled="sectionIndex(id) === sections.length - 1" @click="moveSection(id, 1); menu = null">Move down</button><button type="button" @click="removeSection(id)">Delete section</button></div>
          <QuoteProse :editor="editor" :section-id="id" :body="byId(id).body" :nodes="byId(id).nodes" :section-number="sectionIndex(id) + 1" :editable="editable" @move-section="moveSection(id, $event)" />
        </div>
        <QuotePositions v-if="page.kind === 'positions' && (page.positionIds.length || pageIndex === pages.findIndex(p => p.kind === 'positions'))" :positions="state.positions" :indices="page.positionIds" :editor="editor" :editable="editable" />
        <QuoteAcceptance v-if="page.acceptance" :document="state" :editor="editor" :editable="editable" :accepted="accepted" />
      </div>
      <footer class="quote-page-footer"><span>{{ state.sender.company }}</span><span>{{ offerNo }} · {{ pageIndex + 1 }} / {{ pages.length }}</span></footer>
    </article>
    <div ref="measureRoot" class="quote-measure" aria-hidden="true" inert>
      <div ref="heightProbe" class="quote-height-probe"></div>
      <div data-measure-cover><QuoteCover :document="state" :editor="editor" :offer-no="offerNo" /></div>
      <div v-for="(section, index) in sections" :key="section.id" :data-measure-section="section.id" class="quote-section"><div class="quote-section-heading"><span class="quote-section-number">{{ index + 1 }}.</span><h2>{{ section.heading }}</h2></div><QuoteProse :editor="editor" :section-id="section.id" :body="section.body" :nodes="section.nodes" :section-number="index + 1" /></div>
      <div v-for="position in state.positions" :key="position.id" :data-measure-position="position.id"><QuotePositions :positions="state.positions" :indices="[position.id]" :editor="editor" /></div>
      <div data-measure-acceptance><QuoteAcceptance :document="state" :editor="editor" :accepted="accepted" /></div>
    </div>
  </div>
</template>
<style>
.quote-document { --quote-paper: var(--surface-raised); --quote-ink: var(--ink); color: var(--quote-ink); font-family: var(--font); font-size: 9pt; }
.quote-page { position: relative; box-sizing: border-box; width: 210mm; height: 297mm; padding: 20mm 21mm 20mm; margin: 0 auto 12mm; background: var(--quote-paper); box-shadow: var(--shadow); overflow: hidden; }
.quote-page-content { height: 245mm; }
.quote-page-footer { position: absolute; bottom: 12mm; left: 21mm; right: 21mm; border-top: 1px solid var(--line-2); padding-top: 3mm; display: flex; justify-content: space-between; gap: 10mm; font-size: 7pt; color: var(--ink-2); }
.quote-section { position: relative; margin-top: 6mm; break-inside: avoid; }.quote-section-heading { display: flex; gap: 2mm; align-items: baseline; margin-bottom: 3mm; font-size: 13pt; font-weight: 700; }.quote-section-heading h2 { font: inherit; margin: 0; flex: 1; }.quote-section-number { color: var(--ink-2); }
.quote-section-menu-button { opacity: 0; background: transparent; border: 1px solid var(--line-2); border-radius: 6px; color: var(--ink); width: 7mm; height: 7mm; padding: 1mm; cursor: pointer; }.quote-section:hover .quote-section-menu-button,.quote-section-menu-button:focus-visible { opacity: 1; }.quote-section-menu-button svg { width: 100%; fill: currentColor; }.quote-section-menu { position: absolute; z-index: 3; right: 0; top: 8mm; display: grid; min-width: 42mm; padding: 2mm; background: var(--surface-raised); border: 1px solid var(--line-2); border-radius: 7px; box-shadow: var(--shadow-pop); }.quote-section-menu button { border: 0; background: transparent; color: var(--ink); text-align: left; padding: 2mm; cursor: pointer; }.quote-section-menu button:hover { background: var(--row-hover); }.quote-section-menu button:disabled { opacity: .4; cursor: default; }
.quote-overflow,.quote-announcement { max-width: 210mm; margin: 0 auto 4mm; background: var(--danger-bg); border: 1px solid var(--danger-line); border-radius: 8px; padding: 3mm; }.quote-announcement { background: var(--surface-raised); border-color: var(--line-2); }
.quote-measure { position: absolute; left: -10000px; top: 0; width: 210mm; padding: 20mm 21mm; box-sizing: border-box; visibility: hidden; background: var(--quote-paper); }.quote-height-probe { height: 245mm; position: absolute; pointer-events: none; }
@media print { .quote-page { margin: 0; box-shadow: none; break-after: page; background: white; color: #203c3d; } .quote-document { color: #203c3d; } .quote-section-menu-button,.quote-overflow,.quote-announcement,.quote-measure { display: none !important; } @page { size: A4; margin: 0; } }
</style>
