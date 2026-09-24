<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { mmText, OFFSET_RANGES, type TextState } from '../../../lib/quotes/inspector'
import type { QuoteEditor } from '../../../lib/quotes/editor'
import type { NumberingOptions, OffsetPatch, QuoteMarker } from '../../../lib/quotes/types'
import MmField from './MmField.vue'
import QuoteIcon from './QuoteIcon.vue'
import SegmentedControl, { type Segment } from './SegmentedControl.vue'

// The Text scope: bold and italic as toggles, the list type as one segmented
// control, the level, numbering options only while the text is numbered (with a
// labelled preview of the number), and the optical offsets of list markers.
// Every control calls the editor's commands on the retained selection.
const props = defineProps<{ editor: QuoteEditor; state: TextState; editable: boolean }>()
const emit = defineEmits<{ run: [command: () => void] }>()
const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)
const mod = mac ? '⌘' : 'Ctrl+'
type Bullet = Exclude<QuoteMarker, 'decimal'>
const LISTS: Segment<'none' | 'bullet' | 'numbered'>[] = [
  { value: 'none', label: 'None', icon: 'list-none', tip: 'Plain paragraphs' },
  { value: 'bullet', label: 'Bullets', icon: 'list-bullet', tip: 'Bulleted list' },
  { value: 'numbered', label: 'Numbers', icon: 'list-numbered', tip: 'Numbered list' },
]
const BULLETS: Segment<Bullet>[] = [
  { value: 'disc', label: 'Dot', glyph: '•' }, { value: 'circle', label: 'Circle', glyph: '◦' },
  { value: 'square', label: 'Square', glyph: '▪' }, { value: 'dash', label: 'Dash', glyph: '–' },
]
const REFERENCES: Segment<'section' | 'own'>[] = [
  { value: 'section', label: 'With section', tip: 'Numbers carry the section number, like 2.1' },
  { value: 'own', label: 'Own count', tip: 'Numbers count on their own, like 1' },
]
const s = computed(() => props.state)
const disabled = computed(() => !props.editable)
const pressed = (value: boolean | 'mixed') => value === 'mixed' ? 'mixed' : value ? 'true' : 'false'
const run = (command: () => void) => emit('run', command)
function setList(kind: 'none' | 'bullet' | 'numbered') {
  const bullet: Bullet = typeof s.value.bullet === 'string' && s.value.bullet !== 'mixed' ? s.value.bullet : 'disc'
  run(() => props.editor.setListMode(kind, kind === 'bullet' ? bullet : undefined))
}
const numbering = (options: NumberingOptions) => run(() => props.editor.setNumbering(options))
const level = computed(() => typeof s.value.depth === 'number' ? s.value.depth + 1 : null)
// "Start at": the number this item shows; typing a number starts the list there.
const startDraft = ref<string | null>(null)
watch(() => [s.value.number, s.value.sectionId], () => { startDraft.value = null })
const startShown = computed(() => startDraft.value ?? (s.value.number != null ? String(s.value.number) : ''))
function commitStart() {
  const raw = startDraft.value
  if (raw === null) return
  startDraft.value = null
  if (!/^\d{1,4}$/.test(raw.trim())) return
  const value = Number(raw)
  if (value < 1 || value > 9999 || value === s.value.number) return
  numbering({ mode: 'start', start: value })
}
function startKeys(event: KeyboardEvent) {
  if (event.key === 'Enter') { event.preventDefault(); commitStart() }
  else if (event.key === 'Escape' && startDraft.value !== null) { event.preventDefault(); event.stopPropagation(); startDraft.value = null }
  else if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
    event.preventDefault()
    const next = Math.max(1, Math.min(9999, (s.value.number ?? 1) + (event.key === 'ArrowUp' ? 1 : -1)))
    if (next !== s.value.number) numbering({ mode: 'start', start: next })
  }
}
const offsets = (patch: OffsetPatch) => run(() => props.editor.setOffsets(patch))
const moved = computed(() => [s.value.markerX, s.value.markerY, s.value.textStart].some(v => v !== null && v !== 0))
const isItem = computed(() => s.value.list === 'bullet' || s.value.list === 'numbered')
</script>

