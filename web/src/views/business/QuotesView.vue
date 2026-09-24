<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Parked (AEON-70, 2026-09-24): quotes and organisations will be ported from Markus's current classic Paimos quote builder; this file is not routed or linked. -->
<script setup lang="ts">
import { setPageTitle } from '../../lib/brand'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getQuote, type Quote, type QuoteState } from '../../lib/business'
import { command, consume, run } from '../../lib/commands'
import { toast } from '../../lib/toast'
import { absoluteTime, highlight, plural, relativeTime } from '../../lib/work'
import { compareAmounts, sumAmounts } from '../../components/business/money'
import { useBusiness } from '../../stores/business'
import AppIcon from '../../components/business/BizIcon.vue'
import BusinessPage from '../../components/business/BusinessPage.vue'
import ChoiceFacet from '../../components/business/ChoiceFacet.vue'
import MoneyText from '../../components/business/MoneyText.vue'
import QuoteCreateDialog from '../../components/business/QuoteCreateDialog.vue'
import QuoteStatus, { QUOTE_STATES } from '../../components/business/QuoteStatus.vue'
import QuoteWorkspace from '../../components/business/QuoteWorkspace.vue'

type SortField = 'key' | 'customer' | 'title' | 'state' | 'total' | 'updated'
const business = useBusiness()
const route = useRoute()
const router = useRouter()
const now = ref(Date.now())
const search = ref<HTMLInputElement>()
const grid = ref<HTMLTableElement>()
const workspace = ref<InstanceType<typeof QuoteWorkspace>>()
const creator = ref<InstanceType<typeof QuoteCreateDialog>>()
const cursor = ref<string | null>(null)
const draftQ = ref(typeof route.query.q === 'string' ? route.query.q : '')
const stickMark = ref<HTMLElement>()
const stuck = ref(false)
let stick: IntersectionObserver | undefined

// ---------- URL state ----------
const quoteKey = computed(() => typeof route.params.quoteKey === 'string' ? route.params.quoteKey : '')
const full = computed(() => !!quoteKey.value && route.query.view === 'full')
const q = computed(() => typeof route.query.q === 'string' ? route.query.q : '')
const states = computed(() => typeof route.query.status === 'string' && route.query.status ? route.query.status.split(',') : [])
const customers = computed(() => typeof route.query.customer === 'string' && route.query.customer ? route.query.customer.split(',') : [])
const sort = computed<{ field: SortField; desc: boolean }>(() => {
  const raw = typeof route.query.sort === 'string' ? route.query.sort : '-updated'
  const desc = raw.startsWith('-'), field = (desc ? raw.slice(1) : raw) as SortField
  return ['key', 'customer', 'title', 'state', 'total', 'updated'].includes(field) ? { field, desc } : { field: 'updated', desc: true }
})
function update(patch: Record<string, string | undefined>) {
  const query = { ...route.query, ...patch }
  for (const key of Object.keys(query)) if (!query[key]) delete query[key]
  void router.replace({ path: route.path, query })
}
let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(draftQ, value => { clearTimeout(searchTimer); searchTimer = setTimeout(() => update({ q: value.trim() || undefined }), 200) })
function toggle(list: string[], value: string) { return list.includes(value) ? list.filter(v => v !== value) : [...list, value] }
function sortBy(field: SortField) {
  const next = sort.value.field !== field ? (field === 'updated' || field === 'total' ? `-${field}` : field) : sort.value.desc ? field : `-${field}`
  update({ sort: next === '-updated' ? undefined : next })
}

