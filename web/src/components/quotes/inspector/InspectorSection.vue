<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { mmText, sectionLabel, SECTION_STYLES, SPACING_RANGE } from '../../../lib/quotes/inspector'
import type { SectionActions } from '../../../lib/quotes/sectionActions'
import type { QuoteEditor } from '../../../lib/quotes/editor'
import type { QuoteDocumentData, SectionNumberingStyle, SectionSettingsPatch } from '../../../lib/quotes/types'
import MmField from './MmField.vue'
import QuoteIcon from './QuoteIcon.vue'
import SectionMenu from './SectionMenu.vue'
import SegmentedControl, { type Segment } from './SegmentedControl.vue'

// The Section scope: how the chosen section is set (its number style, whether it
// starts a new page, extra space around it) and the outline of all sections,
// where each row can be dragged, moved with Alt+Up/Down or opened for its actions.
const props = defineProps<{ editor: QuoteEditor; document: QuoteDocumentData; sectionId: string | null; editable: boolean; actions: SectionActions }>()
const emit = defineEmits<{ run: [command: () => void]; jump: [sectionId: string] }>()
const section = computed(() => props.document.sections.find(s => s.id === props.sectionId) ?? null)
const index = computed(() => props.document.sections.findIndex(s => s.id === props.sectionId))
const STYLES: Segment<SectionNumberingStyle>[] = SECTION_STYLES.map(style => ({ value: style.id, label: style.name, glyph: style.glyph }))
const set = (patch: SectionSettingsPatch) => { const id = props.sectionId; if (id) emit('run', () => props.editor.setSectionSettings(id, patch)) }
const spaced = computed(() => !!section.value && (Number(section.value.spacing_before_mm ?? 0) !== 0 || Number(section.value.spacing_after_mm ?? 0) !== 0))

// ---------- Outline ----------
const list = ref<HTMLElement>()
const menu = ref<{ id: string; anchor: HTMLElement } | null>(null)
const dragging = ref<string | null>(null)
const dropAt = ref<number | null>(null)
const label = (i: number) => sectionLabel(i + 1, props.document.sections[i]?.numbering_style) || '·'
function focusRow(id: string) { void nextTick(() => list.value?.querySelector<HTMLElement>(`[data-outline="${id}"] .row-main`)?.focus()) }
function rowKeys(event: KeyboardEvent, id: string, i: number) {
  if (event.altKey && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) {
    event.preventDefault()
    if (props.editable && props.actions.move(id, event.key === 'ArrowUp' ? -1 : 1)) focusRow(id)
  } else if (!event.altKey && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) {
    event.preventDefault()
    const next = props.document.sections[i + (event.key === 'ArrowUp' ? -1 : 1)]
    if (next) focusRow(next.id)
  } else if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) {
    event.preventDefault(); openMenu(id, event.currentTarget as HTMLElement)
  }
}
function openMenu(id: string, anchor: HTMLElement) { if (props.editable) menu.value = { id, anchor } }
function closeMenu(restore: boolean) { const m = menu.value; menu.value = null; if (restore && m) m.anchor.focus() }
function dragStart(event: DragEvent, id: string) {
  dragging.value = id
  event.dataTransfer?.setData('text/plain', id)
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
}
function dragOver(event: DragEvent, i: number) {
  if (!dragging.value) return
  event.preventDefault()
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  dropAt.value = event.clientY < rect.top + rect.height / 2 ? i : i + 1
}
function drop() {
  const id = dragging.value, at = dropAt.value
  dragging.value = null; dropAt.value = null
  if (!id || at === null) return
  const from = props.document.sections.findIndex(s => s.id === id)
  props.actions.moveTo(id, at > from ? at - 1 : at)
}
function add(after: string) { const created = props.actions.addBelow(after); if (created) emit('jump', created) }
</script>