<template>
  <div class="tab-body">
    <section class="group" aria-labelledby="text-style">
      <h3 id="text-style" class="group-title">Style</h3>
      <div class="style-row">
        <div class="toggles" role="group" aria-label="Character style">
          <button type="button" class="tool" :aria-pressed="pressed(s.bold)" :disabled="disabled" :data-tip="`Bold · ${mod}B`" aria-label="Bold" :aria-keyshortcuts="mac ? 'Meta+B' : 'Control+B'" @mousedown.prevent @click="run(() => editor.setMarks('bold'))"><QuoteIcon name="bold" /></button>
          <button type="button" class="tool" :aria-pressed="pressed(s.italic)" :disabled="disabled" :data-tip="`Italic · ${mod}I`" aria-label="Italic" :aria-keyshortcuts="mac ? 'Meta+I' : 'Control+I'" @mousedown.prevent @click="run(() => editor.setMarks('italic'))"><QuoteIcon name="italic" /></button>
        </div>
        <button type="button" class="tool plain" :disabled="disabled || (s.bold === false && s.italic === false)" data-tip="Remove bold and italic" aria-label="Clear formatting" @mousedown.prevent @click="run(() => editor.setMarks('normal'))"><QuoteIcon name="clear-format" /></button>
        <p class="state-note">{{ s.bold === 'mixed' || s.italic === 'mixed' ? 'Mixed styles' : s.collapsed ? 'For what you type next' : 'On the selection' }}</p>
      </div>
    </section>

    <section class="group" aria-labelledby="text-list">
      <h3 id="text-list" class="group-title">List</h3>
      <SegmentedControl :options="LISTS" :model-value="s.list" label="List type" :disabled="disabled" @choose="setList" />
      <SegmentedControl v-if="s.list === 'bullet'" :options="BULLETS" :model-value="s.bullet" label="Bullet" :disabled="disabled" compact @choose="value => run(() => editor.setListMode('bullet', value))" />
      <div class="level-row">
        <span class="level-text">{{ level ? `Level ${level}` : s.depth === 'mixed' ? 'Mixed levels' : 'Not in a list' }}<span v-if="level" class="level-of"> of 6</span></span>
        <div class="level-tools">
          <button type="button" class="tool" :disabled="disabled || !s.canOutdent" :data-tip="s.canOutdent ? 'Outdent · ⇧Tab' : 'Already at the outermost level'" aria-label="Outdent" @mousedown.prevent @click="run(() => editor.outdent())"><QuoteIcon name="outdent" /></button>
          <button type="button" class="tool" :disabled="disabled || !s.canIndent" :data-tip="s.canIndent ? 'Indent · Tab' : 'Already at the deepest level'" aria-label="Indent" @mousedown.prevent @click="run(() => editor.indent())"><QuoteIcon name="indent" /></button>
        </div>
      </div>
    </section>

    <section v-if="s.list === 'numbered'" class="group" aria-labelledby="text-numbering">
      <h3 id="text-numbering" class="group-title">Numbering</h3>
      <SegmentedControl :options="REFERENCES" :model-value="s.bound === 'mixed' || s.bound === null ? 'mixed' : s.bound ? 'section' : 'own'" label="Numbers count" :disabled="disabled" @choose="value => numbering({ mode: value === 'section' ? 'section' : 'independent' })" />
      <div class="sequence">
        <button type="button" class="chip-toggle" :aria-pressed="pressed(s.continued ?? false)" :disabled="disabled" data-tip="Carry on from the list before the last paragraph" @mousedown.prevent @click="numbering({ mode: s.continued === true ? 'follow' : 'continue' })"><QuoteIcon name="list-continue" :size="15" />Continue</button>
        <button type="button" class="chip-toggle" :aria-pressed="pressed(s.restarted ?? false)" :disabled="disabled" data-tip="Start again at 1" @mousedown.prevent @click="numbering({ mode: s.restarted === true ? 'follow' : 'restart' })"><QuoteIcon name="list-restart" :size="15" />Restart</button>
        <label class="start-field"><span>Start at</span>
          <input :value="startShown" class="num-field" inputmode="numeric" autocomplete="off" :disabled="disabled" aria-label="Start numbering at" @input="startDraft = ($event.target as HTMLInputElement).value" @keydown="startKeys" @change="commitStart" @blur="commitStart" />
        </label>
      </div>
      <p class="preview" aria-live="polite"><span class="preview-label">This item reads</span><span class="preview-value">{{ s.preview ?? '—' }}</span></p>
    </section>

    <section v-if="isItem" class="group" aria-labelledby="text-position">
      <div class="group-head">
        <h3 id="text-position" class="group-title">Marker position</h3>
        <button v-if="moved" type="button" class="reset" :disabled="disabled" data-tip="Back to the default position" @mousedown.prevent @click="offsets({ markerX: null, markerY: null, textStart: null })"><QuoteIcon name="reset" :size="13" />Reset</button>
      </div>
      <div class="fields">
        <MmField label="Marker across" :value="s.markerX" :range="OFFSET_RANGES.markerX" :disabled="disabled" @commit="value => offsets({ markerX: mmText(value) })" />
        <MmField label="Marker down" :value="s.markerY" :range="OFFSET_RANGES.markerY" :disabled="disabled" @commit="value => offsets({ markerY: mmText(value) })" />
        <MmField label="Text starts" :value="s.textStart" :range="OFFSET_RANGES.textStart" :disabled="disabled" @commit="value => offsets({ textStart: mmText(value) })" />
      </div>
    </section>
  </div>