// ---------- Rows ----------
const orgName = (id: string) => business.organisation(id)?.title ?? ''
const keyNumber = (key: string) => Number(key.split('-').pop() ?? 0)
const ORDER: Record<QuoteState, number> = { draft: 0, issued: 1, accepted: 2, void: 3 }
const filtered = computed(() => {
  const needle = q.value.toLowerCase()
  return business.quotes
    .filter(quote => !states.value.length || states.value.includes(quote.state))
    .filter(quote => !customers.value.length || customers.value.includes(quote.customer_org_node_id))
    .filter(quote => !needle || `${quote.key} ${quote.title} ${orgName(quote.customer_org_node_id)}`.toLowerCase().includes(needle))
})
const rows = computed(() => {
  const { field, desc } = sort.value
  const compare = (a: Quote, b: Quote): number => {
    switch (field) {
      case 'key': return keyNumber(a.key) - keyNumber(b.key)
      case 'customer': return orgName(a.customer_org_node_id).localeCompare(orgName(b.customer_org_node_id))
      case 'title': return a.title.localeCompare(b.title)
      case 'state': return ORDER[a.state] - ORDER[b.state]
      case 'total': {
        // Totals compare within a currency; currencies group alphabetically, empty quotes last.
        if (!a.current || !b.current) return (a.current ? 0 : 1) - (b.current ? 0 : 1)
        return a.current.currency.localeCompare(b.current.currency) || compareAmounts(a.current.total, b.current.total)
      }
      default: return Date.parse(a.updated_at) - Date.parse(b.updated_at)
    }
  }
  return [...filtered.value].sort((a, b) => (desc ? -compare(a, b) : compare(a, b)) || keyNumber(b.key) - keyNumber(a.key))
})
const selected = computed(() => quoteKey.value ? business.quoteByKey(quoteKey.value) ?? fetched.value : null)
const fetched = ref<Quote | null>(null)
const resolveError = ref('')
const position = computed(() => {
  const index = selected.value ? rows.value.findIndex(r => r.quote_node_id === selected.value!.quote_node_id) : -1
  return index === -1 ? null : { index, count: rows.value.length }
})
const openCount = computed(() => business.quotes.filter(x => x.state === 'draft' || x.state === 'issued').length)
const openValue = computed(() => {
  const by = new Map<string, string[]>()
  for (const x of business.quotes) if ((x.state === 'draft' || x.state === 'issued') && x.current) by.set(x.current.currency, [...(by.get(x.current.currency) ?? []), x.current.total])
  return [...by.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([currency, totals]) => ({ currency, amount: sumAmounts(totals) }))
})
const stateOptions = computed(() => QUOTE_STATES.map(s => ({ value: s.value, label: s.label, state: s.value, count: business.quotes.filter(x => x.state === s.value).length })))
const customerOptions = computed(() => {
  const ids = [...new Set(business.quotes.map(x => x.customer_org_node_id))]
  return ids.map(id => ({ value: id, label: orgName(id) || 'Organisation', count: business.quotes.filter(x => x.customer_org_node_id === id).length })).sort((a, b) => a.label.localeCompare(b.label))
})
const filtering = computed(() => !!q.value || states.value.length > 0 || customers.value.length > 0)
function clearFilters() { draftQ.value = ''; update({ q: undefined, status: undefined, customer: undefined }) }

// A key the list does not hold (deleted, other filters, deep link) is fetched on its own.
watch(quoteKey, async key => {
  resolveError.value = ''
  if (!key) return
  await business.loadQuotes()
  const known = business.quoteByKey(key)
  if (known) { cursor.value = known.quote_node_id; fetched.value = null; return }
  resolveError.value = `${key.toUpperCase()} is not a quote in this workspace, or it was deleted.`
}, { immediate: true })

// ---------- Panel ----------
let openedFromList = false
function openQuote(quote: Quote) {
  cursor.value = quote.quote_node_id
  const location = { path: `/business/quotes/${encodeURIComponent(quote.key)}`, query: route.query }
  if (quoteKey.value) { void router.replace(location); return }
  openedFromList = true
  void router.push(location)
}
function listQuery() { const { view: _view, ...rest } = route.query; return rest }
function closePanel() {
  if (!quoteKey.value) return
  const back = !full.value && openedFromList && typeof window.history.state?.back === 'string'
  openedFromList = false
  if (back) router.back()
  else void router.replace({ path: '/business/quotes', query: listQuery() })
  void nextTick(() => grid.value?.focus({ preventScroll: true }))
}
function expand() { if (quoteKey.value && !full.value) void router.push({ path: route.path, query: { ...route.query, view: 'full' } }) }
function collapse() { if (full.value) void router.replace({ path: route.path, query: listQuery() }) }
function changed(quote: Quote) { business.upsertQuote(quote); if (fetched.value?.quote_node_id === quote.quote_node_id) fetched.value = quote }
async function created(quote: Quote) {
  toast(`Created ${quote.key}. Add its lines next.`)
  openQuote(quote)
  await nextTick(); setTimeout(() => workspace.value?.focusLines(), 350)
}
function newQuote() { if (business.staff) creator.value?.open(customers.value.length === 1 ? { orgId: customers.value[0] } : {}) }
async function reloadSelected() {
  if (!selected.value) return
  try { changed(await getQuote(selected.value.quote_node_id)) } catch { /* the panel keeps its state */ }
}

