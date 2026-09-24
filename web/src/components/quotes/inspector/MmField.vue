<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { clampMm, formatMm, parseMm, scrubMm, stepMm, type MmRange } from '../../../lib/quotes/inspector'

// A millimetre value: the unit sits inside the field, arrow keys step 0.5 mm
// (Shift: 5 mm), dragging the label scrubs the value, Enter or leaving the field
// commits what was typed, Escape brings the stored value back.
const props = defineProps<{ label: string; value: number | 'mixed' | null; range: MmRange; disabled?: boolean; hint?: string }>()
const emit = defineEmits<{ commit: [value: number] }>()
const id = useId()
const draft = ref<string | null>(null)
const scrub = ref<{ start: number; x: number; moved: boolean; value: number; pointer: number } | null>(null)
// A drag that ends on the label is not also a click that focuses the field.
let dragged = false
const current = computed(() => typeof props.value === 'number' ? props.value : 0)
const shown = computed(() => scrub.value ? formatMm(scrub.value.value) : draft.value ?? (props.value === 'mixed' ? '' : formatMm(current.value)))
const invalid = computed(() => draft.value !== null && draft.value.trim() !== '' && (parseMm(draft.value) === null || parseMm(draft.value)! < props.range.min || parseMm(draft.value)! > props.range.max))
const rangeText = computed(() => `${formatMm(props.range.min)} to ${formatMm(props.range.max)} mm`)

function commitDraft() {
  if (draft.value === null) return
  const parsed = parseMm(draft.value)
  if (parsed === null || parsed < props.range.min || parsed > props.range.max) return
  draft.value = null
  if (parsed !== current.value || props.value === 'mixed') emit('commit', parsed)
}
function keys(event: KeyboardEvent) {
  if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
    event.preventDefault()
    const base = draft.value !== null ? parseMm(draft.value) ?? current.value : current.value
    draft.value = null
    const next = stepMm(base, event.key === 'ArrowUp' ? 1 : -1, event.shiftKey, props.range)
    if (next !== current.value || props.value === 'mixed') emit('commit', next)
  } else if (event.key === 'Enter') { event.preventDefault(); commitDraft() }
  else if (event.key === 'Escape' && draft.value !== null) { event.preventDefault(); event.stopPropagation(); draft.value = null }
}
// Scrubbing: press on the label and drag sideways; the stored value changes once, on release.
function scrubStart(event: PointerEvent) {
  if (props.disabled || event.button !== 0) return
  scrub.value = { start: current.value, x: event.clientX, moved: false, value: current.value, pointer: event.pointerId }
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}
function scrubMove(event: PointerEvent) {
  const s = scrub.value
  if (!s) return
  const dx = event.clientX - s.x
  if (Math.abs(dx) > 3) s.moved = true
  s.value = scrubMm(s.start, dx, event.shiftKey, props.range)
}
function scrubEnd(event: PointerEvent) {
  const s = scrub.value
  scrub.value = null
  if (!s) return
  dragged = s.moved
  if (s.moved) { event.preventDefault(); if (s.value !== current.value) emit('commit', clampMm(s.value, props.range)) }
}
function labelClick(event: MouseEvent) { if (dragged) { event.preventDefault(); dragged = false } }
</script>

<template>
  <div class="mm-row" :class="{ disabled }">
    <label
      :for="id" class="mm-label" :class="{ scrubbing: !!scrub }" :data-tip="disabled ? undefined : `Drag to adjust · ${rangeText}`"
      @pointerdown="scrubStart" @pointermove="scrubMove" @pointerup="scrubEnd" @pointercancel="scrub = null" @click="labelClick"
    >{{ label }}</label>
    <div class="mm-input" :class="{ invalid }">
      <input
        :id="id" class="mm-field" type="text" inputmode="decimal" autocomplete="off" spellcheck="false" role="spinbutton"
        :value="shown" :placeholder="value === 'mixed' ? 'Mixed' : undefined" :disabled="disabled"
        :aria-valuenow="value === 'mixed' ? undefined : current" :aria-valuemin="range.min" :aria-valuemax="range.max"
        :aria-valuetext="value === 'mixed' ? 'Mixed' : `${formatMm(current)} millimetres`" :aria-invalid="invalid || undefined" :aria-describedby="hint ? `${id}-hint` : undefined"
        @input="draft = ($event.target as HTMLInputElement).value" @keydown="keys" @change="commitDraft" @blur="commitDraft(); draft = null"
      />
      <span class="mm-unit" aria-hidden="true">mm</span>
    </div>
    <p v-if="hint || invalid" :id="`${id}-hint`" class="mm-hint" :class="{ bad: invalid }">{{ invalid ? `Use ${rangeText}.` : hint }}</p>
  </div>
</template>

<style scoped>
.mm-row { display: grid; grid-template-columns: minmax(0, 1fr) 112px; align-items: center; column-gap: 12px; min-height: 34px; }
.mm-label { font-size: 13px; color: var(--ink-2); cursor: ew-resize; user-select: none; touch-action: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mm-label.scrubbing { color: var(--teal-ink); }
.disabled .mm-label { cursor: default; color: var(--ink-3); }
.mm-input { position: relative; }
.mm-field { width: 100%; height: 30px; padding: 0 34px 0 10px; border: 1px solid var(--glass-edge); border-radius: 8px; background: var(--field-bg); box-shadow: var(--field-inset), 0 0 0 1px var(--line); color: var(--ink); font: 500 13px/1 var(--mono); font-variant-numeric: tabular-nums; text-align: right; }
.mm-field:focus { outline: none; box-shadow: var(--focus-ring); }
.mm-field::placeholder { color: var(--ink-3); font-family: var(--font); }
.mm-field:disabled { color: var(--ink-3); background: var(--surface-2); }
.invalid .mm-field { box-shadow: var(--field-inset), 0 0 0 1px var(--danger-line); }
.mm-unit { position: absolute; right: 10px; top: 50%; transform: translateY(-50%); font: 500 11px/1 var(--mono); color: var(--ink-3); pointer-events: none; }
.mm-hint { grid-column: 1 / -1; margin-top: 4px; font-size: 11.5px; color: var(--ink-3); }
.mm-hint.bad { color: var(--danger); }
</style>
