<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import type { LocalState } from '../../lib/quoteSession'
import type { ZoomMode } from '../../lib/quotes/zoom'
import QuoteIcon from './inspector/QuoteIcon.vue'
import ZoomControl from './ZoomControl.vue'

// The quote's title bar: back to Quotes, the number and save state on the left,
// zoom in the centre, then Undo and Redo (global to the editor), PDF with the
// header chevron right beside it, and the one toggle for the format panel.
const props = defineProps<{
  offerNo: string; quoteState: string; local: LocalState | 'loading'; canSave: boolean; canUndo: boolean; canRedo: boolean
  zoom: ZoomMode; percent: number; inspectorOpen: boolean; headerCollapsed: boolean; printing: boolean; compact: boolean
}>()
const emit = defineEmits<{ save: []; undo: []; redo: []; zoom: [mode: ZoomMode]; print: []; toggleHeader: []; toggleInspector: [] }>()
const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)
const mod = mac ? '⌘' : 'Ctrl+'
const STATES: Record<string, string> = { draft: 'Draft', issued: 'Issued', accepted: 'Accepted', declined: 'Declined', expired: 'Expired', archived: 'Archived' }
const stateLabel = computed(() => STATES[props.quoteState] ?? props.quoteState.replace(/^./, c => c.toUpperCase()))
const saveText = computed(() => ({
  loading: 'Opening…', clean: 'Saved', dirty: 'Unsaved changes', saving: 'Saving…', offline: 'Offline, kept here', failed: 'Not saved', conflict: 'Changed elsewhere', 'read-only': 'Read only',
} as Record<string, string>)[props.local] ?? '')
const tone = computed(() => props.local === 'failed' || props.local === 'conflict' ? 'bad' : props.local === 'dirty' || props.local === 'offline' ? 'warn' : '')
</script>

<template>
  <header class="titlebar" :class="{ compact }">
    <div class="left">
      <RouterLink class="icon-btn sm flat back" to="/business/quotes" aria-label="Back to Quotes" data-tip="Quotes"><QuoteIcon name="arrow-left" :size="15" /></RouterLink>
      <div class="identity">
        <p class="number">{{ offerNo || 'Draft quote' }}</p>
        <span class="state-chip">{{ stateLabel }}</span>
      </div>
      <div class="save" :class="tone" role="status" aria-live="polite">
        <button
          type="button" class="save-btn" :class="{ ready: canSave }" :disabled="!canSave" aria-label="Save draft" :aria-keyshortcuts="mac ? 'Meta+S' : 'Control+S'"
          :data-tip="canSave ? `Save · ${mod}S` : saveText" @click="emit('save')"
        >
          <svg v-if="local === 'saving'" class="spinner" width="15" height="15" viewBox="0 0 16 16" fill="none" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><circle cx="8" cy="8" r="5.8" class="spin-track" /><path d="M8 2.2a5.8 5.8 0 0 1 5.8 5.8" class="spin-arc" /></svg>
          <QuoteIcon v-else :name="canSave ? 'save' : 'check-circle'" :size="15" />
          <span class="save-text">{{ canSave && local === 'dirty' ? 'Save' : saveText }}</span>
        </button>
        <span v-if="canSave && local === 'dirty' && !compact" class="save-note">Unsaved changes</span>
      </div>
    </div>
    <div class="center"><ZoomControl :mode="zoom" :percent="percent" :compact="compact" @zoom="value => emit('zoom', value)" /></div>
    <div class="right">
      <div class="history" role="group" aria-label="History">
        <button type="button" class="icon-btn sm flat" :disabled="!canUndo" aria-label="Undo" :aria-keyshortcuts="mac ? 'Meta+Z' : 'Control+Z'" :data-tip="`Undo · ${mod}Z`" @mousedown.prevent @click="emit('undo')"><QuoteIcon name="undo" :size="15" /></button>
        <button type="button" class="icon-btn sm flat" :disabled="!canRedo" aria-label="Redo" :aria-keyshortcuts="mac ? 'Meta+Shift+Z' : 'Control+Shift+Z'" :data-tip="`Redo · ${mac ? '⇧⌘' : 'Ctrl+Shift+'}Z`" @mousedown.prevent @click="emit('redo')"><QuoteIcon name="redo" :size="15" /></button>
      </div>
      <div class="print-pair">
        <button type="button" class="btn sm pdf" :disabled="printing" data-tip="Print or save as PDF" aria-label="PDF" @click="emit('print')"><QuoteIcon name="print" :size="15" /><span class="pdf-text">PDF</span></button>
        <button
          type="button" class="icon-btn sm flat chevron" :aria-expanded="!headerCollapsed" :aria-label="headerCollapsed ? 'Show the app header' : 'Hide the app header'"
          :data-tip="headerCollapsed ? 'Show the app header' : 'Hide the app header'" @click="emit('toggleHeader')"
        ><QuoteIcon :name="headerCollapsed ? 'chevron' : 'chevron-up'" :size="15" /></button>
      </div>
      <button type="button" class="icon-btn sm sidebar" :aria-pressed="inspectorOpen" aria-label="Format panel" aria-controls="quote-inspector" :data-tip="inspectorOpen ? 'Hide the format panel' : 'Show the format panel'" @click="emit('toggleInspector')"><QuoteIcon name="sidebar" :size="16" /></button>
    </div>
  </header>