// ---------- Keyboard ----------
function typing(target: EventTarget | null) { return target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) }
function move(step: number) {
  const list = rows.value
  if (!list.length) return
  const from = quoteKey.value && selected.value ? selected.value.quote_node_id : cursor.value
  const index = list.findIndex(r => r.quote_node_id === from)
  const next = list[index === -1 ? (step > 0 ? 0 : list.length - 1) : Math.max(0, Math.min(list.length - 1, index + step))]
  cursor.value = next.quote_node_id
  if (quoteKey.value) openQuote(next)
  void nextTick(() => document.getElementById(`quote-${next.quote_node_id}`)?.scrollIntoView({ block: 'nearest' }))
}
function keydown(event: KeyboardEvent) {
  if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return
  if (document.querySelector('dialog[open], .floating')) return
  const target = event.target as HTMLElement | null
  if (typing(target)) {
    if (event.key === 'ArrowDown' && target === search.value) { event.preventDefault(); search.value?.blur(); move(cursor.value ? 0 : 1); grid.value?.focus() }
    if (event.key === 'Escape' && target === search.value && draftQ.value) { event.preventDefault(); draftQ.value = '' }
    return
  }
  const row = rows.value.find(r => r.quote_node_id === cursor.value)
  switch (event.key) {
    case 'j': case 'ArrowDown': event.preventDefault(); move(1); break
    case 'k': case 'ArrowUp': event.preventDefault(); move(-1); break
    case 'Enter': case 'o':
      if (event.key === 'Enter' && target?.closest('button, a')) return
      if (row) { event.preventDefault(); openQuote(row) }
      break
    case 'Escape': if (quoteKey.value) { event.preventDefault(); if (full.value) collapse(); else closePanel() } break
    case '/': if (!full.value) { event.preventDefault(); search.value?.focus() } break
    case 'n': if (business.staff) { event.preventDefault(); newQuote() } break
    case 'f': if (quoteKey.value) { event.preventDefault(); if (full.value) collapse(); else expand() } break
    case 'e': if (quoteKey.value) { event.preventDefault(); workspace.value?.editTitle() } break
    case 'd': if (selected.value?.current) { event.preventDefault(); void router.push(`/business/quotes/${encodeURIComponent(selected.value.key)}/document`) } break
  }
}
watch(command, value => {
  if (value?.command.name !== 'new-quote') return
  consume()
  void nextTick(newQuote)
}, { immediate: true })
watch(() => route.query.new, value => { if (value) { update({ new: undefined }); void business.loadPlugins().then(() => nextTick(newQuote)) } }, { immediate: true })

let clock: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  void business.loadPlugins().then(() => {
    if (!business.open.quotes) return
    void business.loadQuotes(true); void business.loadCRM(); void business.loadCostUnits(); void business.loadPrincipals()
  })
  window.addEventListener('keydown', keydown)
  clock = setInterval(() => { now.value = Date.now() }, 60_000)
})
watch(stickMark, element => {
  stick?.disconnect()
  const root = document.getElementById('main')
  if (!element || !root) return
  stick = new IntersectionObserver(([entry]) => { stuck.value = !entry.isIntersecting }, { root, threshold: 0 })
  stick.observe(element)
}, { flush: 'post' })
onBeforeUnmount(() => { window.removeEventListener('keydown', keydown); clearInterval(clock); clearTimeout(searchTimer); stick?.disconnect() })
watch(selected, value => { setPageTitle(value ? `${value.key} ${value.title}` : 'Quotes') })
function rowClick(event: MouseEvent, quote: Quote) {
  if ((event.target as HTMLElement).closest('button')) return
  if (event.metaKey || event.ctrlKey) { window.open(`/business/quotes/${encodeURIComponent(quote.key)}`, '_blank', 'noopener'); return }
  openQuote(quote)
}
function linkClick(event: MouseEvent) { if (!(event.metaKey || event.ctrlKey || event.shiftKey || event.button === 1)) event.preventDefault() }
function ariaSort(field: SortField) { return sort.value.field === field ? (sort.value.desc ? 'descending' : 'ascending') : 'none' }
const columns: { field: SortField; label: string; cls: string }[] = [
  { field: 'key', label: 'Key', cls: 'c-key' }, { field: 'customer', label: 'Customer', cls: 'c-customer' }, { field: 'title', label: 'Title', cls: 'c-title' },
  { field: 'state', label: 'Status', cls: 'c-status' }, { field: 'total', label: 'Total', cls: 'c-total' }, { field: 'updated', label: 'Updated', cls: 'c-updated' },
]
</script>

