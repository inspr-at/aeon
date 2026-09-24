<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import QuoteDocument from '../components/quotes/editor/QuoteDocument.vue'
import { brand, setPageTitle } from '../lib/brand'
import { dayText } from '../lib/quotes/list'
import { documentTotal, money } from '../lib/quotes/layout'
import type { QuoteDocumentData } from '../lib/quotes/types'
import { useVersion } from '../stores/version'

// The page a customer opens from a quote link: the sender's name, the frozen
// document exactly as issued, and, while it can be accepted, a short form to
// accept it. Calm like paper, readable at 390 px, in light or dark. It asks
// the server for this one quote only (no session, no cookies) and loads
// nothing from anywhere else, so it runs under the strict public policy.
const props = defineProps<{ publicTenant: string; token: string }>()
interface PublicQuote {
  document: QuoteDocumentData; offer_no: string; version: number; content_sha256: string
  state: string; expires_at: string; acceptable: boolean; receipt_ready: boolean; accepted_at?: string
}
const quote = ref<PublicQuote | null>(null)
const busy = ref(false)
const loading = ref(true)
const missing = ref(false)
const error = ref('')
const overflow = ref('')
const acceptedNow = ref<{ name: string; at: string } | null>(null)
const name = ref('')
const company = ref('')
const note = ref('')
const confirm = ref(false)
const tried = ref(false)
const nameInput = ref<HTMLInputElement>()
const decision = ref<HTMLElement>()
let mutationID = crypto.randomUUID()
const version = useVersion()
void version.load()
const apiPath = computed(() => `/api/public/quotes/${encodeURIComponent(props.publicTenant)}/${encodeURIComponent(props.token)}`)
const pdfPath = computed(() => `${apiPath.value}/pdf`)
const sender = computed(() => quote.value?.document.sender.company?.trim() || '')
const recipient = computed(() => quote.value?.document.recipient.name?.trim() || '')
const total = computed(() => { const d = quote.value?.document; if (!d) return ''; try { return money(documentTotal(d.positions), d.currency) } catch { return '' } })
const linkEnded = computed(() => !!quote.value && Date.parse(quote.value.expires_at) <= Date.now())
const accepted = computed(() => !!quote.value && (quote.value.state === 'accepted' || !!quote.value.accepted_at))
const when = (iso: string | undefined) => iso ? new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(iso)) : ''
const shortDigest = (sha: string) => `${sha.slice(0, 8)}…${sha.slice(-6)}`
const closedReason = computed(() => {
  const q = quote.value
  if (!q || q.acceptable || accepted.value) return ''
  if (linkEnded.value) return `This link ended on ${when(q.expires_at)}.`
  if (q.state === 'issued' && q.document.valid_until) return `The offer was valid until ${dayText(q.document.valid_until)}.`
  return 'A newer version may replace it.'
})

// Receipts follow an acceptance within a minute or so; the page checks a few times.
let poll: number | undefined, polls = 0
async function load(quiet = false) {
  if (!quiet) { loading.value = true; error.value = ''; missing.value = false }
  try {
    const response = await fetch(apiPath.value, { credentials: 'omit', cache: 'no-store', referrerPolicy: 'no-referrer' })
    if (response.status === 404 || response.status === 410) { missing.value = true; quote.value = null; return }
    if (!response.ok) throw new Error('This quote could not be loaded. Please try again in a moment.')
    quote.value = await response.json() as PublicQuote
    setPageTitle(`Quote ${quote.value.offer_no}${sender.value ? ` from ${sender.value}` : ''}`)
    window.clearTimeout(poll)
    if (accepted.value && !quote.value.receipt_ready && polls < 24) { polls++; poll = window.setTimeout(() => { void load(true) }, 5000) }
  } catch (cause) {
    if (!quiet) error.value = cause instanceof Error ? cause.message : 'This quote could not be loaded.'
  } finally { if (!quiet) loading.value = false }
}
watch(() => [props.publicTenant, props.token], () => { mutationID = crypto.randomUUID(); acceptedNow.value = null; polls = 0; void load() }, { immediate: true })

