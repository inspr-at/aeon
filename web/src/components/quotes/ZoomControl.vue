<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import { parseZoom, stepZoom, ZOOM_MAX, ZOOM_MIN, ZOOM_STEPS, zoomLabel, zoomModeName, type ZoomMode } from '../../lib/quotes/zoom'
import FloatingPanel from '../work/FloatingPanel.vue'
import QuoteIcon from './inspector/QuoteIcon.vue'

// Compact zoom: minus and plus together, the current value in between, and a
// small popover with the presets in one grid, the two fit modes and a typed
// percent, all visible at once (no scrolling list).
const props = defineProps<{ mode: ZoomMode; percent: number; compact?: boolean }>()
const emit = defineEmits<{ zoom: [mode: ZoomMode] }>()
const anchor = ref<HTMLElement | null>(null)
const face = ref<HTMLButtonElement>()
const typed = ref('')
const bad = ref(false)
const label = computed(() => zoomLabel(props.mode, props.percent))
const down = computed(() => stepZoom(props.percent, -1))
const up = computed(() => stepZoom(props.percent, 1))
function toggle(event: MouseEvent) { anchor.value = anchor.value ? null : event.currentTarget as HTMLElement; typed.value = ''; bad.value = false }
function close(restore: boolean) { anchor.value = null; if (restore) face.value?.focus() }
function pick(mode: ZoomMode) { emit('zoom', mode); close(true) }
function commitTyped() {
  const value = parseZoom(typed.value)
  if (value === null) { bad.value = true; return }
  pick(value)
}
function gridKeys(event: KeyboardEvent) {
  const buttons = [...(event.currentTarget as HTMLElement).querySelectorAll<HTMLButtonElement>('button')]
  const at = buttons.indexOf(document.activeElement as HTMLButtonElement)
  const move: Record<string, number> = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: 4, ArrowUp: -4 }
  if (!(event.key in move) || at < 0) return
  event.preventDefault(); event.stopPropagation()
  buttons[Math.max(0, Math.min(buttons.length - 1, at + move[event.key]!))]?.focus()
}
</script>

<template>
  <div class="zoom" :class="{ compact }" role="group" aria-label="Zoom">
    <div class="steps">
      <button type="button" class="step" :disabled="down === null" :aria-label="`Zoom out${down ? ` to ${down} %` : ''}`" data-tip="Zoom out" @click="down !== null && emit('zoom', down)"><QuoteIcon name="minus" :size="14" /></button>
      <button type="button" class="step" :disabled="up === null" :aria-label="`Zoom in${up ? ` to ${up} %` : ''}`" data-tip="Zoom in" @click="up !== null && emit('zoom', up)"><QuoteIcon name="plus" :size="14" /></button>
    </div>
    <button ref="face" type="button" class="face" aria-haspopup="dialog" :aria-expanded="!!anchor" :aria-label="`Zoom ${label}${typeof mode === 'string' ? `, ${zoomModeName(mode).toLowerCase()}` : ''}`" data-tip="Zoom" @click="toggle">
      <span class="face-value">{{ label }}</span><QuoteIcon name="chevron" :size="12" class="face-chev" />
    </button>
    <FloatingPanel v-if="anchor" :anchor="anchor" :width="264" label="Zoom" @close="close">
      <div class="zoom-pop">
        <div class="fits">
          <button type="button" class="fit" :aria-pressed="mode === 'width'" data-autofocus @click="pick('width')"><QuoteIcon name="fit-width" :size="15" />Fit width</button>
          <button type="button" class="fit" :aria-pressed="mode === 'page'" @click="pick('page')"><QuoteIcon name="fit-page" :size="15" />Fit page</button>
        </div>
        <div class="presets" role="group" aria-label="Zoom presets" @keydown="gridKeys">
          <button v-for="step in ZOOM_STEPS" :key="step" type="button" class="preset" :aria-pressed="mode === step" @click="pick(step)">{{ step }}</button>
        </div>
        <form class="typed" @submit.prevent="commitTyped">
          <label class="typed-label" for="zoom-typed">Other</label>
          <span class="typed-box">
            <input id="zoom-typed" v-model="typed" class="typed-field" inputmode="numeric" autocomplete="off" :placeholder="String(percent)" :aria-invalid="bad || undefined" aria-describedby="zoom-range" @input="bad = false" />
            <span class="typed-unit" aria-hidden="true">%</span>
          </span>
          <button type="submit" class="btn sm">Set</button>
        </form>
        <p id="zoom-range" class="range" :class="{ bad }">{{ bad ? `Use a whole percent from ${ZOOM_MIN} to ${ZOOM_MAX}.` : `Any whole percent from ${ZOOM_MIN} to ${ZOOM_MAX}.` }}</p>
      </div>
    </FloatingPanel>
  </div>
