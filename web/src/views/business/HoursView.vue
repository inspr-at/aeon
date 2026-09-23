<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, getKinds, getNodes, type WorkNode } from '../../lib/api'
import { useSession } from '../../stores/session'

type Period = { id: string; principal_id: string; starts_at: string; ends_at: string; state: 'open' | 'approved'; revision: number; approval?: { approved_by_principal_id: string; total_seconds: number } }
type Entry = { id: string; principal_id: string; node_id: string; source: string; started_at: string; duration_seconds: number; rate_amount: string; amount: string; currency: string; note: string }
type Totals = { duration_seconds: number; amounts: { currency: string; amount: string }[] }
const session = useSession()
const isAdmin = computed(() => session.identity?.principal.kind === 'person' && session.identity.principal.roles?.includes('admin'))
const isPerson = computed(() => session.identity?.principal.kind === 'person')
const periods = ref<Period[]>([]), selected = ref<Period | null>(null), entries = ref<Entry[]>([])
const nodes = ref<WorkNode[]>([]), costs = ref<WorkNode[]>([]), digest = ref('')
const loading = ref(true), busy = ref(false), error = ref(''), notice = ref(''), ready = ref(false), review = ref(false)
const principal = ref(''), periodStart = ref(''), periodEnd = ref('')
const source = ref<'manual' | 'agent_run'>('manual'), node = ref(''), cost = ref(''), currency = ref('EUR')
const started = ref(''), ended = ref(''), run = ref(''), note = ref('')
const totalNode = ref(''), approvedOnly = ref(false), totals = ref<Totals | null>(null)
let selectionSequence = 0
// Preserve monetary number tokens before JSON.parse; never pass money through
// JavaScript Number. Other numeric fields (seconds/revision) remain numbers.
function exactJSON(raw: string) {
  let money = false
  return JSON.parse(raw.replace(/"(?:\\.|[^"\\])*"|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/g, (token, offset: number) => {
    if (token.startsWith('"')) {
      money = (token === '"amount"' || token === '"rate_amount"') && /^\s*:/.test(raw.slice(offset + token.length))
      return token
    }
    const result = money ? `"${token}"` : token
    money = false
    return result
  }))
}
async function request<T>(path: string, body?: unknown): Promise<{ data: T; etag: string }> {
  const response = await api(path, body === undefined ? {} : { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
  const raw = await response.text()
  const data = raw ? exactJSON(raw) : null
  if (!response.ok) throw new Error(data?.error || `Request failed (${response.status})`)
  return { data, etag: response.headers.get('X-Entries-SHA256')?.replace(/^"|"$/g, '') || '' }
}
function label(id: string) { const n = nodes.value.find(n => n.id === id); return n ? `${n.key} · ${n.title}` : id }
function duration(seconds: number) { return `${Math.floor(seconds / 3600)}h ${Math.floor(seconds % 3600 / 60)}m ${seconds % 60}s` }
function date(value: string) { return new Date(value).toLocaleString() }
function iso(value: string) { if (!value) throw new Error('Choose start and end times.'); return new Date(value).toISOString() }
async function choose(id: string) {
  const sequence = ++selectionSequence
  selected.value = null; entries.value = []; digest.value = ''; review.value = false
  if (!id) return
  loading.value = true; error.value = ''
  try {
    // Revision checks detect changes between these HTTP reads. The approval
    // digest remains pinned to the reviewed period, never silently refreshed.
    const first = await request<Period>(`/time-periods/${encodeURIComponent(id)}`)
    const list = await request<Entry[]>(`/time-entries?period_id=${encodeURIComponent(id)}`)
    const last = await request<Period>(`/time-periods/${encodeURIComponent(id)}`)
    if (sequence !== selectionSequence) return
    if (first.data.revision !== last.data.revision || first.etag !== last.etag) throw new Error('Entries changed while loading. Select the period again.')
    selected.value = last.data; entries.value = list.data; digest.value = last.etag
  } catch (e) { if (sequence === selectionSequence) error.value = (e as Error).message }
  finally { if (sequence === selectionSequence) loading.value = false }
}
async function load() {
  loading.value = true; error.value = ''; ready.value = false
  try {
    periods.value = (await request<Period[]>('/time-periods')).data
    const kinds = await getKinds()
    const kind = kinds.items.find(k => k.slug === 'cost_unit')
    // Follow R1 pagination so imported cost units/projects remain selectable.
    const all: WorkNode[] = []; let cursor: string | null = null
    do { const page = await getNodes({ limit: 200, ...(cursor ? { cursor } : {}) }); all.push(...page.items); cursor = page.next_cursor } while (cursor)
    nodes.value = all; costs.value = all.filter(n => n.kind_id === kind?.id)
    principal.value ||= session.identity?.principal.id || ''
    if (!isPerson.value) source.value = 'agent_run'
    ready.value = true
    if (periods.value[0]) await choose(selected.value?.id || periods.value[0].id)
  } catch (e) { error.value = (e as Error).message }
  finally { loading.value = false }
}
async function mutate(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try { await action() } catch (e) { error.value = (e as Error).message; review.value = false }
  finally { busy.value = false }
}
function createPeriod() { return mutate(async () => {
  const result = await request<Period>('/time-periods', { principal_id: principal.value, starts_at: iso(periodStart.value), ends_at: iso(periodEnd.value) })
  periods.value = [result.data, ...periods.value]; await choose(result.data.id); notice.value = 'Period opened.'
}) }
function createEntry() { return mutate(async () => {
  if (!selected.value) return
  const id = selected.value.id
  const body = { period_id: id, cost_unit_node_id: cost.value, currency: currency.value, source: source.value, note: note.value,
    ...(source.value === 'manual' ? { principal_id: selected.value.principal_id, node_id: node.value, started_at: iso(started.value), ended_at: iso(ended.value) } : { agent_run_id: run.value }) }
  await request<Entry>('/time-entries', body)
  note.value = ''; await choose(id); notice.value = 'Time recorded.'; totals.value = null
}) }
function approve() { return mutate(async () => {
  if (!selected.value || !digest.value || !review.value) return
  const result = await request<Period>(`/time-periods/${selected.value.id}/approve`, { expected_revision: selected.value.revision, expected_entries_sha256: digest.value })
  selected.value = result.data; periods.value = periods.value.map(p => p.id === result.data.id ? result.data : p)
  review.value = false; notice.value = 'Period approved and closed.'; totals.value = null
}) }
async function loadTotals() { await mutate(async () => { totals.value = null; totals.value = (await request<Totals>(`/nodes/${encodeURIComponent(totalNode.value)}/time-totals?approved_only=${approvedOnly.value}`)).data }) }
onMounted(load)
</script>

<template>
  <section class="hours-view" aria-labelledby="hours-title">
    <header><div><p class="eyebrow">Business</p><h1 id="hours-title">Hours</h1><p>Recorded work, frozen rates and approved periods.</p></div><button :disabled="busy || loading" @click="load">Refresh</button></header>
    <p v-if="error" role="alert" class="error">{{ error }} <button :disabled="busy" @click="load">Reload</button></p>
    <p v-if="notice" role="status">{{ notice }}</p>
    <p v-if="loading" role="status">Loading hours…</p>
    <template v-if="ready">
      <div class="columns">
        <section class="card" aria-labelledby="periods-title">
          <h2 id="periods-title">Time periods</h2>
          <label>Period<select :value="selected?.id || ''" :disabled="busy || loading" @change="choose(($event.target as HTMLSelectElement).value)"><option value="">Select a period</option><option v-for="p in periods" :key="p.id" :value="p.id">{{ date(p.starts_at) }} – {{ date(p.ends_at) }} · {{ p.state }}</option></select></label>
          <p v-if="!periods.length">No periods yet. Open one to start recording time.</p>
          <details><summary>Open a period</summary><form @submit.prevent="createPeriod"><label>Principal ID<input v-model="principal" required :readonly="!isAdmin" /></label><label>Period starts<input v-model="periodStart" type="datetime-local" step="1" required /></label><label>Period ends<input v-model="periodEnd" type="datetime-local" step="1" required /></label><button :disabled="busy || loading" class="primary">Open period</button></form></details>
        </section>
        <section class="card" aria-labelledby="totals-title">
          <h2 id="totals-title">Subtree totals</h2>
          <form @submit.prevent="loadTotals"><label>Root work item<select v-model="totalNode" required @change="totals = null"><option value="">Select work item</option><option v-for="n in nodes" :key="n.id" :value="n.id">{{ n.key }} · {{ n.title }}</option></select></label><label class="check"><input v-model="approvedOnly" type="checkbox" @change="totals = null" />Approved periods only</label><button :disabled="busy || !totalNode">Calculate totals</button></form>
          <div v-if="totals" aria-live="polite"><strong>{{ duration(totals.duration_seconds) }}</strong><p v-for="a in totals.amounts" :key="a.currency">{{ a.currency }} {{ a.amount }}</p><p v-if="!totals.amounts.length">No recorded time.</p></div>
        </section>
      </div>
      <section v-if="selected" class="card" aria-labelledby="entries-title">
        <header><div><h2 id="entries-title">{{ selected.state === 'approved' ? 'Approved period' : 'Open period' }}</h2><p>{{ date(selected.starts_at) }} – {{ date(selected.ends_at) }}</p><p>Principal: {{ selected.principal_id }}</p></div><span class="badge">{{ selected.state }}</span></header>
        <p v-if="selected.approval">Approved by {{ selected.approval.approved_by_principal_id }} · {{ duration(selected.approval.total_seconds) }}. This period is closed.</p>
        <div class="table-scroll"><table><caption>Time entries</caption><thead><tr><th>Work item</th><th>Started</th><th>Source</th><th>Duration</th><th>Hourly rate</th><th>Amount</th></tr></thead><tbody><tr v-for="entry in entries" :key="entry.id"><td>{{ label(entry.node_id) }}<small v-if="entry.note">{{ entry.note }}</small></td><td>{{ date(entry.started_at) }}</td><td>{{ entry.source === 'agent_run' ? 'Agent run' : 'Manual' }}</td><td>{{ duration(entry.duration_seconds) }}</td><td>{{ entry.currency }} {{ entry.rate_amount }}</td><td>{{ entry.currency }} {{ entry.amount }}</td></tr></tbody></table></div>
        <p v-if="!entries.length">No time entries in this period.</p>
        <template v-if="selected.state === 'open'">
          <details><summary>Record time</summary><form class="entry-form" @submit.prevent="createEntry">
            <label>Source<select v-model="source"><option v-if="isPerson" value="manual">Manual time</option><option value="agent_run">Terminal agent run</option></select></label>
            <template v-if="source === 'manual'"><label>Work item<select v-model="node" required><option value="">Select work item</option><option v-for="n in nodes" :key="n.id" :value="n.id">{{ n.key }} · {{ n.title }}</option></select></label><label>Started<input v-model="started" type="datetime-local" step="1" required /></label><label>Ended<input v-model="ended" type="datetime-local" step="1" required /></label></template>
            <label v-else>Agent run ID<input v-model="run" required /><small>Uses the run’s recorded principal, work item and terminal interval.</small></label>
            <label>Cost unit<select v-model="cost" required><option value="">Select cost unit</option><option v-for="c in costs" :key="c.id" :value="c.id">{{ c.key }} · {{ c.title }}</option></select></label><label>Currency<input v-model="currency" pattern="[A-Z]{3}" maxlength="3" required /></label><label>Note<textarea v-model="note" maxlength="65536" /></label><p>The effective hourly bill rate is frozen when recorded. Entries cannot be edited.</p><button class="primary" :disabled="busy || loading || !costs.length">Record time</button>
          </form></details>
          <div v-if="isAdmin" class="approval"><p>Approval closes this period and seals exactly the entries shown above.</p><label class="check"><input v-model="review" type="checkbox" :disabled="busy || !digest" />I have reviewed these entries</label><button :disabled="busy || loading || !review || !digest" @click="approve">Approve and close period</button><p v-if="!digest">Reload to obtain the current approval snapshot.</p></div>
        </template>
      </section>
    </template>
  </section>
</template>

<style scoped>
.hours-view{padding:24px;overflow:auto;min-height:0;color:var(--ink);width:100%;box-sizing:border-box}header{display:flex;justify-content:space-between;align-items:start;gap:16px}h1{font-size:28px;margin:0}h2{font-size:18px;margin:0 0 16px}p{color:var(--ink-2);line-height:1.5}.eyebrow{color:var(--teal);margin:0 0 5px;font-size:12px;text-transform:uppercase;letter-spacing:.1em}.columns{display:grid;grid-template-columns:1fr 1fr;gap:20px}.card{background:var(--surface);border:1px solid var(--line);border-radius:var(--radius);padding:20px;margin:20px 0;min-width:0}form{display:grid;gap:12px;margin:14px 0}label{display:grid;gap:6px;color:var(--ink-2);font-size:13px}input,select,textarea,button{font:inherit;color:var(--ink);background:var(--surface);border:1px solid var(--line-2);border-radius:var(--radius-s);padding:10px;min-width:0}button{cursor:pointer;justify-self:start}button.primary{background:var(--teal);color:var(--button-ink)}button:disabled{opacity:.5;cursor:default}button:focus-visible,input:focus-visible,select:focus-visible,summary:focus-visible{outline:2px solid var(--teal);outline-offset:3px}.check{display:flex;align-items:center;gap:8px}.check input{margin:0}summary{cursor:pointer;padding:12px 0;color:var(--teal)}table{border-collapse:collapse;width:100%;font-size:13px}caption{text-align:left;font-weight:600;margin-bottom:10px}th,td{text-align:left;border-bottom:1px solid var(--line);padding:12px;vertical-align:top;font-variant-numeric:tabular-nums}small{display:block;color:var(--ink-2);margin-top:4px;overflow-wrap:anywhere}.table-scroll{overflow:auto}.badge{background:var(--aqua-3);border-radius:20px;padding:6px 12px}.error{color:var(--danger)}.approval{border-top:1px solid var(--line);padding-top:16px;margin-top:16px}.approval button{margin-top:12px}.entry-form{max-width:620px}@media(max-width:760px){.columns{grid-template-columns:1fr;gap:0}.hours-view{padding:14px}.card{padding:16px;margin:12px 0}header{flex-wrap:wrap}}
</style>
