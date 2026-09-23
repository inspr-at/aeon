<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../../lib/api'
import { useSession } from '../../stores/session'

interface Quote { quote_node_id: string; project_node_id: string; customer_org_node_id: string; current_version: number; state: string; revision: number }
interface Line { position: number; description: string; cost_unit_node_id: string; unit: string; quantity: string; rate_amount: string; net_amount: string; tax_rate: string }
interface Version { quote_node_id: string; version: number; title: string; recipient_contact_node_id: string; currency: string; terms_markdown: string; lines: Line[]; subtotal: string; tax_total: string; total: string; content_sha256: string }

const session = useSession()
const isPerson = computed(() => session.identity?.principal.kind === 'person')
const isStaff = computed(() => isPerson.value && session.identity?.principal.roles?.some(role => role === 'admin' || role === 'member'))
const isAdmin = computed(() => isPerson.value && session.identity?.principal.roles?.includes('admin'))
const quotes = ref<Quote[]>([])
const versions = ref<Version[]>([])
const selected = ref<Quote | null>(null)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const createForm = ref({ title: '', project_node_id: '', customer_org_node_id: '' })
const offer = ref({ recipient_contact_node_id: '', currency: 'EUR', title: '', terms_markdown: '' })
const blankLine = () => ({ description: '', cost_unit_node_id: '', unit: 'hour', quantity: '1', tax_rate: '0' })
const offerLines = ref([blankLine()])
const decimal = /^(?:0|[1-9]\d{0,13})(?:\.\d{1,4})?$/
const taxDecimal = /^(?:0(?:\.\d{1,5})?|1(?:\.0{1,5})?)$/
const id = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await api(path, init)
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new Error(typeof body.error === 'string' ? body.error : `Request failed (${response.status})`)
  }
  // Keep the API's exact numeric lexemes for financial fields. JSON.parse
  // otherwise turns a numeric(18,4) value into a binary floating point number.
  const wire = await response.text()
  const exact = wire.replace(/"(quantity|rate_amount|net_amount|tax_rate|subtotal|tax_total|total)":\s*(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g, '"$1":"$2"')
  return JSON.parse(exact) as T
}
async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try { await action() } catch (cause) { error.value = cause instanceof Error ? cause.message : 'Quote request failed' }
  finally { busy.value = false }
}
async function load() { quotes.value = await request<Quote[]>('/quotes'); if (selected.value) selected.value = quotes.value.find(q => q.quote_node_id === selected.value?.quote_node_id) ?? null }
async function selectQuote(q: Quote) { await run(async () => { selected.value = q; versions.value = await request<Version[]>(`/quotes/${q.quote_node_id}/versions`) }) }
async function createQuote() { await run(async () => {
  if (!createForm.value.title.trim() || !id.test(createForm.value.project_node_id) || !id.test(createForm.value.customer_org_node_id)) throw new Error('Enter a title and valid project and customer IDs.')
  const q = await request<Quote>('/quotes', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(createForm.value) })
  await load(); selected.value = q; versions.value = []; createForm.value = { title: '', project_node_id: '', customer_org_node_id: '' }; notice.value = 'Quote created.'
}) }
async function freeze() { await run(async () => {
  if (!selected.value) return
  const f = offer.value
  if (!id.test(f.recipient_contact_node_id) || !f.title.trim() || !/^[A-Z]{3}$/.test(f.currency) || !offerLines.value.length || offerLines.value.some(line => !id.test(line.cost_unit_node_id) || !line.description.trim() || !decimal.test(line.quantity) || /^0(?:\.0{1,4})?$/.test(line.quantity) || !taxDecimal.test(line.tax_rate))) throw new Error('Complete the offer with valid IDs, currency and exact decimal amounts.')
  // Construct JSON numbers from validated decimal strings. No binary float is
  // used in offer arithmetic or transmitted money values.
  const lines = offerLines.value.map(line => `{"description":${JSON.stringify(line.description)},"cost_unit_node_id":${JSON.stringify(line.cost_unit_node_id)},"unit":${JSON.stringify(line.unit)},"quantity":${line.quantity},"tax_rate":${line.tax_rate}}`).join(',')
  const body = `{"expected_revision":${selected.value.revision},"recipient_contact_node_id":${JSON.stringify(f.recipient_contact_node_id)},"currency":${JSON.stringify(f.currency)},"title":${JSON.stringify(f.title)},"terms_markdown":${JSON.stringify(f.terms_markdown)},"lines":[${lines}]}`
  await request<Version>(`/quotes/${selected.value.quote_node_id}/versions`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body })
  await load(); versions.value = await request<Version[]>(`/quotes/${selected.value.quote_node_id}/versions`); notice.value = 'Frozen version created.'
}) }
async function issue(v: Version) { await run(async () => { if (!selected.value) return; await request<Quote>(`/quotes/${selected.value.quote_node_id}/versions/${v.version}/issue`, { method: 'POST' }); await load(); notice.value = 'Quote issued.' }) }
async function accept(v: Version) { await run(async () => { if (!selected.value) return; await request(`/quotes/${selected.value.quote_node_id}/versions/${v.version}/accept`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ expected_content_sha256: v.content_sha256 }) }); await load(); notice.value = 'Acceptance recorded.' }) }
function exportURL(v: Version, format: 'markdown' | 'pdf') { return `/api/quotes/${v.quote_node_id}/versions/${v.version}/export?format=${format}` }
onMounted(() => { void run(load) })
</script>

