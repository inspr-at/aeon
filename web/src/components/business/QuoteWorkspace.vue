<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { APIError, getNode, updateNode, type WorkNode } from '../../lib/api'
import { acceptVersion, exportURL, getQuote, issueVersion, listBindings, listVersions, createVersion, type Binding, type Quote, type QuoteVersion } from '../../lib/business'
import { blocker, changed, draftFrom, evaluate, forgetDraft, recallDraft, rememberDraft, toWrite, totals, type Draft } from '../../lib/quoteDraft'
import { confirmAction } from '../../lib/confirm'
import { toast } from '../../lib/toast'
import { absoluteTime, relativeTime } from '../../lib/work'
import type { SaveResult } from '../../lib/useTicket'
import { diffAmounts, formatAmount, isZero, lineTax, ratePercent, sumAmounts } from './money'
import { useBusiness } from '../../stores/business'
import { useProjects } from '../../stores/projects'
import AppIcon from '../AppIcon.vue'
import InlineTitle from '../work/InlineTitle.vue'
import MarkdownBody from '../MarkdownBody.vue'
import MarkdownEditor from '../work/MarkdownEditor.vue'
import FloatingPanel from '../work/FloatingPanel.vue'
import MoneyText from './MoneyText.vue'
import PickerMenu, { type PickOption } from './PickerMenu.vue'
import QuoteLines from './QuoteLines.vue'
import QuoteStatus from './QuoteStatus.vue'

// One quote in the docked panel or on its own page: who it is for, the lines of
// the next version (or the frozen current one), exact totals, the customer's
// acceptance of an issued version, and every version with its total.
const props = defineProps<{
  quote: Quote | null; quoteKey: string; resolving: boolean; resolveError: string
  position: { index: number; count: number } | null; mode: 'panel' | 'full'; now: number
}>()
const emit = defineEmits<{ close: []; prev: []; next: []; expand: []; collapse: []; changed: [quote: Quote]; retry: [] }>()
const business = useBusiness()
const projects = useProjects()
const router = useRouter()
const root = ref<HTMLElement>()
const scroller = ref<HTMLElement>()
const lines = ref<InstanceType<typeof QuoteLines>>()
const title = ref<InstanceType<typeof InlineTitle>>()
const versions = ref<QuoteVersion[]>([])
const versionsState = ref<'loading' | 'ready' | 'error'>('loading')
const versionsError = ref('')
const node = ref<WorkNode | null>(null)
const draft = ref<Draft | null>(null)
const editing = ref(false)
const restored = ref(false)
const conflict = ref('')
const busy = ref<'' | 'save' | 'issue' | 'accept'>('')
const termsEditing = ref(false)
const termsDraft = ref('')
const bindings = ref<Binding[]>([])
const bindingsState = ref<'idle' | 'loading' | 'ready' | 'forbidden' | 'error'>('idle')
const menu = ref<{ kind: 'recipient' | 'currency' | 'more'; anchor: HTMLElement } | null>(null)

const q = computed(() => props.quote)
const id = computed(() => q.value?.quote_node_id ?? '')
const current = computed(() => versions.value.find(v => v.version === q.value?.current_version) ?? null)
const ordered = computed(() => [...versions.value].sort((a, b) => b.version - a.version))
const canEdit = computed(() => business.staff && !!q.value && q.value.state !== 'void')
const today = () => new Date().toISOString().slice(0, 10)
const results = computed(() => draft.value ? evaluate(draft.value, (unit, kind, currency) => business.rateOn(unit, kind, currency, today())) : [])
const draftTotals = computed(() => totals(results.value))
const dirty = computed(() => !!draft.value && editing.value && changed(draft.value, current.value))
const block = computed(() => draft.value ? blocker(draft.value, results.value) : '')
const nextVersion = computed(() => (q.value?.current_version ?? 0) + 1)
const org = computed(() => q.value ? business.organisation(q.value.customer_org_node_id) : undefined)
const project = computed(() => q.value ? projects.byId(q.value.project_node_id) : undefined)
const recipientId = computed(() => editing.value && draft.value ? draft.value.recipientId : current.value?.recipient_contact_node_id ?? '')
const recipient = computed(() => recipientId.value ? business.contact(recipientId.value) : undefined)
const currency = computed(() => editing.value && draft.value ? draft.value.currency : current.value?.currency ?? draft.value?.currency ?? 'EUR')
const orgContacts = computed(() => q.value ? business.contactsOf(q.value.customer_org_node_id) : [])
const showDraft = computed(() => editing.value && !!draft.value)
const shownTotals = computed(() => showDraft.value
  ? { net: draftTotals.value.net, tax: draftTotals.value.tax, total: draftTotals.value.total, taxes: draftTotals.value.taxes }
  : current.value ? { net: current.value.subtotal, tax: current.value.tax_total, total: current.value.total, taxes: frozenTaxes(current.value) } : null)
const terms = computed(() => showDraft.value ? draft.value!.terms : current.value?.terms_markdown ?? '')
const boundPeople = computed(() => bindings.value.filter(b => b.principal_kind === 'person'))

// Tax per rate of a frozen version, recomputed exactly the way the server summed it.
function frozenTaxes(version: QuoteVersion) {
  const by = new Map<string, string[]>()
  for (const line of version.lines) {
    if (/^0(\.0+)?$/.test(line.tax_rate)) continue
    by.set(line.tax_rate, [...(by.get(line.tax_rate) ?? []), lineTax(line.net_amount, line.tax_rate)])
  }
  return [...by.entries()].sort(([a], [b]) => b.localeCompare(a)).map(([rate, amounts]) => ({ rate, amount: sumAmounts(amounts) }))
}
function defaultCurrency() {
  const counts = new Map<string, number>()
  for (const unit of business.costUnits) for (const rate of unit.rates) counts.set(rate.currency, (counts.get(rate.currency) ?? 0) + 1)
  return [...counts.entries()].sort((a, b) => b[1] - a[1])[0]?.[0] ?? 'EUR'
}

