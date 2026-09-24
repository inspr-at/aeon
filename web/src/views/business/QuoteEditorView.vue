<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { api } from '../../lib/api'
import { headerFolded } from '../../lib/chrome'
import { usePreference } from '../../lib/preferences'
import { QuoteSession, type SessionView } from '../../lib/quoteSession'
import type { ConflictChoices } from '../../lib/quoteMerge'
import { QuotePresence, selectionAnchor, type PresenceSnapshot } from '../../lib/quotePresence'
import type { RecoveryDraft } from '../../lib/quoteRecovery'
import { readZoom, zoomPercent, type ZoomMode } from '../../lib/quotes/zoom'
import { toast } from '../../lib/toast'
import { isTenantAdmin } from '../../components/business/catalog'
import QuoteDocument from '../../components/quotes/editor/QuoteDocument.vue'
import QuoteInspector from '../../components/quotes/inspector/QuoteInspector.vue'
import QuoteIcon from '../../components/quotes/inspector/QuoteIcon.vue'
import QuotePresenceOverlay from '../../components/quotes/collaboration/QuotePresenceOverlay.vue'
import QuoteTitleBar from '../../components/quotes/QuoteTitleBar.vue'
import { useSession } from '../../stores/session'

// The quote editor: the title bar, the paper on a desk you can zoom, and the
// format inspector beside it (docked on wide screens, over the page when it is
// narrower, a sheet on phones). Session, presence and recovery come from QW1;
// the document and its history belong to QuoteDocument (P3).
const props = defineProps<{ quoteId: string }>()
const identity = useSession()
const host = ref<HTMLElement | null>(null)
const desk = ref<HTMLElement | null>(null)
const editor = ref<InstanceType<typeof QuoteDocument> | null>(null)
const quoteSession = shallowRef<QuoteSession | null>(null)
const view = shallowRef<SessionView | null>(null)
const presence = shallowRef<PresenceSnapshot | null>(null)
const recovery = shallowRef<RecoveryDraft | null>(null)
const offerNo = ref('')
const error = ref('')
let quotePresence: QuotePresence | null = null
let unsubscribe: (() => void) | null = null
let generation = 0
const document = computed(() => view.value?.working ?? null)
const canSave = computed(() => view.value?.local === 'dirty' || view.value?.local === 'failed' || view.value?.local === 'offline')
const editable = computed(() => !!view.value && view.value.local !== 'read-only')
const admin = computed(() => isTenantAdmin(identity.identity))

function dispose() {
  unsubscribe?.(); unsubscribe = null
  quotePresence?.stop(); quotePresence = null
  quoteSession.value?.dispose(); quoteSession.value = null
  view.value = null; presence.value = null; recovery.value = null
}
async function open() {
  const current = ++generation
  dispose(); error.value = ''; offerNo.value = ''
  const person = identity.identity
  if (!person) return
  const session = new QuoteSession({ quoteId: props.quoteId, tenantId: person.tenant.id, principalId: person.principal.id })
  quoteSession.value = session
  unsubscribe = session.subscribe(next => { view.value = { ...next } })
  try {
    recovery.value = await session.open()
    if (current !== generation) return
    const response = await api(`/quotes/${encodeURIComponent(props.quoteId)}`)
    if (response.ok) {
      const quote = await response.json() as { offer_no?: string; state?: PresenceSnapshot['state']; revision?: number }
      offerNo.value = quote.offer_no ?? ''
      if (quote.state && quote.state !== 'draft') session.onPresence({ sessions: [], draft_revision: session.view.baseRevision, quote_revision: quote.revision ?? 0, state: quote.state })
    }
    if (current !== generation) return
    quotePresence = new QuotePresence(props.quoteId, person.principal.id,
      snapshot => { presence.value = snapshot; session.onPresence(snapshot) },
      notice => session.onNotice(notice))
    await quotePresence.start(session.view.baseRevision).catch(() => { quotePresence?.stop(); quotePresence = null })
  } catch (cause) {
    if (current === generation) error.value = cause instanceof Error ? cause.message : 'Quote could not be opened.'
  }
}
watch(() => [props.quoteId, identity.identity?.principal.id], () => { void open() }, { immediate: true })
function useRecovery() { if (recovery.value && quoteSession.value) quoteSession.value.restore(recovery.value); recovery.value = null }
async function useServer() { recovery.value = null; await quoteSession.value?.reload(true) }
async function resolveReview(side: 'mine' | 'theirs') {
  const conflicts = view.value?.review?.conflicts ?? []
  const choices = Object.fromEntries(conflicts.map(conflict => [conflict.path, side])) as ConflictChoices
  await quoteSession.value?.acceptReview(choices)
}
function edit(value: NonNullable<typeof document.value>) {
  quoteSession.value?.edit(value)
  const current = quotePresence
  if (current && editor.value) {
    const revision = view.value?.baseRevision ?? 0
    void selectionAnchor(editor.value.editor.selection, value, revision).then(anchor => {
      if (current === quotePresence) current.setSelection(anchor, 'editing', revision)
    })
  }
}

