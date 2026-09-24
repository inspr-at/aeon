<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import QuoteDocument from '../components/quotes/editor/QuoteDocument.vue'
import type { QuoteDocumentData } from '../lib/quotes/types'

// The coordinator passes route params from /offers/:publicTenant/:token and
// mounts this as a bare route that does not refresh a signed-in session.
const props = defineProps<{ publicTenant: string; token: string }>()
interface PublicQuote {
  document: QuoteDocumentData; offer_no: string; version: number; content_sha256: string
  state: string; expires_at: string; acceptable: boolean; receipt_ready: boolean; accepted_at?: string
}
const quote = ref<PublicQuote | null>(null)
const busy = ref(false)
const loading = ref(true)
const error = ref('')
const success = ref('')
const overflow = ref('')
const name = ref('')
const company = ref('')
const note = ref('')
const confirm = ref(false)
let mutationID = crypto.randomUUID()
const apiPath = computed(() => `/api/public/quotes/${encodeURIComponent(props.publicTenant)}/${encodeURIComponent(props.token)}`)
const pdfPath = computed(() => `${apiPath.value}/pdf`)
async function load() {
  loading.value = true
  error.value = ''
  quote.value = null
  try {
    const response = await fetch(apiPath.value, { credentials: 'omit', cache: 'no-store' })
    if (!response.ok) throw new Error('This quote link is unavailable.')
    quote.value = await response.json() as PublicQuote
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'This quote link is unavailable.'
  } finally { loading.value = false }
}
watch(() => [props.publicTenant, props.token], () => { mutationID = crypto.randomUUID(); void load() }, { immediate: true })
async function accept() {
  if (!quote.value || !quote.value.acceptable || !confirm.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    const response = await fetch(`${apiPath.value}/accept`, {
      method: 'POST', credentials: 'omit', cache: 'no-store', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ version: quote.value.version, expected_content_sha256: quote.value.content_sha256,
        client_mutation_id: mutationID, name: name.value.trim(), company: company.value.trim(), note: note.value.trim(), confirm: true }),
    })
    if (!response.ok) throw new Error(response.status === 409 ? 'This quote can no longer be accepted. Reload to see its current state.' : 'Acceptance could not be saved. Please try again.')
    success.value = 'Your acceptance was recorded. The PDF receipt is being prepared.'
    await load()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Acceptance could not be saved.'
  } finally { busy.value = false }
}
</script>
<template>
  <main class="public-quote">
    <header class="public-quote-header"><span class="public-quote-brand">Aeon</span><span>Customer quote</span></header>
    <div v-if="loading" role="status" class="public-quote-message">Loading quote…</div>
    <div v-else-if="!quote" role="alert" class="public-quote-message">{{ error || 'This quote link is unavailable.' }}</div>
    <template v-else>
      <div class="public-quote-intro">
        <div><p class="public-quote-label">Quote {{ quote.offer_no }}</p><h1>{{ quote.document.title }}</h1>
          <p v-if="quote.acceptable">Review the frozen document below, then confirm your decision.</p>
          <p v-else-if="quote.state === 'accepted'">This quote has been accepted.</p>
          <p v-else>This quote remains available to read. Acceptance is closed.</p>
        </div>
        <a class="public-quote-download" :href="pdfPath" target="_blank" rel="noopener noreferrer">Open PDF<span class="sr-only"> in a new tab</span></a>
      </div>
      <p v-if="overflow" role="alert" class="public-quote-message">Document layout needs attention: {{ overflow }}</p>
      <section class="public-quote-paper" aria-label="Quote document">
        <QuoteDocument :document="quote.document" :offer-no="quote.offer_no" :editable="false" @overflow="overflow = $event ?? ''" />
      </section>
      <section v-if="quote.acceptable" class="public-quote-decision" aria-labelledby="decision-title">
        <h2 id="decision-title">Accept this quote</h2>
        <p>Your decision applies to version {{ quote.version }} of the document shown above.</p>
        <form @submit.prevent="accept">
          <label for="public-name">Your name <span aria-hidden="true">*</span></label>
          <input id="public-name" v-model="name" name="name" autocomplete="name" required maxlength="500" />
          <label for="public-company">Company</label>
          <input id="public-company" v-model="company" name="organization" autocomplete="organization" maxlength="500" />
          <label for="public-note">Note</label>
          <textarea id="public-note" v-model="note" name="note" maxlength="4000" rows="3" />
          <label class="public-quote-check" for="public-confirm"><input id="public-confirm" v-model="confirm" type="checkbox" required />I have reviewed this quote and agree to accept it.</label>
          <button class="public-quote-submit" type="submit" :disabled="busy || !confirm">{{ busy ? 'Recording…' : 'Accept quote' }}</button>
        </form>
      </section>
      <p v-if="success" role="status" class="public-quote-message">{{ success }}</p>
      <p v-if="error" role="alert" class="public-quote-message">{{ error }}</p>
    </template>
  </main>
</template>
<style scoped>
.public-quote { min-height: 100vh; background: var(--surface); color: var(--ink); }
.public-quote-header { display: flex; justify-content: space-between; align-items: center; padding: 16px clamp(20px, 4vw, 48px); border-bottom: 1px solid var(--line-2); font-size: 13px; color: var(--ink-2); }
.public-quote-brand { font-family: var(--serif); font-size: 23px; color: var(--ink); }
.public-quote-intro { max-width: 1120px; margin: auto; padding: 36px 20px 24px; display: flex; align-items: start; justify-content: space-between; gap: 24px; }
.public-quote-intro h1 { margin: 4px 0 8px; }.public-quote-intro p { color: var(--ink-2); margin: 0; }
.public-quote-label { font-size: 12px; text-transform: uppercase; letter-spacing: .08em; }
.public-quote-download,.public-quote-submit { display: inline-flex; justify-content: center; align-items: center; min-height: 40px; border: 1px solid var(--line-2); border-radius: 9px; padding: 0 16px; background: var(--surface-raised); color: var(--ink); text-decoration: none; font-weight: 600; white-space: nowrap; cursor: pointer; }
.public-quote-submit { background: var(--ink); color: var(--surface); border-color: var(--ink); justify-self: start; }
.public-quote-submit:disabled { opacity: .55; cursor: wait; }
.public-quote-paper { overflow-x: auto; padding: 12px 20px 24px; --surface-raised: #fff; --ink: #203c3d; --ink-2: #496064; --line-2: #bcc9c9; --font: 'JetBrains Mono', monospace; --serif: 'JetBrains Mono', monospace; }
.public-quote-decision { max-width: 680px; margin: 24px auto 64px; padding: 28px; border-radius: 12px; background: var(--surface-raised); box-shadow: inset 0 0 0 1px var(--line-2); }
.public-quote-decision h2 { margin: 0 0 8px; }.public-quote-decision p { color: var(--ink-2); }
.public-quote-decision form { display: grid; gap: 10px; margin-top: 22px; }
.public-quote-decision input:not([type=checkbox]),.public-quote-decision textarea { width: 100%; border: 1px solid var(--line-2); border-radius: 8px; background: var(--surface); padding: 10px 12px; }
.public-quote-check { display: flex; align-items: start; gap: 10px; margin: 10px 0; }.public-quote-check input { margin-top: 4px; }
.public-quote-message { max-width: 680px; margin: 20px auto; padding: 16px; border-radius: 10px; background: var(--surface-raised); box-shadow: inset 0 0 0 1px var(--line-2); }
@media (max-width: 700px) { .public-quote-intro { display: grid; }.public-quote-decision { margin-inline: 16px; padding: 20px; } }
</style>
