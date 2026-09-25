<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import type { LocalState } from '../../lib/quoteSession'
import type { PresenceSnapshot } from '../../lib/quotePresence'
import { STATUS_META, type QuoteStatus } from '../../lib/quotes/list'
import type { ZoomMode } from '../../lib/quotes/zoom'
import AppIcon from '../AppIcon.vue'
import FloatingPanel from '../work/FloatingPanel.vue'
import QuotePeople from './collaboration/QuotePeople.vue'
import QuoteIcon from './inspector/QuoteIcon.vue'
import QuoteStatusIcon from './QuoteStatusIcon.vue'
import ZoomControl from './ZoomControl.vue'

// The quote's title bar: the way back (or, docked beside the list, the way out),
// the number, status and save state on the left, zoom in the centre, then who
// else is here, Undo and Redo, PDF with the header chevron right beside it, the
// quote's other actions, Details and the one Format panel toggle. Docked, it
// also opens the quote on its own page; on its own page it can go back beside
// the list with the same session.
const props = defineProps<{
  offerNo: string; status: QuoteStatus; archived: boolean; revising: boolean; local: LocalState | 'loading'; canSave: boolean; canUndo: boolean; canRedo: boolean
  zoom: ZoomMode; percent: number; pane: 'format' | 'details' | null; canFormat: boolean; headerCollapsed: boolean; printing: boolean; compact: boolean
  layout: 'full' | 'dock'; presence: PresenceSnapshot | null; principalId: string; admin: boolean; staff: boolean
}>()
const emit = defineEmits<{
  save: []; undo: []; redo: []; zoom: [mode: ZoomMode]; print: []; toggleHeader: []; pane: [pane: 'format' | 'details']
  close: []; expand: []; collapse: []; duplicate: []; archive: []; copyNumber: []; issue: []
}>()
const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)
// Tooltips spell keys as words, as everywhere else ("Close · Esc").
const mod = mac ? 'Cmd ' : 'Ctrl '
const stateLabel = computed(() => props.revising ? 'Revising' : STATUS_META[props.status].label)
const saveText = computed(() => ({
  loading: 'Opening…', clean: 'Saved', dirty: 'Unsaved changes', saving: 'Saving…', offline: 'Offline, kept here', failed: 'Not saved', conflict: 'Changed elsewhere', 'read-only': 'Read only',
} as Record<string, string>)[props.local] ?? '')
const tone = computed(() => props.local === 'failed' || props.local === 'conflict' ? 'bad' : props.local === 'dirty' || props.local === 'offline' ? 'warn' : '')
const editing = computed(() => props.status === 'draft' && props.local !== 'read-only')
const menu = ref<HTMLElement | null>(null)
const more = ref<HTMLButtonElement>()
function toggleMenu(event: MouseEvent) { menu.value = menu.value ? null : event.currentTarget as HTMLElement }
function closeMenu(restore: boolean) { menu.value = null; if (restore) more.value?.focus() }
function act(action: 'duplicate' | 'archive' | 'copyNumber' | 'issue' | 'expand' | 'collapse') {
  menu.value = null
  if (action === 'duplicate') emit('duplicate'); else if (action === 'archive') emit('archive'); else if (action === 'copyNumber') emit('copyNumber')
  else if (action === 'issue') emit('issue'); else if (action === 'expand') emit('expand'); else emit('collapse')
}
function menuKeys(event: KeyboardEvent) {
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  const items = [...(event.currentTarget as HTMLElement).querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)')]
  const index = items.indexOf(document.activeElement as HTMLButtonElement)
  event.preventDefault()
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : event.key === 'ArrowDown' ? Math.min(items.length - 1, index + 1) : Math.max(0, index - 1)
  items[next]?.focus()
}
</script>