// ---------- The editor's surface for the inspector and title bar ----------
const version = computed(() => editor.value?.version ?? 0)
const canUndo = computed(() => { void version.value; return !!editor.value?.editor.history.canUndo })
const canRedo = computed(() => { void version.value; return !!editor.value?.editor.history.canRedo })
function undo() { editor.value?.editor.undo(); editor.value?.touch() }
function redo() { editor.value?.editor.redo(); editor.value?.touch() }

// ---------- Layout: dock (wide), over the page (narrower), sheet (phones) ----------
const pref = usePreference<{ zoom?: ZoomMode; inspector?: boolean; headerFolded?: boolean }>('quote-editor')
const wideQuery = window.matchMedia('(min-width: 1100px)')
const phoneQuery = window.matchMedia('(max-width: 600px)')
const wide = ref(wideQuery.matches)
const phone = ref(phoneQuery.matches)
const layoutChange = () => { wide.value = wideQuery.matches; phone.value = phoneQuery.matches; floatingOpen.value = false }
const mode = computed<'dock' | 'overlay' | 'sheet'>(() => wide.value ? 'dock' : phone.value ? 'sheet' : 'overlay')
const floatingOpen = ref(false)
const inspectorOpen = computed(() => mode.value === 'dock' ? pref.value.value?.inspector !== false : floatingOpen.value)
function toggleInspector() {
  if (mode.value === 'dock') pref.save({ ...(pref.value.value ?? {}), inspector: !inspectorOpen.value })
  else floatingOpen.value = !floatingOpen.value
}
function toggleHeader() {
  headerFolded.value = !headerFolded.value
  pref.save({ ...(pref.value.value ?? {}), headerFolded: headerFolded.value })
}
void pref.ready.then(() => { headerFolded.value = !!pref.value.value?.headerFolded })

// ---------- Zoom ----------
const deskSize = ref({ width: 1200, height: 800 })
const zoomMode = computed<ZoomMode>(() => readZoom(pref.value.value?.zoom, phone.value ? 'width' : 100))
const percent = computed(() => zoomPercent(zoomMode.value, deskSize.value))
function setZoom(next: ZoomMode) { pref.save({ ...(pref.value.value ?? {}), zoom: next }) }
let sizer: ResizeObserver | undefined
watch(desk, el => { sizer?.disconnect(); if (el) { sizer = new ResizeObserver(([entry]) => { deskSize.value = { width: entry.contentRect.width, height: entry.contentRect.height } }); sizer.observe(el) } })

// ---------- PDF: save first, then the browser's print (or Save as PDF) ----------
const printing = ref(false)
async function print() {
  if (printing.value) return
  printing.value = true
  try {
    if (canSave.value) await quoteSession.value?.save()
    if (view.value?.local === 'failed' || view.value?.local === 'conflict') { toast('Save the quote before printing it.', { tone: 'error' }); return }
    await editor.value?.whenReady()
    await nextTick()
    window.print()
  } finally { printing.value = false }
}