// ---------- Loading ----------
let generation = 0
async function load() {
  const quoteId = id.value
  if (!quoteId) return
  const request = ++generation
  versionsState.value = 'loading'; conflict.value = ''; termsEditing.value = false; menu.value = null
  scroller.value?.scrollTo({ top: 0 })
  void business.loadCRM().then(() => q.value && business.loadLinks([q.value.customer_org_node_id]))
  void business.loadCostUnits()
  void business.loadPrincipals()
  void projects.load()
  try {
    const [list, fresh] = await Promise.all([listVersions(quoteId), getNode(quoteId).catch(() => null)])
    if (request !== generation) return
    versions.value = list
    node.value = fresh
    versionsState.value = 'ready'
    setupDraft()
  } catch (e) {
    if (request !== generation) return
    versionsState.value = 'error'
    versionsError.value = e instanceof Error ? e.message : 'Versions could not be loaded.'
  }
}
function setupDraft() {
  const quote = q.value
  if (!quote) return
  const kept = recallDraft(quote.quote_node_id)
  restored.value = false
  if (kept && canEdit.value) {
    draft.value = kept; editing.value = true; restored.value = true
    if (kept.baseRevision !== quote.revision) conflict.value = `This quote changed since you started these edits: it is at version ${quote.current_version} now. Your lines are kept; saving makes them version ${quote.current_version + 1}.`
    kept.baseRevision = quote.revision; kept.baseVersion = quote.current_version
    return
  }
  if (quote.state === 'draft' && canEdit.value) {
    const only = business.contactsOf(quote.customer_org_node_id)
    draft.value = draftFrom(quote.quote_node_id, quote.revision, current.value, { currency: defaultCurrency(), recipientId: only.length === 1 ? only[0].id : '' })
    editing.value = true
  } else { draft.value = null; editing.value = false }
}
watch(id, () => { versions.value = []; draft.value = null; editing.value = false; bindings.value = []; void load() }, { immediate: true })
// A recipient chosen before the organisation's contacts arrived: fill it once they do.
watch(orgContacts, contacts => { if (draft.value && editing.value && !draft.value.recipientId && contacts.length === 1 && !current.value) draft.value.recipientId = contacts[0].id })
watch(draft, value => { if (!value) return; if (dirty.value) rememberDraft(value); else forgetDraft(value.quoteId) }, { deep: true })
watch(recipientId, async contactId => {
  bindings.value = []
  if (!contactId || !business.staff) { bindingsState.value = 'idle'; return }
  bindingsState.value = 'loading'
  try { const found = await listBindings(contactId); if (recipientId.value === contactId) { bindings.value = found; bindingsState.value = 'ready' } }
  catch (e) { bindingsState.value = e instanceof APIError && e.status === 403 ? 'forbidden' : 'error' }
}, { immediate: true })

async function refresh() {
  const quoteId = id.value
  const [fresh, list] = await Promise.all([getQuote(quoteId), listVersions(quoteId)])
  if (quoteId !== id.value) return null
  emit('changed', fresh)
  versions.value = list
  return fresh
}

// ---------- Title ----------
async function saveTitle(value: string): Promise<SaveResult> {
  if (!q.value) return 'error'
  try {
    const current = node.value ?? await getNode(q.value.quote_node_id)
    node.value = await updateNode(q.value.quote_node_id, { title: value }, { ifUnmodifiedSince: current.updated_at })
    emit('changed', { ...q.value, title: node.value.title, updated_at: node.value.updated_at })
    return 'ok'
  } catch (e) {
    if (e instanceof APIError && e.status === 412) {
      node.value = await getNode(q.value.quote_node_id)
      emit('changed', { ...q.value, title: node.value.title })
      toast(`${q.value.key} was renamed elsewhere. The newer title is shown; your draft is kept.`, { tone: 'error' })
      return 'conflict'
    }
    toast(`The title was not saved: ${e instanceof Error ? e.message : 'unknown error'}`, { tone: 'error' })
    return 'error'
  }
}

// ---------- Actions ----------
async function save() {
  const quote = q.value, value = draft.value
  if (!quote || !value || block.value || busy.value || !dirty.value) return
  busy.value = 'save'
  try {
    const frozen = await createVersion(quote.quote_node_id, toWrite(value, quote.title, results.value))
    forgetDraft(quote.quote_node_id)
    conflict.value = ''; restored.value = false
    const fresh = await refresh()
    if (fresh) draft.value = draftFrom(fresh.quote_node_id, fresh.revision, frozen)
    toast(`Version ${frozen.version} saved: ${formatAmount(frozen.total, frozen.currency)} ${frozen.currency}.`)
  } catch (e) {
    if (e instanceof APIError && e.status === 409) {
      const fresh = await refresh().catch(() => null)
      if (fresh && draft.value) {
        draft.value.baseRevision = fresh.revision; draft.value.baseVersion = fresh.current_version
        conflict.value = fresh.state === 'void'
          ? 'This quote was made void while you were editing. It cannot take a new version.'
          : `Version ${fresh.current_version} was saved elsewhere while you were editing. Your lines are kept; saving again makes them version ${fresh.current_version + 1}.`
      }
    } else toast(`The version was not saved: ${e instanceof Error ? e.message : 'unknown error'}`, { tone: 'error' })
  } finally { busy.value = '' }
}
async function issue() {
  const quote = q.value, version = current.value
  if (!quote || !version || busy.value) return
  const who = recipient.value?.title ?? 'the recipient'
  const ok = await confirmAction({
    title: `Issue version ${version.version} to ${who}?`,
    body: `The offer goes out exactly as frozen: ${version.lines.length} ${version.lines.length === 1 ? 'line' : 'lines'}, total ${formatAmount(version.total, version.currency)} ${version.currency}. ${who} can then accept this version. A later change needs a new version.`,
    confirmLabel: `Issue version ${version.version}`,
  })
  if (!ok) return
  busy.value = 'issue'
  try {
    const fresh = await issueVersion(quote.quote_node_id, version.version)
    emit('changed', fresh)
    versions.value = await listVersions(quote.quote_node_id)
    draft.value = null; editing.value = false
    toast(`Version ${version.version} issued to ${who}.`)
  } catch (e) {
    if (e instanceof APIError && e.status === 409) { await refresh().catch(() => null); setupDraft(); toast(`${quote.key} changed before it was issued. The latest state is shown.`, { tone: 'error' }) }
    else toast(`The quote was not issued: ${e instanceof Error ? e.message : 'unknown error'}`, { tone: 'error' })
  } finally { busy.value = '' }
}
async function accept() {
  const quote = q.value, version = current.value
  if (!quote || !version || busy.value) return
  const ok = await confirmAction({
    title: `Accept version ${version.version} of ${quote.key}?`,
    body: `You accept this offer exactly as issued: total ${formatAmount(version.total, version.currency)} ${version.currency}, offer digest ${version.content_sha256.slice(0, 8)}…${version.content_sha256.slice(-6)}. Your acceptance is recorded with your name and cannot be withdrawn here.`,
    confirmLabel: 'Accept offer',
  })
  if (!ok) return
  busy.value = 'accept'
  try {
    await acceptVersion(quote.quote_node_id, version.version, version.content_sha256)
    await refresh()
    toast(`You accepted ${quote.key}, version ${version.version}.`)
  } catch (e) {
    if (e instanceof APIError && e.status === 403) toast('Only the person bound to the recipient contact can accept this offer.', { tone: 'error' })
    else if (e instanceof APIError && e.status === 409) { await refresh().catch(() => null); toast('The offer changed before your acceptance. The latest version is shown; nothing was accepted.', { tone: 'error' }) }
    else toast(`The offer was not accepted: ${e instanceof Error ? e.message : 'unknown error'}`, { tone: 'error' })
  } finally { busy.value = '' }
}
async function revise() {
  const quote = q.value
  if (!quote || !canEdit.value) return
  if (quote.state === 'accepted') {
    const ok = await confirmAction({
      title: 'Start a new version?',
      body: `Version ${quote.current_version} stays accepted and on record. ${quote.key} returns to draft once you save the new version, until it is issued and accepted again.`,
      confirmLabel: 'Start new version',
    })
    if (!ok) return
  }
  draft.value = draftFrom(quote.quote_node_id, quote.revision, current.value, { currency: defaultCurrency() })
  editing.value = true
  await nextTick(); lines.value?.focusLast()
}
async function discard() {
  const quote = q.value
  if (!quote || !draft.value) return
  if (dirty.value) {
    const ok = await confirmAction({ title: 'Discard these changes?', body: `The lines go back to ${current.value ? `version ${current.value.version}` : 'empty'}.`, confirmLabel: 'Discard changes', danger: true })
    if (!ok) return
  }
  forgetDraft(quote.quote_node_id); conflict.value = ''; restored.value = false
  if (quote.state === 'draft') draft.value = draftFrom(quote.quote_node_id, quote.revision, current.value, { currency: defaultCurrency() })
  else { draft.value = null; editing.value = false }
}