<template>
  <BusinessPage v-show="!full" title="Quotes" area="quotes" :panel-open="!!quoteKey && !full">
    <template #summary>
      <span v-if="business.quotesLoaded && !business.quotes.length">No quotes yet</span>
      <template v-else-if="business.quotesLoaded">
        <span>{{ plural(business.quotes.length, 'quote') }} · {{ openCount }} open</span>
        <template v-for="row in openValue" :key="row.currency"> · <MoneyText :amount="row.amount" :currency="row.currency" /></template>
        <span v-if="openValue.length"> open value</span>
      </template>
      <span v-else class="skeleton summary-skeleton" />
    </template>

    <div ref="stickMark" class="stick-mark" aria-hidden="true" />
    <div class="toolbar" :class="{ stuck }" role="toolbar" aria-label="Quote list">
      <label class="search-field list-search">
        <AppIcon name="search" :size="14" />
        <input ref="search" v-model="draftQ" class="field" type="search" placeholder="Search key, title or customer" aria-label="Search quotes" autocomplete="off" spellcheck="false" />
        <kbd v-if="!draftQ" class="keycap slash" aria-hidden="true">/</kbd>
      </label>
      <ChoiceFacet label="Status" :options="stateOptions" :selected="states" @toggle="value => update({ status: toggle(states, value).join(',') || undefined })" @clear="update({ status: undefined })" />
      <ChoiceFacet label="Customer" :options="customerOptions" :selected="customers" @toggle="value => update({ customer: toggle(customers, value).join(',') || undefined })" @clear="update({ customer: undefined })" />
      <button v-if="filtering" type="button" class="btn sm ghost" @click="clearFilters">Clear</button>
      <span class="spacer" />
      <span class="count mono">{{ business.quotesLoaded ? plural(rows.length, 'quote') : '' }}</span>
      <button v-if="business.staff" type="button" class="btn primary new-btn" aria-keyshortcuts="n" data-tip="New quote · n" @click="newQuote"><AppIcon name="plus" :size="14" /><span class="new-label">New quote</span></button>
    </div>

    <div class="table-card">
      <table ref="grid" class="quotes" role="grid" aria-label="Quotes" tabindex="0" :aria-activedescendant="cursor ? `quote-${cursor}` : undefined" :aria-busy="!business.quotesLoaded" @focus="!cursor && rows[0] && (cursor = rows[0].quote_node_id)">
        <thead>
          <tr>
            <th v-for="column in columns" :key="column.field" scope="col" :class="column.cls" :aria-sort="ariaSort(column.field)">
              <button type="button" class="th-sort" :class="{ on: sort.field === column.field }" :data-tip="`Sort by ${column.label.toLowerCase()}`" @click="sortBy(column.field)">
                <span>{{ column.label }}</span>
                <span class="sort-mark" aria-hidden="true"><AppIcon v-if="sort.field === column.field" :name="sort.desc ? 'arrow-down' : 'arrow-up'" :size="11" /></span>
              </button>
            </th>
          </tr>
        </thead>
        <tbody v-if="!business.quotesLoaded && !business.quotesError" class="skeleton-body" aria-hidden="true">
          <tr v-for="i in 6" :key="i" class="quote-row ghost">
            <td class="c-key"><div class="cell"><span class="skeleton sk-key" /></div></td>
            <td class="c-customer"><div class="cell"><span class="skeleton sk-word" /></div></td>
            <td class="c-title"><div class="cell"><span class="skeleton" :style="{ width: `${40 + (i * 29) % 40}%` }" /></div></td>
            <td class="c-status"><div class="cell"><span class="skeleton sk-word" /></div></td>
            <td class="c-total"><div class="cell"><span class="skeleton sk-word" /></div></td>
            <td class="c-updated"><div class="cell"><span class="skeleton sk-time" /></div></td>
          </tr>
        </tbody>
        <tbody v-else>
          <tr
            v-for="quote in rows" :id="`quote-${quote.quote_node_id}`" :key="quote.quote_node_id" class="quote-row" role="row"
            :class="{ cursor: cursor === quote.quote_node_id, open: selected?.quote_node_id === quote.quote_node_id }" :aria-selected="selected?.quote_node_id === quote.quote_node_id"
            @click="rowClick($event, quote)"
          >
            <td class="c-key"><div class="cell"><span class="key"><template v-for="(part, i) in highlight(quote.key, q)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span></div></td>
            <td class="c-customer"><div class="cell"><span class="customer"><template v-for="(part, i) in highlight(orgName(quote.customer_org_node_id) || '—', q)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span></div></td>
            <td class="c-title"><div class="cell">
              <a class="title-link" :href="`/business/quotes/${encodeURIComponent(quote.key)}`" tabindex="-1" @click="linkClick"><template v-for="(part, i) in highlight(quote.title, q)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></a>
              <span v-if="quote.current" class="ver mono" :data-tip="`${quote.current.line_count} ${quote.current.line_count === 1 ? 'line' : 'lines'} in version ${quote.current.version}`">v{{ quote.current.version }}</span>
            </div></td>
            <td class="c-status"><div class="cell"><QuoteStatus :state="quote.state" /><span v-if="quote.viewer_can_accept" class="you-chip">For you</span></div></td>
            <td class="c-total"><div class="cell"><MoneyText v-if="quote.current" :amount="quote.current.total" :currency="quote.current.currency" /><span v-else class="empty">No version</span></div></td>
            <td class="c-updated"><div class="cell"><time :datetime="quote.updated_at" :data-tip="absoluteTime(quote.updated_at)">{{ relativeTime(quote.updated_at, { now }) }}</time></div></td>
          </tr>
        </tbody>
      </table>
      <div v-if="business.quotesError" class="state" role="alert">
        <span class="state-icon danger"><AppIcon name="alert" :size="18" /></span>
        <h2>Quotes could not be loaded</h2>
        <p>{{ business.quotesError }}</p>
        <button type="button" class="btn" @click="business.loadQuotes(true)"><AppIcon name="refresh" :size="14" />Try again</button>
      </div>
      <div v-else-if="business.quotesLoaded && !rows.length" class="state">
        <span class="state-icon"><AppIcon :name="filtering ? 'filter' : 'document'" :size="18" /></span>
        <template v-if="filtering">
          <h2>No quote matches these filters</h2>
          <p>{{ q ? `Nothing with “${q}” in its key, title or customer.` : 'Try fewer filters.' }}</p>
          <button type="button" class="btn" @click="clearFilters">Clear filters</button>
        </template>
        <template v-else>
          <h2>No quotes yet</h2>
          <p>A quote offers a customer lines of work at your rates. Each version is frozen with exact totals; the customer accepts the version you issue.</p>
          <button v-if="business.staff" type="button" class="btn" @click="newQuote"><AppIcon name="plus" :size="14" />New quote</button>
        </template>
      </div>
    </div>
    <p v-if="rows.length" class="hint">
      <kbd class="keycap">j</kbd><kbd class="keycap">k</kbd> move · <kbd class="keycap"><AppIcon name="enter" /></kbd> open · <kbd class="keycap">n</kbd> new quote · <kbd class="keycap">/</kbd> search ·
      <button type="button" class="hint-link" @click="run({ name: 'shortcuts' })"><kbd class="keycap">?</kbd> all shortcuts</button>
    </p>
  </BusinessPage>

  <div v-if="full" class="full-page">
    <QuoteWorkspace
      ref="workspace" :quote="selected" :quote-key="quoteKey.toUpperCase()" :resolving="!business.quotesLoaded" :resolve-error="resolveError" :position="position" mode="full" :now="now"
      @close="collapse" @prev="move(-1)" @next="move(1)" @expand="expand" @collapse="collapse" @changed="changed" @retry="reloadSelected"
    />
  </div>
  <QuoteWorkspace
    v-else-if="quoteKey" ref="workspace" :quote="selected" :quote-key="quoteKey.toUpperCase()" :resolving="!business.quotesLoaded" :resolve-error="resolveError" :position="position" mode="panel" :now="now"
    @close="closePanel" @prev="move(-1)" @next="move(1)" @expand="expand" @collapse="collapse" @changed="changed" @retry="reloadSelected"
  />
  <QuoteCreateDialog ref="creator" @created="created" />