// ---------- Keys: undo and redo are the editor's everywhere on the page ----------
const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)
function keys(event: KeyboardEvent) {
  if (event.defaultPrevented || !editor.value) return
  const target = event.target as HTMLElement
  const field = !!target.closest?.('input, textarea, select')
  const mod = mac ? event.metaKey : event.ctrlKey
  const key = event.key.toLowerCase()
  if (mod && !event.altKey && key === 's') { event.preventDefault(); void quoteSession.value?.save(); return }
  if (field || globalThis.document.querySelector('dialog[open]')) return
  if (mod && !event.altKey && (key === 'z' || key === 'y')) { event.preventDefault(); if (key === 'y' || event.shiftKey) redo(); else undo(); return }
  if (mod && !event.altKey && (key === 'b' || key === 'i') && editable.value) { event.preventDefault(); editor.value.setMarks(key === 'b' ? 'bold' : 'italic'); editor.value.touch() }
  if (event.key === 'Escape' && mode.value !== 'dock' && floatingOpen.value && !globalThis.document.querySelector('.floating')) { floatingOpen.value = false }
}
onMounted(() => {
  window.addEventListener('keydown', keys)
  wideQuery.addEventListener('change', layoutChange); phoneQuery.addEventListener('change', layoutChange)
})
onBeforeUnmount(() => {
  generation++; dispose()
  window.removeEventListener('keydown', keys)
  wideQuery.removeEventListener('change', layoutChange); phoneQuery.removeEventListener('change', layoutChange)
  sizer?.disconnect()
})
function jump(id: string) { editor.value?.jump(id); if (mode.value === 'sheet') floatingOpen.value = false }
</script>

<template>
  <section class="quote-editor-page" :class="[mode, { 'inspector-open': inspectorOpen && !!document }]" aria-label="Quote editor">
    <QuoteTitleBar
      class="quote-titlebar" :offer-no="offerNo" :quote-state="view?.quoteState ?? 'draft'" :local="view?.local ?? 'loading'" :can-save="canSave" :can-undo="canUndo" :can-redo="canRedo"
      :zoom="zoomMode" :percent="percent" :inspector-open="inspectorOpen" :header-collapsed="headerFolded" :printing="printing" :compact="phone"
      @save="quoteSession?.save()" @undo="undo" @redo="redo" @zoom="setZoom" @print="print" @toggle-header="toggleHeader" @toggle-inspector="toggleInspector"
    />
    <div v-if="error || recovery || view?.remote === 'newer' || view?.review" class="quote-notices">
      <p v-if="error" class="notice bad" role="alert"><QuoteIcon name="alert" :size="15" />{{ error }}</p>
      <p v-if="recovery" class="notice" role="alert"><QuoteIcon name="history" :size="15" /><span>Unsaved work from this tab is available.</span><button type="button" class="btn sm" @click="useRecovery">Restore my work</button><button type="button" class="btn sm ghost" @click="useServer">Use saved draft</button></p>
      <p v-if="view?.remote === 'newer'" class="notice" role="status"><QuoteIcon name="refresh" :size="15" /><span>A newer draft is available.</span><button type="button" class="btn sm" @click="quoteSession?.reload()">Review changes</button></p>
      <p v-if="view?.review" class="notice" role="alert"><QuoteIcon name="alert" :size="15" /><span>{{ view.review.conflicts.length ? `${view.review.conflicts.length} concurrent change conflicts need a choice.` : 'These changes can be merged.' }}</span><button type="button" class="btn sm" @click="resolveReview('mine')">{{ view.review.conflicts.length ? 'Keep my changes' : 'Merge changes' }}</button><button v-if="view.review.conflicts.length" type="button" class="btn sm ghost" @click="resolveReview('theirs')">Use newer changes</button></p>
    </div>
    <div class="workspace">
      <div ref="desk" class="quote-desk">
        <div v-if="!document && !error" class="loading" role="status" aria-label="Loading quote"><span class="skeleton page-skeleton" /></div>
        <div v-if="document" class="desk-inner">
          <div ref="host" class="paper-stack" :style="{ zoom: percent / 100 }">
            <QuoteDocument ref="editor" :document="document" :offer-no="offerNo" :editable="editable" @update:document="edit" />
          </div>
        </div>
      </div>
      <div v-if="document && inspectorOpen && editor" id="quote-inspector" class="quote-inspector-slot">
        <QuoteInspector :editor="editor.editor" :version="version" :offer-no="offerNo" :editable="editable" :admin="admin" :actions="editor.actions" :mode="mode" @close="floatingOpen = false" @jump="jump" />
      </div>
      <button v-if="document && inspectorOpen && mode === 'sheet'" type="button" class="sheet-scrim" aria-label="Close format" tabindex="-1" @click="floatingOpen = false" />
    </div>
    <QuotePresenceOverlay v-if="document && identity.identity" :root="host" :document="document" :revision="view?.baseRevision ?? 0" :principal-id="identity.identity.principal.id" :presence="presence" />
  </section>
