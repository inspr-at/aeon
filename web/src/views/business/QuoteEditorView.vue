<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { api } from '../../lib/api'
import { QuoteSession, type SessionView } from '../../lib/quoteSession'
import type { ConflictChoices } from '../../lib/quoteMerge'
import { QuotePresence, selectionAnchor, type PresenceSnapshot } from '../../lib/quotePresence'
import type { RecoveryDraft } from '../../lib/quoteRecovery'
import QuoteDocument from '../../components/quotes/editor/QuoteDocument.vue'
import QuotePresenceOverlay from '../../components/quotes/collaboration/QuotePresenceOverlay.vue'
import { useSession } from '../../stores/session'

const props = defineProps<{ quoteId: string }>()
const identity = useSession()
const host = ref<HTMLElement | null>(null)
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
onBeforeUnmount(() => { generation++; dispose() })
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
</script>

<template>
  <section class="quote-editor-page" aria-label="Quote editor">
    <header class="quote-editor-bar">
      <RouterLink to="/business/quotes">Quotes</RouterLink>
      <span>{{ offerNo || 'Draft quote' }}</span>
      <span v-if="view" role="status">{{ view.local === 'clean' ? 'Saved' : view.local === 'read-only' ? 'Read only' : view.local }}</span>
      <button type="button" :disabled="!canSave" @click="quoteSession?.save()">Save draft</button>
    </header>
    <div v-if="error" role="alert">{{ error }}</div>
    <div v-if="recovery" class="quote-editor-recovery" role="alert">
      Unsaved work from this tab is available.
      <button type="button" @click="useRecovery">Restore my work</button>
      <button type="button" @click="useServer">Use saved draft</button>
    </div>
    <div v-if="view?.remote === 'newer'" class="quote-editor-recovery" role="status">
      A newer draft is available. <button type="button" @click="quoteSession?.reload()">Review changes</button>
    </div>
    <div v-if="view?.review" class="quote-editor-recovery" role="alert">
      {{ view.review.conflicts.length ? `${view.review.conflicts.length} concurrent change conflicts need a choice.` : 'These changes can be merged.' }}
      <button type="button" @click="resolveReview('mine')">{{ view.review.conflicts.length ? 'Keep my changes' : 'Merge changes' }}</button>
      <button v-if="view.review.conflicts.length" type="button" @click="resolveReview('theirs')">Use newer changes</button>
    </div>
    <div v-if="!document && !error" role="status">Loading quote…</div>
    <div v-if="document" ref="host" class="quote-editor-document">
      <QuoteDocument ref="editor" :document="document" :offer-no="offerNo" :editable="view?.local !== 'read-only'" @update:document="edit" />
    </div>
    <QuotePresenceOverlay v-if="document && identity.identity" :root="host" :document="document" :revision="view?.baseRevision ?? 0" :principal-id="identity.identity.principal.id" :presence="presence" />
  </section>
</template>

<style scoped>
.quote-editor-page { padding: 24px; }
.quote-editor-bar { display: flex; align-items: center; gap: 16px; margin-bottom: 20px; }
.quote-editor-bar span:nth-child(2) { font-weight: 700; }
.quote-editor-bar span:nth-child(3) { margin-left: auto; color: var(--ink-2); }
.quote-editor-bar button,.quote-editor-recovery button { border: 1px solid var(--line-2); border-radius: 8px; background: var(--surface-raised); color: var(--ink); padding: 8px 12px; cursor: pointer; }
.quote-editor-bar button:disabled { opacity: .5; cursor: default; }
.quote-editor-recovery { max-width: 800px; margin: 0 auto 16px; padding: 12px; border-radius: 8px; background: var(--surface-raised); box-shadow: inset 0 0 0 1px var(--line-2); }
.quote-editor-recovery button { margin-left: 8px; }
.quote-editor-document { overflow-x: auto; }
</style>