// ---------- Recipient, currency and terms ----------
const recipientOptions = computed<PickOption[]>(() => orgContacts.value.map(contact => ({ value: contact.id, label: contact.title, note: typeof contact.fields.email === 'string' ? contact.fields.email : undefined, icon: 'user' })))
const currencyOptions = computed<PickOption[]>(() => [...new Set([...business.currencies, currency.value])].sort().map(code => ({ value: code, label: code, note: code === currency.value ? 'current' : undefined })))
function openMenu(kind: 'recipient' | 'currency' | 'more', event: MouseEvent) {
  const anchor = event.currentTarget as HTMLElement
  menu.value = menu.value?.kind === kind ? null : { kind, anchor }
}
function closeMenu(restore: boolean) { const anchor = menu.value?.anchor; menu.value = null; if (restore) anchor?.focus() }
function chooseRecipient(option: PickOption) { if (draft.value) draft.value.recipientId = option.value; closeMenu(true) }
function chooseCurrency(option: PickOption) { if (draft.value) draft.value.currency = option.value; closeMenu(true) }
function editTerms() { termsDraft.value = draft.value?.terms ?? ''; termsEditing.value = true }
function saveTerms() { if (draft.value) draft.value.terms = termsDraft.value; termsEditing.value = false }

// ---------- More ----------
function copy(text: string, label: string) { navigator.clipboard.writeText(text).then(() => toast(`Copied ${label}`), () => toast(`${label} could not be copied`, { tone: 'error' })) }
function copyLink() { if (q.value) copy(`${location.origin}/business/quotes/${encodeURIComponent(q.value.key)}`, 'link'); menu.value = null }
function documentPath(version?: number) { return `/business/quotes/${encodeURIComponent(q.value?.key ?? props.quoteKey)}/document${version ? `?v=${version}` : ''}` }
function download(format: 'markdown' | 'pdf') { const v = current.value; if (q.value && v) window.open(exportURL(q.value.quote_node_id, v.version, format), '_blank', 'noopener'); menu.value = null }
function versionState(version: QuoteVersion) {
  if (version.acceptance) return { label: 'Accepted', tone: 'ok' }
  if (version.issue) return { label: version.version === q.value?.current_version ? 'Issued' : 'Issued, replaced', tone: version.version === q.value?.current_version ? 'busy' : '' }
  return { label: version.version === q.value?.current_version ? 'Draft' : 'Replaced', tone: '' }
}
function delta(version: QuoteVersion) {
  const previous = versions.value.find(v => v.version === version.version - 1)
  if (!previous || previous.currency !== version.currency) return null
  const value = diffAmounts(version.total, previous.total)
  return isZero(value) ? null : value
}
const digestShort = (digest: string) => `${digest.slice(0, 8)}…${digest.slice(-6)}`

// ---------- Keyboard (panel-level; the page handles list keys) ----------
function keydown(event: KeyboardEvent) {
  if (event.defaultPrevented) return
  if ((event.metaKey || event.ctrlKey) && event.key === 'Enter' && dirty.value) { event.preventDefault(); void save() }
}
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value) { event.preventDefault(); event.returnValue = '' } }
window.addEventListener('beforeunload', beforeUnload)
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
defineExpose({
  el: root, isDirty: () => dirty.value || !!title.value?.isDirty() || termsEditing.value,
  focus: () => root.value?.focus({ preventScroll: true }), editTitle: () => title.value?.start(), focusLines: () => lines.value?.focusFirst(),
})
</script>