<template>
  <header class="titlebar" :class="{ compact, dock: layout === 'dock' }">
    <div class="left">
      <RouterLink v-if="layout === 'full'" class="icon-btn sm flat back" to="/business/quotes" aria-label="Back to Quotes" data-tip="Quotes"><QuoteIcon name="arrow-left" :size="15" /></RouterLink>
      <div class="identity">
        <p class="number">{{ offerNo || 'Draft quote' }}</p>
        <span class="state-chip" :class="status"><QuoteStatusIcon :status="status" :label="false" :size="12" />{{ stateLabel }}</span>
        <span v-if="archived" class="state-chip">Archived</span>
      </div>
      <div class="save" :class="tone" role="status" aria-live="polite">
        <button
          v-if="editing || local === 'loading'" type="button" class="save-btn" :class="{ ready: canSave }" :disabled="!canSave" aria-label="Save draft" :aria-keyshortcuts="mac ? 'Meta+S' : 'Control+S'"
          :data-tip="canSave ? `Save · ${mod}S` : saveText" @click="emit('save')"
        >
          <svg v-if="local === 'saving'" class="spinner" width="15" height="15" viewBox="0 0 16 16" fill="none" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><circle cx="8" cy="8" r="5.8" class="spin-track" /><path d="M8 2.2a5.8 5.8 0 0 1 5.8 5.8" class="spin-arc" /></svg>
          <QuoteIcon v-else :name="canSave ? 'save' : 'check-circle'" :size="15" />
          <span class="save-text">{{ canSave && local === 'dirty' ? 'Save' : saveText }}</span>
        </button>
        <span v-else class="frozen" :data-tip="status === 'draft' ? 'You can read this draft; an admin or member edits it' : 'Issued versions never change; revise to make a new one'"><AppIcon name="eye" :size="14" /><span class="save-text">Read only</span></span>
        <span v-if="canSave && local === 'dirty' && !compact" class="save-note">Unsaved changes</span>
      </div>
      <div v-if="compact" class="win">
        <QuotePeople :presence="presence" :principal-id="principalId" compact />
        <template v-if="layout === 'dock'">
          <button type="button" class="icon-btn sm flat" aria-label="Open on its own page" data-tip="Open on its own page" @click="emit('expand')"><AppIcon name="expand" :size="15" /></button>
          <button type="button" class="icon-btn sm flat" aria-label="Close the quote" data-tip="Close · Esc" @click="emit('close')"><AppIcon name="close" :size="15" /></button>
        </template>
      </div>
    </div>
    <div class="center"><ZoomControl :mode="zoom" :percent="percent" :compact="compact" @zoom="value => emit('zoom', value)" /></div>
    <div class="right">
      <QuotePeople v-if="!compact" :presence="presence" :principal-id="principalId" />
      <div v-if="editing" class="history" role="group" aria-label="History">
        <button type="button" class="icon-btn sm flat" :disabled="!canUndo" aria-label="Undo" :aria-keyshortcuts="mac ? 'Meta+Z' : 'Control+Z'" :data-tip="`Undo · ${mod}Z`" @mousedown.prevent @click="emit('undo')"><QuoteIcon name="undo" :size="15" /></button>
        <button type="button" class="icon-btn sm flat" :disabled="!canRedo" aria-label="Redo" :aria-keyshortcuts="mac ? 'Meta+Shift+Z' : 'Control+Shift+Z'" :data-tip="`Redo · ${mac ? 'Shift Cmd ' : 'Ctrl Shift '}Z`" @mousedown.prevent @click="emit('redo')"><QuoteIcon name="redo" :size="15" /></button>
      </div>
      <div class="print-pair">
        <button type="button" class="btn sm pdf" :disabled="printing" data-tip="Print or save as PDF" aria-label="PDF" @click="emit('print')"><QuoteIcon name="print" :size="15" /><span class="pdf-text">PDF</span></button>
        <button
          v-if="layout === 'full'" type="button" class="icon-btn sm flat chevron" :aria-expanded="!headerCollapsed" :aria-label="headerCollapsed ? 'Show the app header' : 'Hide the app header'"
          :data-tip="headerCollapsed ? 'Show the app header' : 'Hide the app header'" @click="emit('toggleHeader')"
        ><QuoteIcon :name="headerCollapsed ? 'chevron' : 'chevron-up'" :size="15" /></button>
      </div>
      <button ref="more" type="button" class="icon-btn sm flat" aria-label="More actions" aria-haspopup="menu" :aria-expanded="!!menu" data-tip="More" @click="toggleMenu"><AppIcon name="more" :size="15" /></button>
      <span class="divider" aria-hidden="true" />
      <button type="button" class="btn sm pane-btn" :aria-pressed="pane === 'details'" aria-label="Details" aria-controls="quote-side" data-tip="Status, customer link, versions and files" @click="emit('pane', 'details')">
        <QuoteIcon name="seal" :size="14" /><span class="pane-text">Details</span>
      </button>
      <button v-if="canFormat" type="button" class="icon-btn sm sidebar" :aria-pressed="pane === 'format'" aria-label="Format panel" aria-controls="quote-side" :data-tip="pane === 'format' ? 'Hide the format panel' : 'Show the format panel'" @click="emit('pane', 'format')"><QuoteIcon name="sidebar" :size="16" /></button>
      <template v-if="layout === 'dock' && !compact">
        <span class="divider" aria-hidden="true" />
        <button type="button" class="icon-btn sm flat" aria-label="Open on its own page" data-tip="Open on its own page" @click="emit('expand')"><AppIcon name="expand" :size="15" /></button>
        <button type="button" class="icon-btn sm flat" aria-label="Close the quote" data-tip="Close · Esc" @click="emit('close')"><AppIcon name="close" :size="15" /></button>
      </template>
    </div>
    <FloatingPanel v-if="menu" :anchor="menu" :width="248" align="end" label="Quote actions" @close="closeMenu">
      <div role="menu" aria-label="Quote actions" @keydown="menuKeys">
        <button v-if="admin && status === 'draft' && !archived" type="button" role="menuitem" class="menu-item" data-autofocus @click="act('issue')"><QuoteIcon name="seal" :size="14" />Issue quote…</button>
        <button v-if="staff" type="button" role="menuitem" class="menu-item" @click="act('duplicate')"><AppIcon name="copy" :size="14" />Duplicate as a new quote</button>
        <button type="button" role="menuitem" class="menu-item" :disabled="!offerNo" @click="act('copyNumber')"><AppIcon name="tag" :size="14" />Copy the quote number</button>
        <button v-if="layout === 'full'" type="button" role="menuitem" class="menu-item" @click="act('collapse')"><AppIcon name="collapse" :size="14" />Show beside the list</button>
        <button v-else type="button" role="menuitem" class="menu-item" @click="act('expand')"><AppIcon name="expand" :size="14" />Open on its own page</button>
        <template v-if="admin">
          <div class="menu-sep" role="separator" />
          <button type="button" role="menuitem" class="menu-item" @click="act('archive')"><AppIcon name="archive" :size="14" />{{ archived ? 'Restore from the archive' : 'Archive' }}</button>
        </template>
      </div>
    </FloatingPanel>
  </header>