<template>
  <main class="quotes-page">
    <header class="quotes-heading"><div><p class="eyebrow">Business</p><h1>Quotes</h1><p>Frozen offers, exact prices and customer acceptance.</p></div><button class="button secondary" :disabled="busy" @click="run(load)">Refresh</button></header>
    <p v-if="error" class="error" role="alert">{{ error }}</p><p v-if="notice" role="status">{{ notice }}</p>
    <div class="quotes-layout">
      <section class="glass-card" aria-label="Quotes"><h2>Offers</h2><p v-if="!quotes.length">No quotes yet.</p>
        <button v-for="q in quotes" :key="q.quote_node_id" class="quote-row" :class="{ active: selected?.quote_node_id === q.quote_node_id }" @click="selectQuote(q)"><span>{{ q.quote_node_id }}</span><span>{{ q.state }} · v{{ q.current_version }}</span></button>
        <form v-if="isStaff" class="quote-form" @submit.prevent="createQuote"><h3>New quote</h3><label>Title<input v-model="createForm.title" required /></label><label>Project ID<input v-model="createForm.project_node_id" required /></label><label>Customer organisation ID<input v-model="createForm.customer_org_node_id" required /></label><button class="button" :disabled="busy">Create quote</button></form>
      </section>
      <section class="glass-card" aria-label="Quote details"><template v-if="selected"><h2>Quote {{ selected.quote_node_id }}</h2><p>Project {{ selected.project_node_id }} · Customer {{ selected.customer_org_node_id }}</p><p>Status: <strong>{{ selected.state }}</strong> · Revision {{ selected.revision }}</p>
        <article v-for="v in versions" :key="v.version" class="version"><div class="version-heading"><h3>{{ v.title }} · v{{ v.version }}</h3><span>{{ v.total }} {{ v.currency }}</span></div><p>Recipient contact {{ v.recipient_contact_node_id }}</p><ul><li v-for="line in v.lines" :key="line.position">{{ line.description }} — {{ line.quantity }} {{ line.unit }} × {{ line.rate_amount }} = {{ line.net_amount }} {{ v.currency }}</li></ul><p>Subtotal {{ v.subtotal }} · Tax {{ v.tax_total }} · Total {{ v.total }} {{ v.currency }}</p><p class="digest">SHA-256 {{ v.content_sha256 }}</p><div class="quote-actions"><a :href="exportURL(v, 'markdown')" target="_blank" rel="noopener">Markdown</a><a :href="exportURL(v, 'pdf')" target="_blank" rel="noopener">PDF</a><button v-if="isAdmin && selected.state === 'draft' && selected.current_version === v.version" class="button" :disabled="busy" @click="issue(v)">Issue</button><button v-if="isPerson && selected.state === 'issued' && selected.current_version === v.version" class="button" :disabled="busy" @click="accept(v)">Accept this exact offer</button></div></article>
        <form v-if="isStaff" class="quote-form" @submit.prevent="freeze"><h3>New frozen version</h3><label>Title<input v-model="offer.title" required /></label><label>Recipient contact ID<input v-model="offer.recipient_contact_node_id" required /></label><label>Currency<input v-model="offer.currency" maxlength="3" required /></label><fieldset v-for="(line, index) in offerLines" :key="index" class="quote-line"><legend>Line {{ index + 1 }}</legend><label>Description<input v-model="line.description" required /></label><label>Cost unit ID<input v-model="line.cost_unit_node_id" required /></label><label>Unit<select v-model="line.unit"><option>hour</option><option>day</option><option>item</option></select></label><label>Quantity<input v-model="line.quantity" inputmode="decimal" required /></label><label>Tax rate (0–1)<input v-model="line.tax_rate" inputmode="decimal" required /></label><button v-if="offerLines.length > 1" type="button" class="button secondary" @click="offerLines.splice(index, 1)">Remove line</button></fieldset><button type="button" class="button secondary" :disabled="offerLines.length >= 100" @click="offerLines.push(blankLine())">Add line</button><label>Markdown terms<textarea v-model="offer.terms_markdown" rows="5" /></label><button class="button" :disabled="busy">Freeze version</button></form>
      </template><p v-else>Select a quote to see its versions.</p></section>
    </div>
  </main>
</template>

<style scoped>
.quotes-page{padding:2rem;max-width:1400px;margin:auto}.quotes-heading,.version-heading{display:flex;align-items:center;justify-content:space-between;gap:1rem}.eyebrow{text-transform:uppercase;letter-spacing:.12em;opacity:.65}.quotes-layout{display:grid;grid-template-columns:minmax(260px,1fr) minmax(400px,2fr);gap:1rem}.quotes-layout>section{padding:1.25rem;min-width:0}.quote-row{display:flex;justify-content:space-between;gap:1rem;width:100%;padding:.75rem;background:transparent;border:1px solid currentColor;border-radius:.5rem;color:inherit;text-align:left;margin:.4rem 0;cursor:pointer}.quote-row span:first-child{overflow:hidden;text-overflow:ellipsis}.quote-row.active{outline:2px solid currentColor}.quote-form,.quote-line{display:grid;gap:.8rem;margin-top:2rem}.quote-line{margin-top:0;padding:1rem;border:1px solid currentColor;border-radius:.5rem}.quote-form label{display:grid;gap:.25rem}.quote-form input,.quote-form select,.quote-form textarea{width:100%;box-sizing:border-box}.version{border-top:1px solid currentColor;padding:1rem 0}.version ul{padding-left:1.2rem}.digest{font-family:monospace;overflow-wrap:anywhere;font-size:.8rem}.quote-actions{display:flex;align-items:center;gap:1rem;flex-wrap:wrap}@media(max-width:800px){.quotes-layout{grid-template-columns:1fr}.quotes-page{padding:1rem}}
</style>