async function accept() {
  tried.value = true
  if (!quote.value || !quote.value.acceptable || busy.value) return
  if (!name.value.trim()) { nameInput.value?.focus(); return }
  if (!confirm.value) return
  busy.value = true
  error.value = ''
  try {
    const response = await fetch(`${apiPath.value}/accept`, {
      method: 'POST', credentials: 'omit', cache: 'no-store', referrerPolicy: 'no-referrer', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ version: quote.value.version, expected_content_sha256: quote.value.content_sha256,
        client_mutation_id: mutationID, name: name.value.trim(), company: company.value.trim(), note: note.value.trim(), confirm: true }),
    })
    if (response.status === 429) throw new Error('Too many attempts in a short time. Please wait a minute and try again.')
    if (!response.ok) throw new Error(response.status === 409 ? 'This quote can no longer be accepted: it changed, ended or was already decided. Reload the page to see where it stands.' : 'Your acceptance could not be saved. Nothing was recorded; please try again.')
    const body = await response.json().catch(() => ({})) as { accepted_at?: string }
    acceptedNow.value = { name: name.value.trim(), at: body.accepted_at ?? new Date().toISOString() }
    await load(true)
    await nextTick()
    decision.value?.focus()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Your acceptance could not be saved.'
  } finally { busy.value = false }
}
function toDecision() { decision.value?.scrollIntoView({ behavior: 'smooth', block: 'start' }); void nextTick(() => nameInput.value?.focus({ preventScroll: true })) }

// The paper keeps its A4 proportions and shrinks to the screen, never scrolls sideways.
const desk = ref<HTMLElement>()
const scale = ref(1)
let sizer: ResizeObserver | undefined
let frame = 0
watch(desk, el => {
  sizer?.disconnect()
  if (!el) return
  // Measured after layout settles: a changed scale may change the width it was measured at.
  sizer = new ResizeObserver(([entry]) => { cancelAnimationFrame(frame); const width = entry.contentRect.width; frame = requestAnimationFrame(() => { scale.value = Math.min(1, Math.max(0.3, Math.floor(width) / 794)) }) })
  sizer.observe(el)
})
// Private by construction: no referrer leaves this page and search engines are asked to stay away.
const metas: HTMLMetaElement[] = []
onMounted(() => {
  for (const [key, content] of [['robots', 'noindex, nofollow, noarchive'], ['referrer', 'no-referrer']] as const) {
    const meta = document.createElement('meta'); meta.name = key; meta.content = content; document.head.append(meta); metas.push(meta)
  }
})
onBeforeUnmount(() => { sizer?.disconnect(); cancelAnimationFrame(frame); window.clearTimeout(poll); for (const meta of metas) meta.remove() })
</script>

