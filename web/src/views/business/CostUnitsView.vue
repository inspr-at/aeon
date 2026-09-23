<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { APIError, api, getKinds, getNodes, type WorkNode } from '../../lib/api'
import { useSession } from '../../stores/session'

interface Rate {
  id: string
  cost_unit_node_id: string
  unit: string
  currency: string
  internal_amount: string
  bill_amount: string
  effective_from: string
  effective_until: string | null
  created_by_principal_id: string
  created_at: string
}

const session = useSession()
const units = ref<WorkNode[]>([])
const selected = ref('')
const rates = ref<Rate[]>([])
const truncated = ref(false)
const busy = ref(false)
const ratesBusy = ref(false)
const pending = ref(false)
const listError = ref('')
const rateError = ref('')
const actionError = ref('')
const notice = ref('')
const form = reactive({ unit: 'hour', currency: 'EUR', internal: '', bill: '', from: utcToday(), until: '' })

const admin = computed(() => session.identity?.principal.kind === 'person' && !!session.identity.principal.roles?.includes('admin'))
const current = computed(() => units.value.find(unit => unit.id === selected.value) ?? null)
const groups = computed(() => {
  const order: string[] = []
  const grouped = new Map<string, Rate[]>()
  for (const rate of rates.value) {
    const rows = grouped.get(rate.currency)
    if (!rows) { grouped.set(rate.currency, [rate]); order.push(rate.currency) }
    else rows.push(rate)
  }
  return order.map(currency => ({ currency, rates: grouped.get(currency) ?? [] }))
})

onMounted(() => { void loadUnits() })
watch(selected, () => { notice.value = ''; actionError.value = ''; void loadRates() })