</template>

<style scoped>
.titlebar { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); align-items: center; gap: 12px; min-height: 52px; padding: 8px 16px; border-bottom: 1px solid var(--line-2); background: var(--surface-raised-2); backdrop-filter: blur(14px) saturate(1.15); -webkit-backdrop-filter: blur(14px) saturate(1.15); }
.left, .right { display: flex; align-items: center; gap: 8px; min-width: 0; }
.right { justify-content: flex-end; }
.center { display: flex; justify-content: center; }
.back { flex-shrink: 0; }
.identity { display: flex; align-items: center; gap: 8px; min-width: 0; }
.number { font: 600 14px/1.2 var(--mono); color: var(--ink); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-variant-ligatures: none; }
.state-chip { flex-shrink: 0; height: 20px; padding: 0 8px; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); color: var(--ink-2); font: 600 10px/20px var(--mono); letter-spacing: .06em; text-transform: uppercase; font-variant-ligatures: none; }
.save { display: flex; align-items: center; gap: 8px; min-width: 0; margin-left: 4px; }
.save-btn { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px 0 8px; border: 0; border-radius: 8px; background: transparent; color: var(--ink-2); font-size: 12.5px; font-weight: 600; white-space: nowrap; }
.save-btn:disabled { cursor: default; }
.save-btn.ready { background: var(--seg-on); color: var(--teal-ink); box-shadow: inset 0 0 0 1px var(--chip-teal-line); }
@media (hover: hover) { .save-btn.ready:hover { filter: brightness(1.03); } }
.save-btn:focus-visible { box-shadow: var(--focus-ring); }
.save.bad .save-btn { color: var(--danger); }
.save-note { font-size: 12px; color: var(--gold-ink); white-space: nowrap; }
.spinner { animation: spin .8s linear infinite; }
.spin-track { stroke: var(--line-2); }
.spin-arc { stroke: var(--teal); }
@keyframes spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .spinner { animation-duration: 2.4s; } }
.history { display: inline-flex; gap: 2px; padding-right: 8px; margin-right: 2px; border-right: 1px solid var(--line); }
.print-pair { display: inline-flex; align-items: center; gap: 2px; }
.pdf { gap: 6px; }
.chevron { width: 26px; }
.sidebar[aria-pressed="true"] { background: var(--seg-on); color: var(--teal-ink); box-shadow: inset 0 0 0 1px var(--chip-teal-line); }
/* Narrow: labels step aside; phones stack the bar in two rows (see .compact). */
@media (max-width: 1100px) { .save-note { display: none; } }
.compact { grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: "left left" "center right"; row-gap: 6px; padding: 6px 12px 8px; }
.compact .left { grid-area: left; }
.compact .center { grid-area: center; justify-content: flex-start; }
.compact .right { grid-area: right; gap: 4px; }
.compact .pdf-text { display: none; }
.compact .pdf { width: 32px; padding: 0; justify-content: center; }
.compact .history { padding-right: 4px; }
.compact .save { margin-left: auto; }
</style>