<template>
  <component :is="mode === 'panel' ? 'aside' : 'article'" ref="root" class="quote-ws" :class="mode" aria-label="Quote details" tabindex="-1" @keydown="keydown">
    <header class="panel-bar">
      <button v-if="mode === 'full'" type="button" class="icon-btn sm flat" aria-label="Back to quotes" data-tip="Back to quotes · Esc" @click="emit('close')"><AppIcon name="chevron-left" :size="16" /></button>
      <button type="button" class="key-chip" :aria-label="`Copy ${q?.key ?? quoteKey}`" :data-tip="`Copy ${q?.key ?? quoteKey}`" @click="copy(q?.key ?? quoteKey, q?.key ?? quoteKey)">
        <AppIcon name="document" :size="12" /><span>{{ q?.key ?? quoteKey }}</span><AppIcon name="copy" :size="11" class="copy-glyph" />
      </button>
      <span v-if="position" class="position mono">{{ position.index + 1 }} / {{ position.count }}</span>
      <div v-if="mode === 'panel'" class="nav">
        <button type="button" class="icon-btn sm flat" aria-label="Previous quote" aria-keyshortcuts="k" data-tip="Previous · k" :disabled="!position || position.index === 0" @click="emit('prev')"><AppIcon name="chevron-up" :size="15" /></button>
        <button type="button" class="icon-btn sm flat" aria-label="Next quote" aria-keyshortcuts="j" data-tip="Next · j" :disabled="!position || position.index >= position.count - 1" @click="emit('next')"><AppIcon name="chevron" :size="15" /></button>
      </div>
      <span class="spacer" />
      <RouterLink v-if="q && current" class="icon-btn sm flat" :to="documentPath()" aria-label="Open the offer document" data-tip="Offer document · d"><AppIcon name="print" :size="14" /></RouterLink>
      <button v-if="mode === 'panel'" type="button" class="icon-btn sm flat wide-only" aria-label="Open as full page" data-tip="Full page · f" @click="emit('expand')"><AppIcon name="expand" :size="14" /></button>
      <button v-else type="button" class="icon-btn sm flat wide-only" aria-label="Show beside the list" data-tip="Side panel · f" @click="emit('collapse')"><AppIcon name="collapse" :size="14" /></button>
      <button type="button" class="icon-btn sm flat" aria-label="More actions" aria-haspopup="menu" :aria-expanded="menu?.kind === 'more'" data-tip="More" @click="openMenu('more', $event)"><AppIcon name="more" :size="15" /></button>
      <button v-if="mode === 'panel'" type="button" class="icon-btn sm flat" aria-label="Close quote details" aria-keyshortcuts="Escape" data-tip="Close · Esc" @click="emit('close')"><AppIcon name="close" :size="15" /></button>
    </header>

    <div ref="scroller" class="ws-scroll">
      <div v-if="resolveError && !q" class="ws-state" role="alert">
        <span class="state-icon danger"><AppIcon name="alert" :size="18" /></span>
        <h2>This quote could not be opened</h2>
        <p>{{ resolveError }}</p>
        <button type="button" class="btn" @click="emit('retry')"><AppIcon name="refresh" :size="14" />Try again</button>
      </div>
      <div v-else-if="!q" class="ws-skeleton" role="status" aria-label="Loading quote">
        <span class="skeleton title-skel" /><span class="chips"><span class="skeleton chip" /><span class="skeleton chip" /><span class="skeleton chip" /></span>
        <span class="skeleton block-skel" />
      </div>

      <div v-else class="ws-grid">
        <div class="ws-main">
          <InlineTitle ref="title" :value="q.title" :editable="canEdit" :large="mode === 'full'" :save="saveTitle" />
          <dl class="facts">
            <div class="fact"><dt>Status</dt><dd><span class="prop"><QuoteStatus :state="q.state" /><span v-if="q.current_version" class="ver mono">v{{ q.current_version }}</span></span></dd></div>
            <div class="fact"><dt>Customer</dt><dd><RouterLink class="prop link" :to="org ? `/business/organisations/${encodeURIComponent(org.key)}` : '/business/organisations'"><AppIcon name="building" :size="13" class="faint-icon" /><span class="prop-text">{{ org?.title ?? 'Customer' }}</span></RouterLink></dd></div>
            <div class="fact"><dt>Recipient</dt><dd>
              <button v-if="showDraft" type="button" class="prop" :class="{ unset: !recipient }" aria-haspopup="dialog" :aria-expanded="menu?.kind === 'recipient'" @click="openMenu('recipient', $event)"><AppIcon name="user" :size="13" class="faint-icon" /><span class="prop-text">{{ recipient?.title ?? 'Choose recipient' }}</span><AppIcon name="chevron" :size="12" class="chev" /></button>
              <span v-else class="prop"><AppIcon name="user" :size="13" class="faint-icon" /><span class="prop-text">{{ recipient?.title ?? '—' }}</span></span>
            </dd></div>
            <div class="fact"><dt>Project</dt><dd><RouterLink v-if="project" class="prop link" :to="`/p/${encodeURIComponent(project.routeKey)}`"><span class="key-badge">{{ project.routeKey }}</span><span class="prop-text">{{ project.title }}</span></RouterLink><span v-else class="prop faint">—</span></dd></div>
            <div class="fact"><dt>Currency</dt><dd>
              <button v-if="showDraft" type="button" class="prop mono" aria-haspopup="dialog" :aria-expanded="menu?.kind === 'currency'" @click="openMenu('currency', $event)">{{ currency }}<AppIcon name="chevron" :size="12" class="chev" /></button>
              <span v-else class="prop mono">{{ currency }}</span>
            </dd></div>
          </dl>
          <p class="meta">Updated <time :datetime="q.updated_at" :data-tip="absoluteTime(q.updated_at)">{{ relativeTime(q.updated_at, { now, long: true }) }}</time> · Created <time :datetime="q.created_at" :data-tip="absoluteTime(q.created_at)">{{ relativeTime(q.created_at, { now, long: true }) }}</time></p>
          <div class="divider" />

          <!-- Where the quote stands, and what comes next. -->
          <div v-if="conflict" class="banner warn" role="alert"><AppIcon name="alert" :size="15" /><p>{{ conflict }}</p></div>
          <div v-else-if="restored && dirty" class="banner" role="note"><AppIcon name="history" :size="15" /><p>Your unsaved lines from earlier are back. Save them as version {{ nextVersion }}, or discard them.</p></div>
          <div v-else-if="q.state === 'issued' && current" class="banner teal" role="note">
            <AppIcon name="send" :size="15" />
            <p>Issued {{ current.issue ? relativeTime(current.issue.issued_at, { now, long: true }) : '' }}{{ current.issue ? ` by ${business.nameOf(current.issue.issued_by_principal_id)}` : '' }}. Waiting for {{ recipient?.title ?? 'the recipient' }} to accept version {{ current.version }}.</p>
            <button v-if="canEdit && !editing" type="button" class="btn sm" @click="revise">Revise</button>
          </div>
          <div v-else-if="q.state === 'accepted' && current?.acceptance" class="banner ok" role="note">
            <AppIcon name="seal" :size="15" />
            <p>Accepted by {{ business.nameOf(current.acceptance.customer_principal_id) }} {{ relativeTime(current.acceptance.accepted_at, { now, long: true }) }}.</p>
            <button v-if="canEdit && !editing" type="button" class="btn sm" @click="revise">New version</button>
          </div>
          <div v-else-if="q.state === 'void'" class="banner" role="note"><AppIcon name="archive" :size="15" /><p>This quote is void. Its versions stay readable.</p></div>

          <section class="block" aria-labelledby="lines-title">
            <div class="block-head">
              <h3 id="lines-title" class="eyebrow">{{ showDraft ? (dirty ? `Version ${nextVersion} · unsaved` : current ? `Version ${current.version}` : 'Lines') : current ? `Version ${current.version} · frozen` : 'Lines' }}</h3>
              <span v-if="showDraft && dirty" class="unsaved-dot" aria-hidden="true" />
              <span class="spacer" />
              <button v-if="showDraft && (dirty || q.state !== 'draft')" type="button" class="text-btn" @click="discard">{{ q.state === 'draft' ? 'Discard changes' : 'Cancel revision' }}</button>
            </div>
            <div v-if="versionsState === 'loading'" class="lines-skeleton" aria-hidden="true"><span v-for="i in 3" :key="i" class="skeleton" /></div>
            <p v-else-if="versionsState === 'error'" class="inline-error" role="alert"><AppIcon name="alert" :size="14" />{{ versionsError }} <button type="button" class="btn sm" @click="load">Try again</button></p>
            <template v-else>
              <QuoteLines ref="lines" :draft="showDraft ? draft : null" :results="results" :frozen="showDraft ? null : current" :editable="showDraft && canEdit" @save="save" />
              <p v-if="!showDraft && !current" class="empty-line">No version yet.</p>
              <dl v-if="shownTotals && (showDraft || current)" class="totals" aria-label="Totals">
                <div class="total-row"><dt>Net</dt><dd><MoneyText :amount="shownTotals.net" :currency="currency" /></dd></div>
                <template v-if="shownTotals.taxes.length > 1">
                  <div v-for="tax in shownTotals.taxes" :key="tax.rate" class="total-row sub"><dt>Tax {{ ratePercent(tax.rate) }} %</dt><dd><MoneyText :amount="tax.amount" :currency="currency" /></dd></div>
                </template>
                <div class="total-row"><dt>{{ shownTotals.taxes.length > 1 ? 'Tax total' : 'Tax' }}<span v-if="shownTotals.taxes.length === 1" class="rate"> {{ ratePercent(shownTotals.taxes[0].rate) }} %</span></dt><dd><MoneyText :amount="shownTotals.tax" :currency="currency" /></dd></div>
                <div class="total-row grand"><dt>Total</dt><dd><MoneyText :amount="shownTotals.total" :currency="currency" strong /></dd></div>
              </dl>
            </template>
          </section>

          <section v-if="showDraft || terms.trim()" class="block" aria-labelledby="terms-title">
            <div class="block-head">
              <h3 id="terms-title" class="eyebrow">Terms</h3>
              <span class="spacer" />
              <button v-if="showDraft && canEdit && !termsEditing && terms.trim()" type="button" class="icon-btn sm flat" :aria-label="terms.trim() ? 'Edit terms' : 'Add terms'" :data-tip="terms.trim() ? 'Edit terms' : 'Add terms'" @click="editTerms"><AppIcon :name="terms.trim() ? 'edit' : 'plus'" :size="13" /></button>
            </div>
            <MarkdownEditor v-if="termsEditing" v-model="termsDraft" label="Terms" save-label="Use these terms" placeholder="Payment terms, validity, what is not included…" :min-rows="4" compact @save="saveTerms" @cancel="termsEditing = false" />
            <MarkdownBody v-else-if="terms.trim()" :body="terms" class="terms" />
            <button v-else type="button" class="add-section" @click="editTerms"><AppIcon name="plus" :size="12" />Add terms</button>
          </section>

          <section v-if="current || recipient" class="block" aria-labelledby="accept-title">
            <h3 id="accept-title" class="eyebrow">Customer acceptance</h3>
            <div class="accept-card" :class="{ ok: !!current?.acceptance }">
              <span class="accept-icon"><AppIcon :name="current?.acceptance ? 'seal' : 'shield'" :size="16" /></span>
              <div class="accept-text">
                <template v-if="current?.acceptance && q.state === 'accepted'">
                  <strong>Accepted by {{ business.nameOf(current.acceptance.customer_principal_id) }}</strong>
                  <span><time :datetime="current.acceptance.accepted_at">{{ absoluteTime(current.acceptance.accepted_at) }}</time> · version {{ current.version }} · the accepted digest matches this version.</span>
                </template>
                <template v-else-if="q.state === 'issued' && current">
                  <strong>{{ q.viewer_can_accept ? 'You can accept this offer' : `Waiting for ${recipient?.title ?? 'the recipient'}` }}</strong>
                  <span v-if="q.viewer_can_accept">Accepting records your approval of version {{ current.version }} exactly as issued.</span>
                  <span v-else-if="boundPeople.length">{{ boundPeople.map(b => b.principal_name).join(', ') }} can accept once signed in.</span>
                  <span v-else-if="bindingsState === 'ready'" class="warn-text">{{ recipient?.title ?? 'The recipient' }} is not bound to a person yet, so nobody can accept. <RouterLink v-if="org" :to="`/business/organisations/${encodeURIComponent(org.key)}`">Bind a person</RouterLink></span>
                </template>
                <template v-else>
                  <strong>Not issued yet</strong>
                  <span v-if="boundPeople.length">Once issued, {{ boundPeople.map(b => b.principal_name).join(', ') }} can accept the exact version.</span>
                  <span v-else-if="bindingsState === 'ready' && recipient">Once issued, {{ recipient.title }} accepts it after being bound to a person in Organisations.</span>
                  <span v-else>Once issued, the recipient accepts the exact version.</span>
                </template>
                <span v-if="current && q.state !== 'draft'" class="digest">Offer digest <code :data-tip="current.content_sha256">{{ digestShort(current.content_sha256) }}</code><button type="button" class="icon-btn sm flat" aria-label="Copy the offer digest" data-tip="Copy digest" @click="copy(current.content_sha256, 'digest')"><AppIcon name="copy" :size="12" /></button></span>
              </div>
            </div>
          </section>

          <section v-if="versions.length" class="block" aria-labelledby="versions-title">
            <h3 id="versions-title" class="eyebrow">Versions</h3>
            <ol class="versions" aria-label="Versions">
              <li v-for="version in ordered" :key="version.version" class="version" :class="{ current: version.version === q.current_version }">
                <RouterLink class="version-row" :to="documentPath(version.version)" :aria-label="`Version ${version.version}, ${versionState(version).label}, total ${formatAmount(version.total, version.currency)} ${version.currency}. Open the document`">
                  <span class="v-badge mono">v{{ version.version }}</span>
                  <span class="v-text">
                    <span class="v-state" :class="versionState(version).tone">{{ versionState(version).label }}</span>
                    <span class="v-meta">{{ version.lines.length }} {{ version.lines.length === 1 ? 'line' : 'lines' }} · {{ business.nameOf(version.created_by_principal_id) }} · <time :datetime="version.created_at" :data-tip="absoluteTime(version.created_at)">{{ relativeTime(version.created_at, { now }) }}</time></span>
                  </span>
                  <span class="v-total">
                    <MoneyText :amount="version.total" :currency="version.currency" />
                    <MoneyText v-if="delta(version)" class="v-delta" :amount="delta(version)!" :currency="version.currency" signed />
                  </span>
                  <AppIcon name="chevron-right" :size="13" class="go" />
                </RouterLink>
              </li>
            </ol>
          </section>
        </div>

      </div>
    </div>

    <!-- One primary action at a time. -->
    <footer v-if="q && versionsState === 'ready'" class="ws-actions">
      <template v-if="showDraft && dirty">
        <p class="action-note" :class="{ warn: !!block }">{{ block || `Saving freezes these lines as version ${nextVersion}.` }}</p>
        <button type="button" class="btn primary" :disabled="!!block || !!busy" aria-keyshortcuts="Meta+Enter Control+Enter" @click="save"><AppIcon name="lock" :size="13" />{{ busy === 'save' ? 'Saving…' : `Save version ${nextVersion}` }}</button>
      </template>
      <template v-else-if="q.state === 'draft' && current && business.admin">
        <p class="action-note">Version {{ current.version }} is frozen. Issuing sends it to {{ recipient?.title ?? 'the recipient' }} for acceptance.</p>
        <button type="button" class="btn primary" :disabled="!!busy" @click="issue"><AppIcon name="send" :size="13" />{{ busy === 'issue' ? 'Issuing…' : `Issue version ${current.version}` }}</button>
      </template>
      <template v-else-if="q.state === 'draft' && current">
        <p class="action-note">Version {{ current.version }} is frozen. A workspace admin issues it.</p>
      </template>
      <template v-else-if="q.state === 'issued' && q.viewer_can_accept && current">
        <p class="action-note">Total {{ formatAmount(current.total, current.currency) }} {{ current.currency }}, version {{ current.version }}.</p>
        <button type="button" class="btn primary" :disabled="!!busy" @click="accept"><AppIcon name="seal" :size="13" />{{ busy === 'accept' ? 'Accepting…' : 'Accept offer' }}</button>
      </template>
      <template v-else-if="showDraft && !current">
        <p class="action-note">Add lines; saving freezes them as version 1.</p>
      </template>
      <template v-else-if="current">
        <p class="action-note">{{ q.state === 'accepted' ? 'Accepted and on record.' : q.state === 'void' ? 'Void.' : 'Frozen and issued.' }}</p>
        <button type="button" class="btn" @click="router.push(documentPath())"><AppIcon name="print" :size="13" />Offer document</button>
      </template>
    </footer>

    <PickerMenu v-if="menu?.kind === 'recipient'" :anchor="menu.anchor" title="Recipient" :options="recipientOptions" :current="recipientId" placeholder="Find a contact…" :empty="org ? `${org.title} has no contacts yet. Add one in Organisations.` : 'No contacts yet.'" @choose="chooseRecipient" @close="closeMenu" />
    <PickerMenu v-if="menu?.kind === 'currency'" :anchor="menu.anchor" title="Currency" :options="currencyOptions" :current="currency" :width="220" placeholder="Find a currency…" empty="No rates in any currency yet." @choose="chooseCurrency" @close="closeMenu" />
    <FloatingPanel v-if="menu?.kind === 'more' && q" :anchor="menu.anchor" :width="240" align="end" :label="`Actions for ${q.key}`" @close="closeMenu">
      <div class="more-menu" role="menu" :aria-label="`Actions for ${q.key}`">
        <button type="button" role="menuitem" class="menu-item" data-autofocus @click="copyLink"><AppIcon name="link" :size="14" />Copy link</button>
        <button type="button" role="menuitem" class="menu-item" @click="copy(q.key, q.key); menu = null"><AppIcon name="copy" :size="14" />Copy key</button>
        <div class="menu-sep" role="separator" />
        <button type="button" role="menuitem" class="menu-item" :disabled="!current" @click="router.push(documentPath()); menu = null"><AppIcon name="print" :size="14" />Offer document</button>
        <button type="button" role="menuitem" class="menu-item" :disabled="!current" @click="download('pdf')"><AppIcon name="download" :size="14" />Download PDF</button>
        <button type="button" role="menuitem" class="menu-item" :disabled="!current" @click="download('markdown')"><AppIcon name="document" :size="14" />Download Markdown</button>
      </div>
    </FloatingPanel>
  </component>