</template>

<style scoped>
.tab-body { display: grid; grid-template-columns: minmax(0, 1fr); }
.group { min-width: 0; }
.group { display: grid; gap: 10px; padding: 16px 0; border-top: 1px solid var(--line); }
.group:first-child { border-top: 0; padding-top: 4px; }
.group-head { display: flex; align-items: center; justify-content: space-between; min-height: 22px; }
.group-title { font: 500 10.5px/1.4 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.style-row { display: flex; align-items: center; gap: 8px; }
.toggles { display: inline-flex; gap: 2px; padding: 3px; border-radius: 10px; background: var(--seg-bg); }
.tool { display: grid; place-items: center; width: 34px; height: 30px; padding: 0; border: 0; border-radius: 7px; background: transparent; color: var(--ink-2); }
.toggles .tool { width: 36px; }
@media (hover: hover) { .tool:hover:not(:disabled) { color: var(--ink); background: var(--row-hover); } }
.tool[aria-pressed="true"] { background: var(--seg-on); color: var(--teal-ink); box-shadow: 0 1px 2px rgba(32, 60, 61, .14), inset 0 0 0 1px var(--glass-edge); }
.tool[aria-pressed="mixed"] { color: var(--teal-ink); background: var(--row-selected); }
.tool:focus-visible { box-shadow: var(--focus-ring); }
.tool:disabled { color: var(--ink-3); opacity: .6; cursor: not-allowed; }
.tool.plain { border-radius: 8px; }
.state-note { margin-left: auto; font-size: 11.5px; color: var(--ink-3); text-align: right; }
.level-row { display: flex; align-items: center; justify-content: space-between; min-height: 34px; }
.level-text { font-size: 13px; color: var(--ink); }
.level-of { color: var(--ink-3); }
.level-tools { display: inline-flex; gap: 4px; }
.level-tools .tool { border-radius: 8px; box-shadow: inset 0 0 0 1px var(--line-2); }
.sequence { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
.chip-toggle { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 10px 0 8px; border: 0; border-radius: 8px; background: transparent; box-shadow: inset 0 0 0 1px var(--line-2); color: var(--ink-2); font-size: 12.5px; font-weight: 600; }
@media (hover: hover) { .chip-toggle:hover:not(:disabled) { color: var(--ink); background: var(--row-hover); } }
.chip-toggle[aria-pressed="true"] { background: var(--seg-on); color: var(--teal-ink); box-shadow: inset 0 0 0 1px var(--chip-teal-line); }
.chip-toggle:focus-visible { box-shadow: var(--focus-ring); }
.chip-toggle:disabled { color: var(--ink-3); cursor: not-allowed; }
.start-field { display: inline-flex; align-items: center; gap: 6px; margin-left: auto; font-size: 12.5px; color: var(--ink-2); }
.num-field { width: 56px; height: 30px; padding: 0 8px; border: 1px solid var(--glass-edge); border-radius: 8px; background: var(--field-bg); box-shadow: var(--field-inset), 0 0 0 1px var(--line); color: var(--ink); font: 500 13px/1 var(--mono); text-align: right; }
.num-field:focus { outline: none; box-shadow: var(--focus-ring); }
.preview { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 8px 10px; border-radius: 9px; background: var(--surface-2); }
.preview-label { font-size: 12.5px; color: var(--ink-2); }
.preview-value { font: 600 14px/1 var(--mono); color: var(--ink); font-variant-numeric: tabular-nums; }
.fields { display: grid; gap: 6px; }
.reset { display: inline-flex; align-items: center; gap: 5px; height: 24px; padding: 0 8px; border: 0; border-radius: 999px; background: transparent; color: var(--teal-ink); font-size: 12px; font-weight: 600; }
.reset:hover { background: var(--row-hover); }
.reset:focus-visible { box-shadow: var(--focus-ring); }
</style>