<template>
  <div class="tab-body">
    <template v-if="section">
      <section class="group" aria-labelledby="section-number">
        <h3 id="section-number" class="group-title">Number style</h3>
        <SegmentedControl :options="STYLES" :model-value="section.numbering_style ?? 'decimal'" label="Section number style" :disabled="!editable" compact @choose="value => set({ numberingStyle: value })" />
        <p class="note">This section is numbered <strong>{{ sectionLabel(index + 1, section.numbering_style) || 'without a number' }}</strong></p>
      </section>

      <section class="group" aria-labelledby="section-page">
        <h3 id="section-page" class="group-title">Page</h3>
        <label class="switch-row">
          <span class="switch-text"><QuoteIcon name="page-break" :size="15" />Start on a new page</span>
          <span class="switch"><input type="checkbox" :checked="!!section.page_break_before" :disabled="!editable" @change="set({ pageBreakBefore: ($event.target as HTMLInputElement).checked })" /></span>
        </label>
        <p class="note">A section always stays whole on one page; one that does not fit moves to the next.</p>
      </section>

      <section class="group" aria-labelledby="section-spacing">
        <div class="group-head">
          <h3 id="section-spacing" class="group-title">Extra space</h3>
          <button v-if="spaced" type="button" class="reset" :disabled="!editable" data-tip="Back to the normal spacing" @click="set({ spacingBeforeMm: null, spacingAfterMm: null })"><QuoteIcon name="reset" :size="13" />Reset</button>
        </div>
        <div class="fields">
          <MmField label="Before the section" :value="Number(section.spacing_before_mm ?? 0)" :range="SPACING_RANGE" :disabled="!editable" @commit="value => set({ spacingBeforeMm: mmText(value) })" />
          <MmField label="After the section" :value="Number(section.spacing_after_mm ?? 0)" :range="SPACING_RANGE" :disabled="!editable" @commit="value => set({ spacingAfterMm: mmText(value) })" />
        </div>
      </section>
    </template>
    <div v-else class="empty">
      <p>Click a section on the page, or pick one below, to set its number style, page and spacing.</p>
    </div>

    <section class="group" aria-labelledby="section-outline">
      <div class="group-head">
        <h3 id="section-outline" class="group-title">All sections</h3>
        <span class="count">{{ document.sections.length }} of 20</span>
      </div>
      <ol ref="list" class="outline" :class="{ dragging: !!dragging }" aria-labelledby="section-outline" @dragleave.self="dropAt = null">
        <li
          v-for="(s, i) in document.sections" :key="s.id" class="outline-row" :class="{ current: s.id === sectionId, lifted: dragging === s.id, 'drop-before': dropAt === i, 'drop-after': dropAt === i + 1 && i === document.sections.length - 1 }"
          :data-outline="s.id" @dragover="dragOver($event, i)" @drop.prevent="drop"
        >
          <span v-if="editable" class="grip" draggable="true" aria-hidden="true" data-tip="Drag to reorder" @dragstart="dragStart($event, s.id)" @dragend="dragging = null; dropAt = null"><QuoteIcon name="grip" :size="14" /></span>
          <button type="button" class="row-main" :aria-current="s.id === sectionId ? 'true' : undefined" :aria-keyshortcuts="editable ? 'Alt+ArrowUp Alt+ArrowDown' : undefined" @click="emit('jump', s.id)" @keydown="rowKeys($event, s.id, i)" @contextmenu.prevent="openMenu(s.id, $event.currentTarget as HTMLElement)">
            <span class="row-number">{{ label(i) }}</span>
            <span class="row-title" :class="{ untitled: !s.heading.trim() }">{{ s.heading.trim() || 'Untitled section' }}</span>
          </button>
          <button v-if="editable" type="button" class="row-more" :aria-label="`Actions for section ${i + 1}`" aria-haspopup="menu" :aria-expanded="menu?.id === s.id" data-tip="Section actions" @click="openMenu(s.id, $event.currentTarget as HTMLElement)"><QuoteIcon name="more" :size="15" /></button>
        </li>
      </ol>
      <button v-if="editable" type="button" class="add-end" :disabled="document.sections.length >= 20" @click="add(document.sections.at(-1)!.id)"><QuoteIcon name="section-add" :size="15" />Add a section at the end</button>
    </section>
    <SectionMenu
      v-if="menu" :anchor="menu.anchor" :index="document.sections.findIndex(s => s.id === menu!.id)" :count="document.sections.length" :label="`Actions for section ${document.sections.findIndex(s => s.id === menu!.id) + 1}`"
      @close="closeMenu" @add="add(menu!.id)" @up="actions.move(menu!.id, -1)" @down="actions.move(menu!.id, 1)" @remove="actions.remove(menu!.id)"
    />
  </div>