</template>

<style scoped>
.quote-ws { display: flex; flex-direction: column; min-height: 0; outline: none; }
.quote-ws.panel {
  position: fixed; z-index: 15; top: calc(var(--header-h) + 10px); right: 10px; bottom: 10px; width: min(560px, calc(100vw - 20px));
  border-radius: var(--radius); border: 1px solid var(--glass-edge);
  background: linear-gradient(165deg, var(--surface-raised), var(--surface-raised-2)); box-shadow: var(--shadow-pop), var(--shadow);
  backdrop-filter: blur(20px) saturate(1.15); -webkit-backdrop-filter: blur(20px) saturate(1.15);
}
.quote-ws.panel:focus-visible { box-shadow: var(--shadow-pop), var(--focus-ring); }
@media (min-width: 1100px) { .quote-ws.panel { width: var(--panel-w); } }
@media (prefers-reduced-motion: no-preference) {
  .quote-ws.panel { animation: panel-in .22s cubic-bezier(.2, .7, .2, 1); }
  @keyframes panel-in { from { opacity: 0; transform: translateX(24px); } to { opacity: 1; transform: none; } }
}
.panel-bar { display: flex; align-items: center; gap: 6px; height: 52px; padding: 0 10px 0 14px; border-bottom: 1px solid var(--line); flex-shrink: 0; }
.full .panel-bar { padding-left: 8px; border-bottom: 0; }
.key-chip { display: inline-flex; flex-shrink: 0; align-items: center; gap: 6px; height: 26px; padding: 0 9px 0 10px; border: 0; border-radius: 7px; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); font: 600 12px/1 var(--mono); letter-spacing: .03em; font-variant-ligatures: none; }
.key-chip:hover { box-shadow: inset 0 0 0 1px var(--teal); }
.key-chip:focus-visible { box-shadow: var(--focus-ring); }
.copy-glyph { opacity: .45; }
.position { margin-left: 6px; font-size: 11.5px; color: var(--ink-3); }
.nav { display: inline-flex; gap: 2px; margin-left: 2px; }
.nav .icon-btn:disabled { opacity: .35; }
.spacer { flex: 1; }
.ws-scroll { flex: 1; min-height: 0; overflow: auto; overscroll-behavior: contain; }
.panel .ws-scroll { padding: 18px 22px 24px; }
.facts { display: flex; flex-wrap: wrap; gap: 6px; margin: 14px 0 0; }
.fact dt { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
.fact dd { margin: 0; }
.prop { display: inline-flex; align-items: center; gap: 7px; max-width: 260px; height: 28px; padding: 0 11px 0 9px; border: 0; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); color: var(--ink); font-size: 12.5px; white-space: nowrap; text-decoration: none; }
button.prop, a.prop { cursor: pointer; }
@media (hover: hover) { button.prop:hover, a.prop:hover { background: var(--row-hover); box-shadow: inset 0 0 0 1px var(--glass-rim); } }
.prop:focus-visible { box-shadow: var(--focus-ring); }
.prop.unset { color: var(--gold-ink); box-shadow: inset 0 0 0 1px rgba(214, 155, 49, .45); }
.prop.mono { font-family: var(--mono); font-size: 12px; font-variant-ligatures: none; }
.prop-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
.prop .key-badge { height: 18px; padding: 0 6px; font-size: 10px; }
.ver { font-size: 11px; color: var(--ink-3); }
.chev, .faint-icon { color: var(--ink-3); flex-shrink: 0; }
.faint { color: var(--ink-3); }
.meta { margin-top: 12px; font-size: 12.5px; color: var(--ink-3); }
.meta time { color: var(--ink-2); }
.divider { height: 1px; margin: 16px 0 18px; background: linear-gradient(90deg, var(--line-2), transparent); }
.banner { display: flex; align-items: center; gap: 10px; margin-bottom: 18px; padding: 10px 12px; border-radius: 12px; background: var(--code-bg); color: var(--ink-2); }
.banner p { flex: 1; font-size: 13px; color: var(--ink); }
.banner.warn { background: var(--gold-wash); box-shadow: inset 0 0 0 1px rgba(214, 155, 49, .35); color: var(--gold-ink); }
.banner.teal { background: var(--aqua-wash); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.banner.ok { background: rgba(47, 122, 90, .08); box-shadow: inset 0 0 0 1px rgba(47, 122, 90, .28); color: var(--ok); }
.block { margin-top: 24px; }
.block:first-of-type { margin-top: 0; }
.block-head { display: flex; align-items: center; gap: 8px; min-height: 28px; margin-bottom: 8px; }
.block > .eyebrow { margin-bottom: 10px; }
.unsaved-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--gold); box-shadow: 0 0 0 3px rgba(214, 155, 49, .18); }
.text-btn { height: 26px; padding: 0 10px; border: 0; border-radius: 999px; background: transparent; color: var(--ink-2); font-size: 12.5px; font-weight: 600; }
.text-btn:hover { background: var(--row-hover); color: var(--ink); }
.text-btn:focus-visible { box-shadow: var(--focus-ring); }
.lines-skeleton { display: grid; gap: 14px; padding: 12px 4px; }
.lines-skeleton .skeleton { height: 12px; }
.empty-line { margin-top: 8px; font-size: 13px; color: var(--ink-3); }
.inline-error { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; padding: 8px 12px; border-radius: 10px; background: var(--danger-bg); color: var(--danger); font-size: 13px; }
.totals { display: grid; justify-content: end; gap: 2px; margin: 12px 0 0; }
.total-row { display: grid; grid-template-columns: 110px minmax(140px, auto); align-items: baseline; gap: 16px; padding: 3px 8px; }
.total-row dt { font-size: 12.5px; color: var(--ink-2); text-align: right; }
.total-row dd { margin: 0; text-align: right; font-size: 13.5px; }
.total-row.sub dt { font-size: 12px; color: var(--ink-3); }
.total-row .rate { color: var(--ink-3); }
.total-row.grand { margin-top: 4px; padding-top: 8px; border-top: 1px solid var(--line-2); }
.total-row.grand dt { font-weight: 650; color: var(--ink); }
.total-row.grand dd { font-size: 15px; }
.terms { font-size: 13.5px; }
.add-section { display: inline-flex; align-items: center; gap: 5px; height: 26px; padding: 0 10px; border: 0; border-radius: 999px; background: transparent; box-shadow: inset 0 0 0 1px var(--line); color: var(--ink-3); font-size: 12px; }
.add-section:hover { color: var(--teal-ink); box-shadow: inset 0 0 0 1px var(--glass-rim); background: var(--row-hover); }
.accept-card { display: flex; gap: 12px; padding: 12px 14px; border-radius: 12px; background: var(--code-bg); }
.accept-card.ok { background: rgba(47, 122, 90, .08); box-shadow: inset 0 0 0 1px rgba(47, 122, 90, .25); }
.accept-icon { display: grid; place-items: center; flex-shrink: 0; width: 32px; height: 32px; border-radius: 10px; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.accept-card.ok .accept-icon { background: rgba(47, 122, 90, .12); box-shadow: inset 0 0 0 1px rgba(47, 122, 90, .3); color: var(--ok); }
.accept-text { display: grid; gap: 3px; min-width: 0; font-size: 12.5px; color: var(--ink-2); }
.accept-text strong { font-size: 13.5px; color: var(--ink); }
.warn-text { color: var(--gold-ink); }
.digest { display: inline-flex; align-items: center; gap: 6px; margin-top: 4px; font-size: 12px; color: var(--ink-3); }
.digest code, .side-digest code { padding: 2px 6px; border-radius: 6px; background: var(--code-bg); font-size: 11.5px; color: var(--ink-2); }
.versions { display: grid; gap: 2px; margin: 0; padding: 0; list-style: none; }
.version-row { display: grid; grid-template-columns: 40px minmax(0, 1fr) auto 14px; align-items: center; gap: 12px; min-height: 48px; padding: 6px 10px; border-radius: 10px; color: var(--ink); text-decoration: none; }
@media (hover: hover) { .version-row:hover { background: var(--row-hover); } }
.version-row:focus-visible { background: var(--row-selected); box-shadow: inset 0 0 0 1px var(--glass-rim); }
.v-badge { display: inline-grid; place-items: center; height: 24px; border-radius: 7px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); font-size: 11.5px; color: var(--ink-2); }
.current .v-badge { background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); font-weight: 700; }
.v-text { display: grid; min-width: 0; }
.v-state { font-size: 13px; font-weight: 600; }
.v-state.ok { color: var(--ok); }
.v-state.busy { color: var(--teal-ink); }
.v-meta { font-size: 12px; color: var(--ink-3); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.v-total { display: grid; justify-items: end; gap: 2px; font-size: 13px; }
.v-delta { font-size: 11.5px; }
.go { color: var(--ink-3); opacity: 0; }
.version-row:hover .go, .version-row:focus-visible .go { opacity: 1; color: var(--teal); }
.ws-actions { flex-shrink: 0; display: flex; align-items: center; gap: 12px; padding: 10px 14px 12px 20px; border-top: 1px solid var(--line); background: var(--surface-raised-2); }
.panel .ws-actions { border-radius: 0 0 var(--radius) var(--radius); }
.action-note { flex: 1; min-width: 0; font-size: 12.5px; color: var(--ink-2); }
.action-note.warn { color: var(--gold-ink); }
.ws-state { display: grid; justify-items: center; gap: 8px; padding: 56px 16px; text-align: center; }
.ws-state h2 { font-size: 17px; }
.ws-state p { font-size: 13.5px; }
.ws-state .btn { margin-top: 8px; }
.state-icon { display: grid; place-items: center; width: 44px; height: 44px; margin-bottom: 4px; border-radius: 50%; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.state-icon.danger { background: var(--danger-bg); box-shadow: inset 0 0 0 1px var(--danger-line); color: var(--danger); }
.ws-skeleton { display: grid; gap: 14px; }
.ws-skeleton .title-skel { width: 70%; height: 18px; border-radius: 8px; }
.ws-skeleton .chips { display: flex; gap: 8px; }
.ws-skeleton .chip { width: 90px; height: 26px; border-radius: 999px; }
.ws-skeleton .block-skel { height: 160px; border-radius: 12px; margin-top: 16px; }
.more-menu { display: grid; gap: 1px; }
.menu-item { display: flex; align-items: center; gap: 10px; width: 100%; height: 34px; padding: 0 10px; border: 0; border-radius: 8px; background: transparent; color: var(--ink); font-size: 13.5px; text-align: left; }
.menu-item svg { color: var(--ink-2); }
@media (hover: hover) { .menu-item:hover:not(:disabled) { background: var(--row-hover); } }
.menu-item:focus-visible { background: var(--row-selected); box-shadow: inset 0 0 0 1px var(--glass-rim); }
.menu-sep { height: 1px; margin: 4px 6px; background: var(--line); }
/* Full page: the offer left at a comfortable measure, the totals card right. */
.quote-ws.full { width: 100%; max-width: 1120px; min-height: 100%; margin: 0 auto; }
.full .ws-scroll { overflow: visible; }
.full .ws-grid { padding: 20px 0 40px; }
.full .ws-main { min-width: 0; }
.full .ws-actions { position: sticky; bottom: 0; margin: 0 -28px; padding: 12px 28px; border-radius: 0; background: var(--glass); backdrop-filter: blur(18px) saturate(1.2); -webkit-backdrop-filter: blur(18px) saturate(1.2); }
@media (max-width: 720px) {
  .quote-ws.panel { z-index: 40; inset: 0; width: auto; height: 100dvh; border-radius: 0; border: 0; background: var(--canvas); }
  .panel-bar { height: 56px; padding: 0 6px 0 12px; }
  .panel-bar .icon-btn { width: 44px; height: 44px; }
  .position, .wide-only { display: none; }
  .panel .ws-scroll { padding: 16px 16px 24px; }
  .facts { flex-wrap: nowrap; overflow-x: auto; margin: 14px -16px 0; padding: 2px 16px 4px; scrollbar-width: none; }
  .facts::-webkit-scrollbar { display: none; }
  .fact { flex-shrink: 0; }
  .prop { height: 34px; }
  .ws-actions { flex-wrap: wrap; padding: 10px 12px calc(10px + env(safe-area-inset-bottom)); border-radius: 0 !important; background: var(--surface-raised); }
  .ws-actions .btn { height: 44px; flex: 1 0 auto; }
  .action-note { flex-basis: 100%; }
  .total-row { grid-template-columns: 90px minmax(120px, auto); }
  .full .ws-actions { margin: 0 -12px; }
  @media (prefers-reduced-motion: no-preference) { .quote-ws.panel { animation-name: sheet-in; } @keyframes sheet-in { from { transform: translateY(24px); opacity: 0; } to { transform: none; opacity: 1; } } }
}
</style>