function utcToday(): string { return new Date().toISOString().slice(0, 10) }
function classicRecord(node: WorkNode): Record<string, unknown> | null {
  const classic = node.fields?.classic
  if (!classic || typeof classic !== 'object' || Array.isArray(classic)) return null
  return classic as Record<string, unknown>
}
function classicID(node: WorkNode): string {
  const id = classicRecord(node)?.id
  if (typeof id === 'string' && id.trim()) return id.trim()
  if (typeof id === 'number' && Number.isInteger(id)) return String(id)
  return ''
}
function classicSource(node: WorkNode): string {
  const source = classicRecord(node)?.source_id
  return typeof source === 'string' ? source : ''
}
function messageOf(cause: unknown, fallback: string): string {
  return cause instanceof Error && cause.message ? cause.message : fallback
}
async function readBody(path: string, method = 'GET', raw?: string): Promise<string> {
  const response = await api(path, {
    method,
    ...(raw === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: raw }),
  })
  const text = await response.text()
  if (!response.ok) {
    let message = `Request failed (${response.status})`
    try {
      const data = JSON.parse(text) as { message?: unknown; error?: unknown }
      if (typeof data.message === 'string' && data.message) message = data.message
      else if (typeof data.error === 'string' && data.error) message = data.error
    } catch { /* The status line is the fallback. */ }
    throw new APIError(response.status, message)
  }
  return text
}
function parseRates(raw: string): Rate[] {
  const preserved = raw.replace(/"(internal_amount|bill_amount)"\s*:\s*(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g, '"$1":"$2"')
  const data: unknown = JSON.parse(preserved)
  if (!Array.isArray(data)) throw new Error('Invalid rate list')
  return data as Rate[]
}
async function loadUnits() {
  busy.value = true
  listError.value = ''
  try {
    const kinds = await getKinds()
    const kind = kinds.items.find(item => item.slug === 'cost_unit')
    if (!kind) { units.value = []; selected.value = ''; truncated.value = false; return }
    const all: WorkNode[] = []
    let cursor = ''
    for (let page = 0; page < 40; page += 1) {
      const response = await getNodes({ kind_id: kind.id, sort: 'key', direction: 'asc', limit: 50, ...(cursor ? { cursor } : {}) })
      all.push(...response.items)
      cursor = response.next_cursor ?? ''
      if (!cursor) break
    }
    truncated.value = cursor !== ''
    units.value = all
    const previous = selected.value
    if (!all.some(unit => unit.id === selected.value)) selected.value = all[0]?.id ?? ''
    if (selected.value === previous) void loadRates()
  } catch (cause) {
    listError.value = messageOf(cause, 'Cost units are unavailable.')
  } finally { busy.value = false }
}
async function loadRates() {
  rateError.value = ''
  if (!selected.value) { rates.value = []; return }
  ratesBusy.value = true
  try {
    rates.value = parseRates(await readBody(`/cost-units/${encodeURIComponent(selected.value)}/rates`))
  } catch (cause) {
    rates.value = []
    rateError.value = messageOf(cause, 'Rates are unavailable.')
  } finally { ratesBusy.value = false }
}
function amountProblem(value: string, label: string): string {
  if (!/^(?:0|[1-9]\d{0,13})(?:\.\d{1,4})?$/.test(value)) return `${label} must be a non-negative amount with at most 4 decimal places.`
  return ''
}
function validateForm(): string {
  form.currency = form.currency.trim().toUpperCase()
  form.internal = form.internal.trim()
  form.bill = form.bill.trim()
  if (!['hour', 'day', 'item'].includes(form.unit)) return 'Choose hour, day, or item.'
  if (!/^[A-Z]{3}$/.test(form.currency)) return 'Currency is a three-letter ISO code.'
  const internal = amountProblem(form.internal, 'Internal amount')
  if (internal) return internal
  const bill = amountProblem(form.bill, 'Bill amount')
  if (bill) return bill
  if (!/^\d{4}-\d{2}-\d{2}$/.test(form.from)) return 'Effective from is a UTC date.'
  if (form.until && (!/^\d{4}-\d{2}-\d{2}$/.test(form.until) || form.until <= form.from)) return 'Effective until is a UTC date after the start.'
  return ''
}
function requestBody(): string {
  const until = form.until ? `,"effective_until":${JSON.stringify(form.until)}` : ''
  return `{"unit":${JSON.stringify(form.unit)},"currency":${JSON.stringify(form.currency)},"internal_amount":${form.internal},"bill_amount":${form.bill},"effective_from":${JSON.stringify(form.from)}${until}}`
}
async function submit() {
  if (!current.value || pending.value || !admin.value) return
  const problem = validateForm()
  actionError.value = problem
  if (problem) return
  pending.value = true
  notice.value = ''
  try {
    await readBody(`/cost-units/${encodeURIComponent(current.value.id)}/rates`, 'POST', requestBody())
    form.internal = ''
    form.bill = ''
    form.until = ''
    notice.value = 'Rate recorded.'
    await loadRates()
  } catch (cause) {
    actionError.value = messageOf(cause, 'The rate was not recorded.')
  } finally { pending.value = false }
}
</script>

<template>
  <section class="cost-units" aria-labelledby="cost-units-title">
    <header class="cost-head">
      <div>
        <p class="eyebrow">Rates</p>
        <h1 id="cost-units-title">Cost units</h1>
        <p>Each price starts on a UTC date and stays in its own currency. A later price does not rewrite an earlier one. This is not an invoice.</p>
      </div>
      <button class="button secondary" type="button" :disabled="busy" @click="loadUnits">
        <svg class="glyph" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 12a8 8 0 1 1-2.3-5.7" /><path d="M20 4v6h-6" /></svg>
        Refresh
      </button>
    </header>
    <p v-if="listError" class="error" role="alert">{{ listError }}</p>
    <p v-if="truncated" class="notice">Showing the first 2,000 cost units, ordered by key.</p>
    <div class="layout">
      <div class="glass-card unit-list">
        <h2>Units</h2>
        <p v-if="busy" class="muted">Loading cost units…</p>
        <p v-else-if="!units.length" class="muted">No cost units yet. Imported cost units keep their original keys under the cost_unit kind.</p>
        <ul v-else>
          <li v-for="unit in units" :key="unit.id">
            <button type="button" :aria-current="unit.id === selected ? 'true' : undefined" @click="selected = unit.id">
              <span class="key">{{ unit.key }}</span>
              <span class="title">{{ unit.title }}</span>
              <span v-if="classicRecord(unit)" class="badge">Imported</span>
            </button>
          </li>
        </ul>
      </div>
      <div class="detail">
        <article v-if="current" class="glass-card unit-card">
          <div class="unit-heading">
            <div>
              <p class="eyebrow">{{ current.state }}</p>
              <h2>{{ current.title }}</h2>
            </div>
            <p class="key-large">{{ current.key }}</p>
          </div>
          <dl>
            <div><dt>Kind</dt><dd>cost_unit</dd></div>
            <div v-if="classicID(current)"><dt>Classic id</dt><dd>{{ classicID(current) }}</dd></div>
            <div v-if="classicSource(current)"><dt>Source</dt><dd>{{ classicSource(current) }}</dd></div>
          </dl>
          <p v-if="rateError" class="error" role="alert">{{ rateError }}</p>
          <p v-else-if="ratesBusy" class="muted">Loading rates…</p>
          <p v-else-if="!groups.length" class="muted">No rates for this cost unit.</p>
          <section v-for="group in groups" :key="group.currency" class="currency-group">
            <h3>{{ group.currency }}</h3>
            <table>
              <caption class="sr-only">{{ group.currency }} rates for {{ current.key }}</caption>
              <thead><tr><th>From</th><th>Until</th><th>Unit</th><th>Internal</th><th>Bill</th></tr></thead>
              <tbody>
                <tr v-for="rate in group.rates" :key="rate.id">
                  <td>{{ rate.effective_from }}</td>
                  <td>{{ rate.effective_until ?? 'Open' }}</td>
                  <td>{{ rate.unit }}</td>
                  <td class="amount">{{ rate.internal_amount }}</td>
                  <td class="amount">{{ rate.bill_amount }}</td>
                </tr>
              </tbody>
            </table>
          </section>
        </article>
        <p v-else class="glass-card empty-detail">Select a cost unit to see its rates.</p>
        <form v-if="admin && current" class="glass-card rate-form" @submit.prevent="submit">
          <h2>New rate</h2>
          <p>Recording a price adds a row. If one open price in this currency and unit already covers the start date, it closes the day the new price begins.</p>
          <div class="fields">
            <label>Unit
              <select v-model="form.unit" :disabled="pending">
                <option value="hour">Hour</option>
                <option value="day">Day</option>
                <option value="item">Item</option>
              </select>
            </label>
            <label>Currency
              <input v-model="form.currency" maxlength="3" autocapitalize="characters" autocomplete="off" :disabled="pending" @change="form.currency = form.currency.toUpperCase()" />
            </label>
            <label>Internal amount
              <input v-model="form.internal" inputmode="decimal" autocomplete="off" spellcheck="false" :disabled="pending" />
            </label>
            <label>Bill amount
              <input v-model="form.bill" inputmode="decimal" autocomplete="off" spellcheck="false" :disabled="pending" />
            </label>
            <label>Effective from
              <input v-model="form.from" type="date" :disabled="pending" />
            </label>
            <label>Effective until
              <input v-model="form.until" type="date" :disabled="pending" />
            </label>
          </div>
          <p v-if="actionError" class="error" role="alert">{{ actionError }}</p>
          <p v-if="notice" role="status">{{ notice }}</p>
          <button class="button" type="submit" :disabled="pending || ratesBusy">
            <svg class="glyph" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14M5 12h14" /></svg>
            Record rate
          </button>
        </form>
        <p v-else-if="current" class="notice">A tenant admin records a new rate. Prices already stored stay as they are.</p>
      </div>
    </div>
  </section>
</template>

<style scoped>
.cost-units { max-width: 1120px; margin: 0 auto; padding: 28px 28px 48px; display: grid; gap: 22px; }
.cost-head, .unit-heading { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.cost-head h1, .unit-card h2, .rate-form h2, .unit-list h2 { margin-top: 6px; }
.layout { display: grid; grid-template-columns: minmax(240px, 320px) minmax(0, 1fr); gap: 20px; align-items: start; }
.detail { min-width: 0; }
.unit-list, .unit-card, .rate-form, .empty-detail { padding: 22px; }
.unit-list ul { list-style: none; margin: 16px 0 0; padding: 0; display: grid; gap: 8px; }
.unit-list button { width: 100%; min-height: 64px; padding: 10px 12px; display: grid; grid-template-columns: auto 1fr; gap: 4px 10px; align-items: center; text-align: left; border: 1px solid var(--line); border-radius: var(--radius-s); background: var(--surface); color: var(--ink); }
.unit-list button[aria-current="true"] { border-color: var(--teal); background: var(--aqua-3); }
.key, .key-large, .amount { font-family: var(--mono); }
.key { color: var(--teal-ink); font-size: 12px; }
.title { overflow-wrap: anywhere; }
.badge { grid-column: 1 / -1; justify-self: start; border: 1px solid var(--line-2); border-radius: 20px; padding: 2px 8px; font: 11px/1.4 var(--mono); color: var(--ink-2); }
.key-large { margin: 0; font-size: 18px; color: var(--teal-ink); }
.unit-card dl { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; margin: 18px 0 0; }
.unit-card dt, .muted { color: var(--ink-2); }
.unit-card dd { margin: 2px 0 0; overflow-wrap: anywhere; }
.currency-group { margin-top: 18px; overflow-x: auto; max-width: 100%; }
.currency-group h3 { margin: 0 0 8px; font: 600 14px/1.4 var(--mono); letter-spacing: .08em; }
table { width: 100%; border-collapse: collapse; font-size: 14px; }
th, td { text-align: left; padding: 8px 6px; border-bottom: 1px solid var(--line); }
th { color: var(--ink-2); font-weight: 500; }
.amount { text-align: right; }
.rate-form { display: grid; gap: 14px; margin-top: 20px; }
.fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.fields label { display: grid; gap: 6px; color: var(--ink-2); font-size: 13px; }
.fields input, .fields select { width: 100%; min-width: 0; min-height: 44px; padding: 10px 12px; border: 1px solid var(--line-2); border-radius: var(--radius-s); background: var(--surface); color: var(--ink); font: inherit; }
.notice { margin: 0; color: var(--ink-2); }
.glyph { width: 18px; height: 18px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
.empty-detail { color: var(--ink-2); }
@media (max-width: 800px) {
  .cost-units { padding: 20px 16px 36px; }
  .layout, .fields, .unit-card dl, .cost-head { grid-template-columns: 1fr; }
  .cost-head { display: grid; }
}
</style>