<template>
  <main class="public-quote">
    <header class="pq-bar">
      <div class="pq-bar-inner">
        <p class="pq-sender">{{ sender || 'Quote' }}</p>
        <p v-if="quote" class="pq-ref"><span>Quote {{ quote.offer_no }}</span><span>Version {{ quote.version }}</span></p>
      </div>
    </header>

    <div v-if="loading" class="pq-wrap" role="status" aria-label="Loading the quote"><div class="pq-card pq-skeleton"><span class="skeleton" /><span class="skeleton short" /></div></div>
    <div v-else-if="missing" class="pq-wrap">
      <section class="pq-card pq-message" role="alert">
        <h1>This link does not open a quote</h1>
        <p>It may have ended or been revoked, or it was copied only in part. The sender can share a new link with you.</p>
      </section>
    </div>
    <div v-else-if="!quote" class="pq-wrap">
      <section class="pq-card pq-message" role="alert">
        <h1>The quote could not be loaded</h1>
        <p>{{ error }}</p>
        <button type="button" class="pq-btn" @click="load()">Try again</button>
      </section>
    </div>
    <template v-else>
      <div class="pq-wrap">
        <section class="pq-card pq-intro" aria-labelledby="pq-title">
          <p class="pq-eyebrow">{{ recipient ? `For ${recipient}` : 'Quote' }}</p>
          <h1 id="pq-title">{{ quote.document.title }}</h1>
          <p v-if="quote.document.subtitle" class="pq-sub">{{ quote.document.subtitle }}</p>
          <dl class="pq-facts">
            <div><dt>Net total</dt><dd class="pq-mono">{{ total }}</dd></div>
            <div><dt>Dated</dt><dd>{{ dayText(quote.document.offer_date) }}</dd></div>
            <div><dt>Valid until</dt><dd>{{ dayText(quote.document.valid_until) }}</dd></div>
          </dl>
          <p v-if="accepted" class="pq-status ok" role="status"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="8" cy="8" r="6.2" /><path d="m5.3 8.2 1.9 1.9 3.6-3.9" /></svg><span>Accepted{{ quote.accepted_at ? ` on ${when(quote.accepted_at)}` : '' }}.</span></p>
          <p v-else-if="quote.acceptable" class="pq-status">Please read the quote below. If it suits you, you can accept it at the end of this page; no account is needed.</p>
          <p v-else class="pq-status muted">This quote remains available to read. Acceptance is closed. {{ closedReason }}</p>
          <div class="pq-actions">
            <button v-if="quote.acceptable" type="button" class="pq-btn primary" @click="toDecision">Review and accept</button>
            <a class="pq-btn" :href="pdfPath" target="_blank" rel="noopener noreferrer"><svg width="15" height="15" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M8 2.6v7.6M4.8 7.2 8 10.4l3.2-3.2M3 13.2h10" /></svg>{{ accepted && quote.receipt_ready ? 'Receipt (PDF)' : 'PDF' }}<span class="sr-only"> (opens in a new tab)</span></a>
          </div>
        </section>
      </div>

      <p v-if="overflow" role="alert" class="pq-wrap pq-note">Part of the document did not fit on its page: {{ overflow }}</p>
      <section ref="desk" class="pq-desk" aria-label="Quote document">
        <div class="pq-paper" :style="{ zoom: scale }">
          <QuoteDocument :document="quote.document" :offer-no="quote.offer_no" :editable="false" @overflow="overflow = $event ?? ''" />
        </div>
      </section>

      <div class="pq-wrap">
        <section v-if="acceptedNow" ref="decision" class="pq-card pq-done" tabindex="-1" aria-labelledby="pq-done-title">
          <h2 id="pq-done-title">Thank you{{ acceptedNow ? `, ${acceptedNow.name}` : '' }}</h2>
          <p role="status">Your acceptance of version {{ quote.version }} was recorded{{ acceptedNow ? ` on ${when(acceptedNow.at)}` : '' }}. {{ quote.receipt_ready ? 'Your receipt is ready.' : 'Your receipt (a PDF of this quote with the acceptance) is being prepared and will appear here.' }}</p>
          <a v-if="quote.receipt_ready" class="pq-btn" :href="pdfPath" target="_blank" rel="noopener noreferrer">Open the receipt<span class="sr-only"> (opens in a new tab)</span></a>
        </section>
        <section v-else-if="quote.acceptable" ref="decision" class="pq-card pq-decision" tabindex="-1" aria-labelledby="decision-title">
          <h2 id="decision-title">Accept this quote</h2>
          <p class="pq-lead">You accept version {{ quote.version }} of quote {{ quote.offer_no }}{{ total ? `, net ${total}` : '' }}, exactly as shown above.</p>
          <form novalidate @submit.prevent="accept">
            <div class="pq-field">
              <label for="public-name">Your name</label>
              <input id="public-name" ref="nameInput" v-model="name" name="name" autocomplete="name" required maxlength="500" :aria-invalid="tried && !name.trim()" aria-describedby="public-name-note" />
              <p v-if="tried && !name.trim()" id="public-name-note" class="pq-bad" role="alert">Please enter your name.</p>
            </div>
            <div class="pq-field">
              <label for="public-company">Company <span class="pq-opt">optional</span></label>
              <input id="public-company" v-model="company" name="organization" autocomplete="organization" maxlength="500" />
            </div>
            <div class="pq-field">
              <label for="public-note">Note to {{ sender || 'the sender' }} <span class="pq-opt">optional</span></label>
              <textarea id="public-note" v-model="note" name="note" maxlength="4000" rows="3" />
            </div>
            <label class="pq-check" for="public-confirm"><input id="public-confirm" v-model="confirm" type="checkbox" required />I have reviewed this quote and agree to accept it.</label>
            <button class="pq-btn primary pq-submit" type="submit" :disabled="busy || !confirm">{{ busy ? 'Recording…' : 'Accept quote' }}</button>
          </form>
          <p class="pq-fine">Your name, company, note and the time are recorded with the quote’s fingerprint <span class="pq-mono">{{ shortDigest(quote.content_sha256) }}</span>, so everyone can later see what exactly was accepted.</p>
        </section>
        <p v-if="error && quote" role="alert" class="pq-card pq-error">{{ error }}</p>
      </div>
    </template>
    <footer class="pq-foot"><p>This page shows only this quote{{ sender ? `, shared with you by ${sender}` : '' }}. Made with {{ brand.wordmark }}.</p></footer>
  </main>