</template>

<style scoped>
.tab-body { display: grid; grid-template-columns: minmax(0, 1fr); }
.group { min-width: 0; }
.group { display: grid; gap: 10px; padding: 16px 0; border-top: 1px solid var(--line); }
.group:first-child { border-top: 0; padding-top: 4px; }
.group-head { display: flex; align-items: center; justify-content: space-between; min-height: 22px; }
.group-title { font: 500 10.5px/1.4 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.count { font: 500 11px/1 var(--mono); color: var(--ink-3); }
.note { font-size: 12px; line-height: 1.5; color: var(--ink-2); }
.note strong { font-weight: 600; color: var(--ink); font-family: var(--mono); }
.switch-row { display: flex; align-items: center; justify-content: space-between; gap: 10px; min-height: 34px; cursor: pointer; }
.switch-text { display: inline-flex; align-items: center; gap: 8px; font-size: 13px; color: var(--ink); }
.switch-text svg { color: var(--ink-3); }
.fields { display: grid; gap: 6px; }
.reset { display: inline-flex; align-items: center; gap: 5px; height: 24px; padding: 0 8px; border: 0; border-radius: 999px; background: transparent; color: var(--teal-ink); font-size: 12px; font-weight: 600; }
.reset:hover { background: var(--row-hover); }
.reset:focus-visible { box-shadow: var(--focus-ring); }
.empty { padding: 4px 0 16px; font-size: 13px; line-height: 1.5; color: var(--ink-2); }
.outline { display: grid; gap: 2px; margin: 0; padding: 0; list-style: none; }
.outline-row { position: relative; display: flex; align-items: center; gap: 2px; min-height: 36px; border-radius: 9px; }
@media (hover: hover) { .outline-row:hover { background: var(--row-hover); } }
.outline-row.current { background: var(--row-selected); }
.outline-row.lifted { opacity: .45; }
/* Where a dragged section will land: a line between rows, drawn as its own element. */
.outline-row.drop-before::before, .outline-row.drop-after::after { content: ''; position: absolute; left: 6px; right: 6px; height: 2px; border-radius: 2px; background: var(--teal); pointer-events: none; }
.outline-row.drop-before::before { top: -2px; }
.outline-row.drop-after::after { bottom: -2px; }
.grip { display: grid; place-items: center; width: 20px; height: 28px; color: var(--ink-3); cursor: grab; opacity: 0; }
.outline-row:hover .grip, .outline-row:focus-within .grip { opacity: 1; }
@media (hover: none) { .grip { opacity: 1; } }
.row-main { display: flex; align-items: baseline; gap: 8px; flex: 1; min-width: 0; height: 36px; padding: 0 6px; border: 0; border-radius: 8px; background: transparent; color: var(--ink); text-align: left; }
.row-main:focus-visible { box-shadow: var(--focus-ring); }
.row-number { flex-shrink: 0; min-width: 22px; font: 500 12px/36px var(--mono); color: var(--ink-3); font-variant-numeric: tabular-nums; }
.row-title { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; line-height: 36px; }
.row-title.untitled { color: var(--ink-3); font-style: italic; }
.current .row-number, .current .row-title { color: var(--teal-ink); }
.row-more { display: grid; place-items: center; flex-shrink: 0; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 7px; background: transparent; color: var(--ink-3); opacity: 0; }
.outline-row:hover .row-more, .outline-row:focus-within .row-more, .row-more[aria-expanded="true"] { opacity: 1; }
@media (hover: none) { .row-more { opacity: 1; } }
.row-more:hover { color: var(--ink); background: var(--btn-bg-hover); }
.row-more:focus-visible { box-shadow: var(--focus-ring); opacity: 1; }
.add-end { display: inline-flex; align-items: center; justify-self: start; gap: 7px; height: 30px; padding: 0 10px; border: 0; border-radius: 8px; background: transparent; color: var(--teal-ink); font-size: 12.5px; font-weight: 600; }
.add-end:hover:not(:disabled) { background: var(--row-hover); }
.add-end:focus-visible { box-shadow: var(--focus-ring); }
.add-end:disabled { color: var(--ink-3); }
</style>