</template>

<style scoped>
.zoom { display: inline-flex; align-items: center; gap: 2px; padding: 2px; border-radius: 10px; background: var(--seg-bg); }
.steps { display: inline-flex; gap: 2px; }
.step, .face { display: inline-flex; align-items: center; justify-content: center; height: 28px; border: 0; border-radius: 8px; background: transparent; color: var(--ink-2); }
.step { width: 28px; padding: 0; }
.face { flex-shrink: 0; gap: 4px; min-width: 70px; padding: 0 8px 0 10px; font: 600 12.5px/1 var(--font); font-variant-numeric: tabular-nums; color: var(--ink); white-space: nowrap; }
.face-chev { color: var(--ink-3); }
@media (hover: hover) { .step:hover:not(:disabled), .face:hover { color: var(--ink); background: var(--row-hover); } }
.face[aria-expanded="true"] { background: var(--seg-on); box-shadow: inset 0 0 0 1px var(--glass-edge); }
.step:focus-visible, .face:focus-visible { box-shadow: var(--focus-ring); }
.step:disabled { color: var(--ink-3); opacity: .5; }
.zoom-pop { display: grid; gap: 10px; padding: 4px; }
.fits { display: grid; grid-template-columns: 1fr 1fr; gap: 4px; }
.fit { display: inline-flex; align-items: center; justify-content: center; gap: 6px; height: 32px; border: 0; border-radius: 8px; background: var(--surface-2); color: var(--ink); font-size: 12.5px; font-weight: 600; }
.presets { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 4px; }
.preset { height: 30px; border: 0; border-radius: 8px; background: transparent; box-shadow: inset 0 0 0 1px var(--line); color: var(--ink); font: 500 12.5px/1 var(--mono); font-variant-numeric: tabular-nums; }
@media (hover: hover) { .fit:hover, .preset:hover { background: var(--row-hover); } }
.fit[aria-pressed="true"], .preset[aria-pressed="true"] { background: var(--seg-on); color: var(--teal-ink); box-shadow: inset 0 0 0 1px var(--chip-teal-line); }
.fit:focus-visible, .preset:focus-visible { box-shadow: var(--focus-ring); }
.typed { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 8px; }
.typed-label { font-size: 12.5px; color: var(--ink-2); }
.typed-field { width: 100%; height: 30px; padding: 0 26px 0 8px; border: 1px solid var(--glass-edge); border-radius: 8px; background: var(--field-bg); box-shadow: var(--field-inset), 0 0 0 1px var(--line); color: var(--ink); font: 500 13px/1 var(--mono); text-align: right; }
.typed-field:focus { outline: none; box-shadow: var(--focus-ring); }
.typed-field[aria-invalid="true"] { box-shadow: var(--field-inset), 0 0 0 1px var(--danger-line); }
.typed-box { position: relative; min-width: 0; }
.typed-unit { position: absolute; right: 9px; top: 50%; transform: translateY(-50%); font: 500 11px/1 var(--mono); color: var(--ink-3); pointer-events: none; }
.range { font-size: 11.5px; color: var(--ink-3); }
.range.bad { color: var(--danger); }
.compact .steps { display: none; }
.compact .face { min-width: 60px; }
/* Phones: a 40 px face with a finger's 44 px reach. */
@media (max-width: 600px) {
  .compact.zoom { padding: 0; background: transparent; }
  .compact .face { position: relative; height: 40px; border-radius: 12px; background: var(--seg-bg); }
  .compact .face::before { content: ''; position: absolute; top: 50%; left: 50%; width: max(100%, 44px); height: 44px; transform: translate(-50%, -50%); }
}
</style>