</template>

<style scoped>
.public-quote { min-height: 100vh; min-height: 100dvh; background: var(--surface); color: var(--ink); font-family: var(--font); }
.pq-bar { border-bottom: 1px solid var(--line-2); background: var(--surface-raised-2); }
.pq-bar-inner { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 4px 16px; max-width: 880px; margin: 0 auto; padding: 16px 20px; }
.pq-sender { font: 600 17px/1.3 var(--serif); color: var(--ink); overflow-wrap: anywhere; }
.pq-ref { display: flex; flex-wrap: wrap; gap: 4px 12px; font: 500 12.5px/1.4 var(--mono); color: var(--ink-2); font-variant-ligatures: none; }
.pq-wrap { max-width: 880px; margin: 0 auto; padding: 0 20px; }
.pq-card { margin: 24px 0; padding: 24px; border-radius: 16px; background: var(--surface-raised); box-shadow: inset 0 0 0 1px var(--line-2), 0 1px 2px rgba(20, 40, 40, .04); }
.pq-card h1 { font: 650 clamp(22px, 4.2vw, 30px)/1.2 var(--serif); letter-spacing: -.01em; text-wrap: balance; overflow-wrap: anywhere; }
.pq-card h2 { font-size: 19px; font-weight: 650; }
.pq-card p { font-size: 14.5px; line-height: 1.55; color: var(--ink-2); }
.pq-eyebrow { font: 500 11px/1.4 var(--mono) !important; letter-spacing: .12em; text-transform: uppercase; color: var(--ink-3) !important; font-variant-ligatures: none; }
.pq-intro { display: grid; gap: 10px; }
.pq-sub { font-size: 16px !important; color: var(--ink-2); }
.pq-facts { display: flex; flex-wrap: wrap; gap: 12px 32px; margin: 6px 0 0; }
.pq-facts div { display: grid; gap: 2px; }
.pq-facts dt { font-size: 12px; color: var(--ink-3); }
.pq-facts dd { margin: 0; font-size: 15px; font-weight: 600; color: var(--ink); }
.pq-mono { font-family: var(--mono); font-variant-numeric: tabular-nums; font-variant-ligatures: none; }
.pq-status { display: flex; align-items: flex-start; gap: 8px; margin-top: 4px; }
.pq-status.ok { color: var(--ink) !important; font-weight: 600; }
.pq-status.ok svg { flex-shrink: 0; margin-top: 3px; color: var(--ok); }
.pq-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 6px; }
.pq-btn { display: inline-flex; align-items: center; justify-content: center; gap: 7px; min-height: 42px; padding: 0 18px; border: 1px solid var(--line-2); border-radius: 10px; background: var(--surface-raised); color: var(--ink); font: 600 14px/1 var(--font); text-decoration: none; white-space: nowrap; cursor: pointer; }
@media (hover: hover) { .pq-btn:hover { background: var(--row-hover); } }
.pq-btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
.pq-btn.primary { border-color: var(--teal); background: var(--teal); color: var(--button-ink); }
@media (hover: hover) { .pq-btn.primary:hover { filter: brightness(1.06); background: var(--teal); } }
.pq-btn:disabled { opacity: .5; cursor: default; }
.pq-note { margin-top: -8px; font-size: 13px; color: var(--danger); }
/* The desk: the frozen document on a quiet surface, as light paper in any theme. */
.pq-desk { padding: 8px 20px 12px; }
.pq-paper { width: 210mm; margin: 0 auto; }
.pq-decision form { display: grid; gap: 14px; margin-top: 18px; }
.pq-lead { margin-top: 6px; }
.pq-field { display: grid; gap: 6px; }
.pq-field label { font-size: 13.5px; font-weight: 600; color: var(--ink); }
.pq-opt { font-weight: 500; color: var(--ink-3); }
.pq-field input, .pq-field textarea { width: 100%; min-height: 44px; border: 1px solid var(--line-2); border-radius: 10px; background: var(--surface); color: var(--ink); padding: 10px 12px; font: 15px/1.4 var(--font); }
.pq-field textarea { resize: vertical; }
.pq-field input:focus-visible, .pq-field textarea:focus-visible { outline: none; box-shadow: var(--focus-ring); border-color: var(--teal); }
.pq-field input[aria-invalid="true"] { border-color: var(--danger); }
.pq-bad { font-size: 13px !important; color: var(--danger) !important; }
.pq-check { display: flex; align-items: flex-start; gap: 10px; font-size: 14.5px; line-height: 1.45; color: var(--ink); cursor: pointer; }
.pq-check input { flex-shrink: 0; width: 18px; height: 18px; margin-top: 1px; accent-color: var(--teal); }
.pq-submit { justify-self: start; min-width: 180px; }
.pq-fine { margin-top: 16px; padding-top: 14px; border-top: 1px solid var(--line); font-size: 12.5px !important; color: var(--ink-3) !important; }
.pq-done { display: grid; gap: 10px; outline: none; }
.pq-done:focus-visible, .pq-decision:focus-visible { box-shadow: inset 0 0 0 1px var(--line-2), var(--focus-ring); }
.pq-decision { outline: none; }
.pq-error { color: var(--danger) !important; background: var(--danger-bg); box-shadow: inset 0 0 0 1px var(--danger-line); font-size: 14px; }
.pq-message { display: grid; gap: 10px; justify-items: start; margin-top: 48px; }
.pq-skeleton { display: grid; gap: 12px; }
.pq-skeleton .skeleton { height: 14px; }
.pq-skeleton .short { width: 50%; }
.pq-foot { max-width: 880px; margin: 0 auto; padding: 8px 20px 40px; }
.pq-foot p { font-size: 12.5px; color: var(--ink-3); text-align: center; }
@media (max-width: 600px) {
  .pq-wrap { padding: 0 12px; }
  .pq-card { margin: 16px 0; padding: 18px 16px; border-radius: 14px; }
  .pq-bar-inner { padding: 12px 16px; }
  .pq-desk { padding: 4px 12px 8px; }
  .pq-actions .pq-btn, .pq-submit { flex: 1 1 100%; width: 100%; }
}
@media print {
  .pq-bar, .pq-wrap, .pq-foot, .pq-note { display: none !important; }
  .pq-desk { padding: 0; }
  .pq-paper { zoom: 1 !important; }
}
</style>