</template>

<style scoped>
.titlebar { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); align-items: center; gap: 12px; min-height: 52px; padding: 8px 16px; border-bottom: 1px solid var(--line-2); background: var(--surface-raised-2); backdrop-filter: blur(14px) saturate(1.15); -webkit-backdrop-filter: blur(14px) saturate(1.15); }
.dock { border-radius: var(--radius) var(--radius) 0 0; }
.left, .right { display: flex; align-items: center; gap: 8px; min-width: 0; }
.right { justify-content: flex-end; gap: 6px; }
.center { display: flex; justify-content: center; }
.back { flex-shrink: 0; }
.identity { display: flex; align-items: center; gap: 8px; min-width: 0; }
.number { font: 600 14px/1.2 var(--mono); color: var(--ink); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-variant-ligatures: none; }
.state-chip { display: inline-flex; align-items: center; gap: 5px; flex-shrink: 0; height: 22px; padding: 0 8px 0 6px; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); color: var(--ink-2); font: 600 10px/22px var(--mono); letter-spacing: .06em; text-transform: uppercase; font-variant-ligatures: none; }
.state-chip:not(:has(svg)) { padding-left: 8px; }
.save { display: flex; align-items: center; gap: 8px; min-width: 0; margin-left: 4px; }
.save-btn { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px 0 8px; border: 0; border-radius: 8px; background: transparent; color: var(--ink-2); font-size: 12.5px; font-weight: 600; white-space: nowrap; }
.save-btn:disabled { cursor: default; }
.save-btn.ready { background: var(--seg-on); color: var(--teal-ink); box-shadow: inset 0 0 0 1px var(--chip-teal-line); }
@media (hover: hover) { .save-btn.ready:hover { filter: brightness(1.03); } }
.save-btn:focus-visible { box-shadow: var(--focus-ring); }
.save.bad .save-btn { color: var(--danger); }
.frozen { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 4px; color: var(--ink-2); font-size: 12.5px; font-weight: 600; white-space: nowrap; }
.frozen svg { color: var(--ink-3); }
.save-note { font-size: 12px; color: var(--gold-ink); white-space: nowrap; }
.spinner { animation: spin .8s linear infinite; }
.spin-track { stroke: var(--line-2); }
.spin-arc { stroke: var(--teal); }
@keyframes spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .spinner { animation-duration: 2.4s; } }
.history { display: inline-flex; gap: 2px; padding-right: 6px; margin-right: 2px; border-right: 1px solid var(--line); }
.print-pair { display: inline-flex; align-items: center; gap: 2px; }
.pdf { gap: 6px; }
.chevron { width: 26px; }
.divider { width: 1px; height: 20px; margin: 0 2px; background: var(--line); }
.pane-btn { gap: 6px; padding: 0 10px; font-weight: 600; color: var(--ink-2); }
.pane-btn[aria-pressed="true"], .sidebar[aria-pressed="true"] { background: var(--seg-on); color: var(--teal-ink); box-shadow: inset 0 0 0 1px var(--chip-teal-line); }
.win { display: flex; align-items: center; gap: 2px; margin-left: auto; flex-shrink: 0; }
.menu-item { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 34px; padding: 0 10px; border: 0; border-radius: 8px; background: transparent; color: var(--ink); font-size: 13.5px; text-align: left; }
.menu-item svg { flex-shrink: 0; color: var(--ink-3); }
@media (hover: hover) { .menu-item:hover:not(:disabled) { background: var(--row-hover); } }
.menu-item:focus-visible { background: var(--row-selected); box-shadow: none; outline: none; }
.menu-item:disabled { color: var(--ink-3); cursor: default; }
.menu-sep { height: 1px; margin: 4px 6px; background: var(--line); }
/* Narrow: labels step aside; phones and the docked panel stack the bar in two rows. */
@media (max-width: 1100px) { .save-note { display: none; } }
.compact { grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: "left left" "center right"; row-gap: 6px; padding: 6px 12px 8px; }
.compact .left { grid-area: left; }
.compact .center { grid-area: center; justify-content: flex-start; }
.compact .right { grid-area: right; gap: 4px; }
.compact .pdf-text, .compact .pane-text { display: none; }
.compact .pdf, .compact .pane-btn { width: 32px; padding: 0; justify-content: center; }
.compact .history { padding-right: 4px; }
.compact .divider { display: none; }
</style>