</template>

<style scoped>
.summary-skeleton { display: inline-block; width: 260px; }
.stick-mark { height: 1px; margin-bottom: -1px; }
.toolbar { position: sticky; top: 0; z-index: 5; display: flex; align-items: center; flex-wrap: wrap; gap: 8px 10px; min-height: 52px; margin: -6px -28px 0; padding: 6px 28px 10px; container: toolbar / inline-size; }
.toolbar.stuck { background: var(--glass); box-shadow: 0 1px 0 var(--line), 0 12px 24px -20px rgba(16, 35, 39, .35); backdrop-filter: blur(18px) saturate(1.2); -webkit-backdrop-filter: blur(18px) saturate(1.2); }
.list-search { width: 280px; }
.list-search .field { height: 32px; padding-right: 30px; font-size: 13.5px; }
.list-search .field::-webkit-search-cancel-button { display: none; }
.slash { position: absolute; right: 8px; pointer-events: none; }
@media (hover: none) { .slash { display: none; } }
.spacer { flex: 1; }
.count { font-size: 12px; color: var(--ink-2); white-space: nowrap; }
.new-btn { height: 32px; padding: 0 14px 0 11px; gap: 6px; }
.table-card { position: relative; border-radius: var(--radius); border: 1px solid var(--glass-edge); overflow: clip; background: linear-gradient(165deg, var(--surface-raised-2), var(--glass) 60%); box-shadow: var(--shadow); container: quotes / inline-size; }
.quotes { width: 100%; border-collapse: separate; border-spacing: 0; table-layout: fixed; font-size: 13.5px; }
.quotes:focus-visible { box-shadow: none; }
th.c-key { width: 104px; } th.c-customer { width: 22%; } th.c-status { width: 150px; } th.c-total { width: 170px; } th.c-updated { width: 104px; }
thead th { height: 34px; padding: 0 12px; text-align: left; font-weight: 500; border-bottom: 1px solid var(--line-2); background: var(--surface-raised-2); }
thead th:first-child { padding-left: 18px; }
thead .c-total, thead .c-updated { text-align: right; }
thead .c-total .th-sort, thead .c-updated .th-sort { flex-direction: row-reverse; }
.th-sort { display: inline-flex; align-items: center; gap: 6px; height: 26px; margin: 0 -6px; padding: 0 6px; border: 0; border-radius: 6px; background: transparent; font: 500 10.5px/1 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.th-sort:hover { color: var(--ink); background: var(--row-hover); }
.th-sort.on { color: var(--teal-ink); }
.th-sort:focus-visible { box-shadow: var(--focus-ring); }
.sort-mark { display: inline-flex; min-width: 11px; }
.quote-row { height: 44px; cursor: default; }
.quote-row td { height: 44px; padding: 0 12px; border-bottom: 1px solid var(--line); vertical-align: middle; }
.quote-row td:first-child { padding-left: 18px; }
tbody .quote-row:last-child td { border-bottom: 0; }
.cell { display: flex; align-items: center; gap: 8px; min-width: 0; white-space: nowrap; }
.c-total .cell, .c-updated .cell { justify-content: flex-end; }
@media (hover: hover) { .quote-row:hover td { background: var(--row-hover); } }
.quote-row.cursor td, .quote-row.open td { background: var(--row-selected); }
.quote-row.open { outline: 1px solid var(--chip-teal-line); outline-offset: -1px; }
.key { font: 500 11.5px/18px var(--mono); color: var(--ink-2); letter-spacing: .01em; font-variant-ligatures: none; }
.quote-row.open .key, .quote-row.cursor .key { color: var(--teal-ink); }
.customer { overflow: hidden; text-overflow: ellipsis; color: var(--ink-2); }
.title-link { min-width: 0; overflow: hidden; text-overflow: ellipsis; color: var(--ink); font-weight: 600; text-decoration: none; }
.title-link:focus-visible { box-shadow: none; }
.ver { flex-shrink: 0; height: 17px; padding: 0 6px; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); font-size: 10.5px; line-height: 17px; color: var(--ink-2); }
.you-chip { height: 18px; padding: 0 7px; border-radius: 999px; background: var(--gold-wash); box-shadow: inset 0 0 0 1px rgba(214, 155, 49, .4); color: var(--gold-ink); font: 600 9.5px/18px var(--mono); letter-spacing: .06em; text-transform: uppercase; font-variant-ligatures: none; }
.c-updated time { font-size: 12.5px; color: var(--ink-2); font-variant-numeric: tabular-nums; }
.empty { color: var(--ink-3); font-size: 12.5px; }
.ghost td { border-bottom-color: var(--line); }
.sk-key { width: 60px; } .sk-word { width: 70px; } .sk-time { width: 44px; margin-left: auto; }
.skeleton-body .skeleton { height: 10px; }
.state { display: grid; justify-items: center; gap: 8px; padding: 56px 24px 64px; text-align: center; }
.state h2 { font-size: 17px; }
.state p { max-width: 460px; font-size: 13.5px; }
.state .btn { margin-top: 10px; }
.state-icon { display: grid; place-items: center; width: 44px; height: 44px; margin-bottom: 6px; border-radius: 50%; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.state-icon.danger { background: var(--danger-bg); box-shadow: inset 0 0 0 1px var(--danger-line); color: var(--danger); }
.hint { display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 5px; padding: 16px 0 4px; font-size: 12px; color: var(--ink-3); }
.hint .keycap + .keycap { margin-left: 2px; }
.hint-link { display: inline-flex; align-items: center; gap: 5px; padding: 0; border: 0; background: transparent; color: var(--ink-3); font-size: 12px; }
.hint-link:hover { color: var(--teal-ink); }
.full-page { width: 100%; padding: 12px 28px 0; }
@container quotes (max-width: 900px) { th.c-customer { width: 26%; } th.c-updated { width: 0; } .c-updated .cell, thead .c-updated .th-sort { display: none; } th.c-status { width: 124px; } }
@container quotes (max-width: 700px) { th.c-customer { width: 0; } .c-customer .cell, thead .c-customer .th-sort { display: none; } th.c-total { width: 140px; } }
@media (max-width: 1100px) { .toolbar { margin: -6px -28px 0; } }
@media (max-width: 720px) {
  .toolbar { margin: -4px -12px 0; padding: 4px 12px 8px; gap: 8px; }
  .list-search { flex: 1 1 100%; width: auto; order: -1; }
  .list-search .field { height: 44px; font-size: 16px; }
  .count { display: none; }
  .new-btn { height: 40px; }
  .quotes, .quotes tbody { display: block; }
  .quotes thead { display: none; }
  .quote-row { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; grid-template-areas: "key status total" "title title title" "customer customer updated"; gap: 4px 10px; height: auto; padding: 10px 14px 11px; border-bottom: 1px solid var(--line); }
  .quote-row td { display: block !important; height: auto; padding: 0 !important; border: 0; background: none !important; box-shadow: none !important; }
  .quote-row.cursor, .quote-row.open { background: var(--row-selected); }
  .c-key { grid-area: key; } .c-status { grid-area: status; } .c-total { grid-area: total; } .c-title { grid-area: title; } .c-customer { grid-area: customer; } .c-updated { grid-area: updated; }
  .c-customer .cell, .c-updated .cell { display: flex !important; }
  .title-link { white-space: normal; font-size: 14.5px; line-height: 1.35; }
  .c-title .cell { white-space: normal; }
  .ghost { display: grid; }
  .full-page { padding: 8px 12px 0; }
  .hint { display: none; }
}
</style>