</template>

<style scoped>
.quote-editor-page { display: flex; flex-direction: column; height: 100%; min-height: 0; }
.quote-notices { display: grid; gap: 6px; padding: 8px 16px 0; }
.notice { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 10px; max-width: 900px; margin: 0 auto; width: 100%; padding: 8px 12px; border-radius: 10px; background: var(--surface-raised-2); box-shadow: inset 0 0 0 1px var(--line-2); font-size: 13px; color: var(--ink); }
.notice svg { color: var(--ink-3); }
.notice span { flex: 1; min-width: 12em; }
.notice.bad { background: var(--danger-bg); box-shadow: inset 0 0 0 1px var(--danger-line); }
.notice.bad svg { color: var(--danger); }
.workspace { position: relative; flex: 1; min-height: 0; display: grid; grid-template-columns: minmax(0, 1fr); }
.inspector-open.dock .workspace { grid-template-columns: minmax(0, 1fr) 320px; }
/* The desk: a quiet surface the paper sits on, scrolling in both directions when zoomed. */
.quote-desk { position: relative; min-width: 0; min-height: 0; overflow: auto; background: var(--surface-sunken); overscroll-behavior: contain; }
.desk-inner { width: max-content; min-width: 100%; padding: 24px; }
.paper-stack { width: 210mm; margin: 0 auto; }
.loading { display: grid; place-items: center; padding: 40px; }
.page-skeleton { width: min(560px, 80%); height: 70vh; border-radius: 6px; }
.quote-inspector-slot { min-height: 0; border-left: 1px solid var(--line-2); background: var(--surface-raised-2); }
/* Narrower screens: the panel floats over the page's right side; the page keeps its zoom. */
.overlay .quote-inspector-slot { position: absolute; top: 8px; right: 8px; bottom: 8px; z-index: 20; width: min(340px, calc(100% - 16px)); border: 0; border-radius: 14px; box-shadow: var(--shadow-pop), var(--shadow); overflow: hidden; }
/* Phones: a sheet from the bottom over the lower part of the page. */
.sheet .quote-inspector-slot { position: absolute; left: 0; right: 0; bottom: 0; z-index: 20; height: min(62%, 540px); border: 0; border-radius: 16px 16px 0 0; box-shadow: var(--shadow-pop), var(--shadow); overflow: hidden; }
.sheet-scrim { position: absolute; inset: 0; z-index: 19; border: 0; background: var(--scrim); }
.sheet .desk-inner { padding: 12px; }
</style>

<style>
/* PDF: only the paper prints, at full size, whatever the zoom and theme. */
@media print {
  .app-shell { display: block !important; height: auto !important; }
  .app-header, .app-footer, .skip-link, .toast-host, .quote-titlebar, .quote-notices, .quote-inspector-slot, .sheet-scrim { display: none !important; }
  main, .quote-desk { overflow: visible !important; background: none !important; }
  .desk-inner { padding: 0 !important; }
  .paper-stack { zoom: 1 !important; }
  .quote-editor-page, .workspace { display: block !important; height: auto !important; }
}
</style>
