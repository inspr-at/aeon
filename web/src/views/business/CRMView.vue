<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Parked (AEON-70, 2026-09-24): quotes and organisations will be ported from Markus's current classic Paimos quote builder; this file is not routed or linked. -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { ListItem } from '../../lib/api'
import { toast } from '../../lib/toast'
import { absoluteTime, highlight, plural, relativeTime } from '../../lib/work'
import { useBusiness } from '../../stores/business'
import AppIcon from '../../components/business/BizIcon.vue'
import BusinessPage from '../../components/business/BusinessPage.vue'
import OrgPanel from '../../components/business/OrgPanel.vue'

// Customers: organisations with their contacts, the projects they are the
// customer of and their quotes. An organisation opens in the docked panel.
type SortField = 'name' | 'contacts' | 'projects' | 'quotes' | 'updated'
const business = useBusiness()
const route = useRoute()
const router = useRouter()
const now = ref(Date.now())
const search = ref<HTMLInputElement>()
const grid = ref<HTMLTableElement>()
const panel = ref<InstanceType<typeof OrgPanel>>()
const createInput = ref<HTMLInputElement>()
const cursor = ref<string | null>(null)
const term = ref('')
const creating = ref(false)
const newName = ref('')
const busy = ref(false)
const linksLoaded = ref(false)

const orgKey = computed(() => typeof route.params.orgKey === 'string' ? route.params.orgKey : '')
const sort = computed<{ field: SortField; desc: boolean }>(() => {
  const raw = typeof route.query.sort === 'string' ? route.query.sort : 'name'
  const desc = raw.startsWith('-'), field = (desc ? raw.slice(1) : raw) as SortField
  return ['name', 'contacts', 'projects', 'quotes', 'updated'].includes(field) ? { field, desc } : { field: 'name', desc: false }
})
function sortBy(field: SortField) {
  const next = sort.value.field !== field ? (field === 'name' ? field : `-${field}`) : sort.value.desc ? field : `-${field}`
  void router.replace({ query: { ...route.query, sort: next === 'name' ? undefined : next } })
}
const quotesOf = (id: string) => business.quotes.filter(q => q.customer_org_node_id === id).length
const counts = (org: ListItem) => ({ contacts: business.links.get(org.id)?.contacts.length, projects: business.links.get(org.id)?.projects.length, quotes: quotesOf(org.id) })
const text = (value: unknown) => typeof value === 'string' ? value : ''
const site = (value: string) => value.replace(/^https?:\/\//, '').replace(/\/$/, '')
const rows = computed(() => {
  const needle = term.value.trim().toLowerCase()
  const { field, desc } = sort.value
  const value = (org: ListItem): number | string => field === 'name' ? org.title.toLowerCase() : field === 'updated' ? Date.parse(org.updated_at) : (counts(org)[field] ?? -1)
  return business.organisations
    .filter(org => !needle || `${org.key} ${org.title} ${text(org.fields.website)} ${text(org.fields.legal_name)}`.toLowerCase().includes(needle))
    .sort((a, b) => { const x = value(a), y = value(b); return (x < y ? -1 : x > y ? 1 : 0) * (desc ? -1 : 1) || a.title.localeCompare(b.title) })
})
const selected = computed(() => orgKey.value ? business.organisations.find(o => o.key.toLowerCase() === orgKey.value.toLowerCase()) ?? null : null)
const resolveError = computed(() => orgKey.value && business.crmLoaded && !selected.value ? `${orgKey.value.toUpperCase()} is not an organisation in this workspace, or it was deleted.` : '')
const position = computed(() => {
  const index = selected.value ? rows.value.findIndex(r => r.id === selected.value!.id) : -1
  return index === -1 ? null : { index, count: rows.value.length }
})
watch(selected, org => { if (org) cursor.value = org.id; document.title = org ? `${org.title} · PAIMOS AEON` : 'Organisations · PAIMOS AEON' })

// ---------- Panel ----------
let openedFromList = false
function openOrg(org: ListItem) {
  cursor.value = org.id
  const location = { path: `/business/organisations/${encodeURIComponent(org.key)}`, query: route.query }
  if (orgKey.value) { void router.replace(location); return }
  openedFromList = true
  void router.push(location)
}
function closePanel() {
  if (!orgKey.value) return
  const back = openedFromList && typeof window.history.state?.back === 'string'
  openedFromList = false
  if (back) router.back(); else void router.replace({ path: '/business/organisations', query: route.query })
  void nextTick(() => grid.value?.focus({ preventScroll: true }))
}
function removed() { void router.replace({ path: '/business/organisations', query: route.query }) }

// ---------- Create ----------
async function startCreate() {
  if (!business.staff) return
  creating.value = true
  await nextTick(); createInput.value?.focus()
}
async function create() {
  const name = newName.value.trim()
  if (!name || busy.value) return
  busy.value = true
  try {
    const node = await business.createCRMNode('organisation', name)
    business.upsertNode('organisations', node)
    business.setLinks(node.id, { contacts: [], projects: [], quotes: [] })
    newName.value = ''; creating.value = false
    toast(`Added ${node.title}. Add its contacts next.`)
    openOrg(node)
    setTimeout(() => panel.value?.focusAddContact(), 350)
  } catch (e) { toast(`The organisation was not added: ${e instanceof Error ? e.message : 'unknown error'}`, { tone: 'error' }) }
  finally { busy.value = false }
}
function createKeys(event: KeyboardEvent) {
  if (event.key === 'Enter') { event.preventDefault(); void create() }
  else if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); creating.value = false; newName.value = '' }
}

// ---------- Keyboard ----------
function typing(target: EventTarget | null) { return target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) }
function move(step: number) {
  const list = rows.value
  if (!list.length) return
  const from = selected.value?.id ?? cursor.value
  const index = list.findIndex(r => r.id === from)
  const next = list[index === -1 ? (step > 0 ? 0 : list.length - 1) : Math.max(0, Math.min(list.length - 1, index + step))]
  cursor.value = next.id
  if (orgKey.value) openOrg(next)
  void nextTick(() => document.getElementById(`org-${next.id}`)?.scrollIntoView({ block: 'nearest' }))
}
function keydown(event: KeyboardEvent) {
  if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return
  if (document.querySelector('dialog[open], .floating')) return
  const target = event.target as HTMLElement | null
  if (typing(target)) {
    if (event.key === 'ArrowDown' && target === search.value) { event.preventDefault(); search.value?.blur(); move(cursor.value ? 0 : 1); grid.value?.focus() }
    return
  }
  if (panel.value?.el?.contains(target) && !['j', 'k', 'Escape'].includes(event.key)) return
  const row = rows.value.find(r => r.id === cursor.value)
  switch (event.key) {
    case 'j': case 'ArrowDown': event.preventDefault(); move(1); break
    case 'k': case 'ArrowUp': event.preventDefault(); move(-1); break
    case 'Enter': case 'o': if (event.key === 'Enter' && target?.closest('button, a')) return; if (row) { event.preventDefault(); openOrg(row) } break
    case 'Escape': if (orgKey.value) { event.preventDefault(); closePanel() } break
    case '/': event.preventDefault(); search.value?.focus(); break
    case 'n': event.preventDefault(); void startCreate(); break
    case 'e': if (orgKey.value) { event.preventDefault(); panel.value?.editTitle() } break
    case 'c': if (orgKey.value) { event.preventDefault(); panel.value?.focusAddContact() } break
  }
}
let clock: ReturnType<typeof setInterval> | undefined
onMounted(async () => {
  window.addEventListener('keydown', keydown)
  clock = setInterval(() => { now.value = Date.now() }, 60_000)
  await business.loadPlugins()
  if (!business.open.crm) return
  void business.loadKinds(); void business.loadPrincipals()
  if (business.open.quotes) void business.loadQuotes()
  await business.loadCRM(true)
  await business.loadLinks(business.organisations.map(org => org.id))
  linksLoaded.value = true
})
onBeforeUnmount(() => { window.removeEventListener('keydown', keydown); clearInterval(clock) })
function rowClick(event: MouseEvent, org: ListItem) { if (!(event.target as HTMLElement).closest('button, a')) openOrg(org) }
function ariaSort(field: SortField) { return sort.value.field === field ? (sort.value.desc ? 'descending' : 'ascending') : 'none' }
const columns: { field: SortField; label: string; cls: string }[] = [
  { field: 'name', label: 'Organisation', cls: 'c-name' }, { field: 'contacts', label: 'Contacts', cls: 'c-num' },
  { field: 'projects', label: 'Projects', cls: 'c-num' }, { field: 'quotes', label: 'Quotes', cls: 'c-num' }, { field: 'updated', label: 'Updated', cls: 'c-updated' },
]
</script>

<template>
  <BusinessPage title="Organisations" area="crm" :panel-open="!!orgKey">
    <template #summary>
      <span v-if="business.crmLoaded">{{ business.organisations.length ? `${plural(business.organisations.length, 'organisation')} · ${plural(business.contacts.length, 'contact')}` : 'No organisations yet' }}</span>
      <span v-else class="skeleton summary-skeleton" />
    </template>

    <div class="toolbar" role="toolbar" aria-label="Organisations">
      <label class="search-field list-search">
        <AppIcon name="search" :size="14" />
        <input ref="search" v-model="term" class="field" type="search" placeholder="Search name or website" aria-label="Search organisations" autocomplete="off" spellcheck="false" />
        <kbd v-if="!term" class="keycap slash" aria-hidden="true">/</kbd>
      </label>
      <span class="spacer" />
      <span class="count mono">{{ business.crmLoaded ? plural(rows.length, 'organisation') : '' }}</span>
      <button v-if="business.staff" type="button" class="btn primary new-btn" aria-keyshortcuts="n" data-tip="New organisation · n" @click="startCreate"><AppIcon name="plus" :size="14" /><span class="new-label">New organisation</span></button>
    </div>

    <div class="table-card">
      <table ref="grid" class="orgs" role="grid" aria-label="Organisations" tabindex="0" :aria-activedescendant="cursor ? `org-${cursor}` : undefined" @focus="!cursor && rows[0] && (cursor = rows[0].id)">
        <thead>
          <tr>
            <th v-for="column in columns" :key="column.label" scope="col" :class="column.cls" :aria-sort="ariaSort(column.field)">
              <button type="button" class="th-sort" :class="{ on: sort.field === column.field }" @click="sortBy(column.field)"><span>{{ column.label }}</span><span class="sort-mark" aria-hidden="true"><AppIcon v-if="sort.field === column.field" :name="sort.desc ? 'arrow-down' : 'arrow-up'" :size="11" /></span></button>
            </th>
          </tr>
        </thead>
        <tbody v-if="creating" class="create-body">
          <tr class="create-row">
            <td :colspan="columns.length">
              <div class="create-cell" @keydown="createKeys">
                <AppIcon name="building" :size="14" class="create-icon" />
                <input ref="createInput" v-model="newName" class="field" placeholder="Organisation name" aria-label="New organisation name" maxlength="512" autocomplete="off" />
                <span class="create-keys" aria-hidden="true"><kbd class="keycap"><AppIcon name="enter" /></kbd> add · <kbd class="keycap">esc</kbd> cancel</span>
                <button type="button" class="btn sm on" :disabled="!newName.trim() || busy" @click="create">{{ busy ? 'Adding…' : 'Add' }}</button>
              </div>
            </td>
          </tr>
        </tbody>
        <tbody v-if="!business.crmLoaded" class="skeleton-body" aria-hidden="true">
          <tr v-for="i in 5" :key="i" class="org-row ghost"><td class="c-name"><div class="cell"><span class="skeleton" :style="{ width: `${30 + (i * 17) % 30}%` }" /></div></td><td v-for="j in 3" :key="j" class="c-num"><div class="cell"><span class="skeleton sk-num" /></div></td><td class="c-updated"><div class="cell"><span class="skeleton sk-num" /></div></td></tr>
        </tbody>
        <tbody v-else>
          <tr v-for="org in rows" :id="`org-${org.id}`" :key="org.id" class="org-row" role="row" :class="{ cursor: cursor === org.id, open: selected?.id === org.id }" :aria-selected="selected?.id === org.id" @click="rowClick($event, org)">
            <td class="c-name"><div class="cell">
              <span class="org-mark" aria-hidden="true"><AppIcon name="building" :size="13" /></span>
              <span class="name-text">
                <span class="org-name"><template v-for="(part, i) in highlight(org.title, term)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span>
                <span class="org-sub"><span class="mono">{{ org.key }}</span><template v-if="text(org.fields.website)"> · {{ site(text(org.fields.website)) }}</template></span>
              </span>
            </div></td>
            <td class="c-num"><div class="cell mono"><template v-if="counts(org).contacts !== undefined">{{ counts(org).contacts }}</template><span v-else-if="!linksLoaded" class="skeleton sk-num" /><span v-else class="faint">—</span></div></td>
            <td class="c-num"><div class="cell mono"><template v-if="counts(org).projects !== undefined">{{ counts(org).projects }}</template><span v-else-if="!linksLoaded" class="skeleton sk-num" /><span v-else class="faint">—</span></div></td>
            <td class="c-num"><div class="cell mono">{{ counts(org).quotes }}</div></td>
            <td class="c-updated"><div class="cell"><time :datetime="org.updated_at" :data-tip="absoluteTime(org.updated_at)">{{ relativeTime(org.updated_at, { now }) }}</time></div></td>
          </tr>
        </tbody>
      </table>
      <div v-if="business.crmLoaded && !rows.length && !creating" class="state">
        <span class="state-icon"><AppIcon :name="term ? 'filter' : 'building'" :size="18" /></span>
        <template v-if="term"><h2>No organisation matches “{{ term }}”</h2><p>Check the spelling or search by website.</p><button type="button" class="btn" @click="term = ''">Clear search</button></template>
        <template v-else><h2>No organisations yet</h2><p>Add your customers here, then their contacts. A quote is always for one organisation.</p><button v-if="business.staff" type="button" class="btn" @click="startCreate"><AppIcon name="plus" :size="14" />New organisation</button></template>
      </div>
    </div>
    <p v-if="rows.length" class="hint"><kbd class="keycap">j</kbd><kbd class="keycap">k</kbd> move · <kbd class="keycap"><AppIcon name="enter" /></kbd> open · <kbd class="keycap">n</kbd> new organisation · <kbd class="keycap">c</kbd> add contact</p>
  </BusinessPage>
  <OrgPanel v-if="orgKey" ref="panel" :org="selected" :org-key="orgKey.toUpperCase()" :position="position" :now="now" :resolve-error="resolveError" @close="closePanel" @prev="move(-1)" @next="move(1)" @removed="removed" />
</template>

<style scoped>
.summary-skeleton { display: inline-block; width: 220px; }
.toolbar { display: flex; align-items: center; gap: 10px; min-height: 52px; padding: 0 0 10px; }
.list-search { width: 300px; }
.list-search .field { height: 32px; padding-right: 30px; font-size: 13.5px; }
.list-search .field::-webkit-search-cancel-button { display: none; }
.slash { position: absolute; right: 8px; pointer-events: none; }
@media (hover: none) { .slash { display: none; } }
.spacer { flex: 1; }
.count { font-size: 12px; color: var(--ink-2); white-space: nowrap; }
.new-btn { height: 32px; padding: 0 14px 0 11px; gap: 6px; }
.table-card { border-radius: var(--radius); border: 1px solid var(--glass-edge); overflow: clip; background: linear-gradient(165deg, var(--surface-raised-2), var(--glass) 60%); box-shadow: var(--shadow); container: orgs / inline-size; }
.orgs { width: 100%; border-collapse: separate; border-spacing: 0; table-layout: fixed; font-size: 13.5px; }
.orgs:focus-visible { box-shadow: none; }
th.c-num { width: 104px; } th.c-updated { width: 110px; }
thead th { height: 34px; padding: 0 12px; text-align: left; font-weight: 500; border-bottom: 1px solid var(--line-2); background: var(--surface-raised-2); }
thead th:first-child { padding-left: 18px; }
thead .c-num, thead .c-updated { text-align: right; }
thead .c-num .th-sort, thead .c-updated .th-sort { flex-direction: row-reverse; }
.th-sort { display: inline-flex; align-items: center; gap: 6px; height: 26px; margin: 0 -6px; padding: 0 6px; border: 0; border-radius: 6px; background: transparent; font: 500 10.5px/1 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.th-sort:hover { color: var(--ink); background: var(--row-hover); }
.th-sort.on { color: var(--teal-ink); }
.th-sort:focus-visible { box-shadow: var(--focus-ring); }
.sort-mark { display: inline-flex; min-width: 11px; }
.org-row { cursor: default; }
.org-row td { height: 54px; padding: 0 12px; border-bottom: 1px solid var(--line); vertical-align: middle; }
.org-row td:first-child { padding-left: 16px; }
tbody .org-row:last-child td { border-bottom: 0; }
.cell { display: flex; align-items: center; gap: 12px; min-width: 0; white-space: nowrap; }
.c-num .cell, .c-updated .cell { justify-content: flex-end; }
.c-num .cell { font-size: 13px; }
@media (hover: hover) { .org-row:hover td { background: var(--row-hover); } }
.org-row.cursor td, .org-row.open td { background: var(--row-selected); }
.org-row.cursor td:first-child, .org-row.open td:first-child { box-shadow: inset 3px 0 0 var(--row-accent); }
.org-mark { display: grid; place-items: center; flex-shrink: 0; width: 30px; height: 30px; border-radius: 9px; background: var(--code-bg); color: var(--ink-2); }
.open .org-mark, .cursor .org-mark { background: var(--chip-teal-bg); color: var(--teal-ink); }
.name-text { display: grid; min-width: 0; }
.org-name { font-weight: 650; overflow: hidden; text-overflow: ellipsis; }
.org-sub { font-size: 12px; color: var(--ink-3); overflow: hidden; text-overflow: ellipsis; }
.org-sub .mono { font-size: 11px; }
.c-updated time { font-size: 12.5px; color: var(--ink-2); }
.faint { color: var(--ink-3); }
.sk-num { width: 28px; height: 9px; }
.ghost td { border-bottom-color: var(--line); }
.create-row td { padding: 8px 10px 8px 16px; border-bottom: 1px solid var(--line); background: var(--row-selected); }
.create-cell { display: flex; align-items: center; gap: 10px; }
.create-cell .field { flex: 1; height: 32px; }
.create-icon { color: var(--teal); }
.create-keys { display: inline-flex; align-items: center; gap: 4px; font-size: 11.5px; color: var(--ink-3); white-space: nowrap; }
.state { display: grid; justify-items: center; gap: 8px; padding: 56px 24px 64px; text-align: center; }
.state h2 { font-size: 17px; }
.state p { max-width: 440px; font-size: 13.5px; }
.state .btn { margin-top: 10px; }
.state-icon { display: grid; place-items: center; width: 44px; height: 44px; margin-bottom: 6px; border-radius: 50%; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.hint { display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 5px; padding: 16px 0 4px; font-size: 12px; color: var(--ink-3); }
.hint .keycap + .keycap { margin-left: 2px; }
@container orgs (max-width: 720px) { th.c-num { width: 84px; } th.c-updated { width: 0; } .c-updated .cell, thead .c-updated .th-sort { display: none; } }
@container orgs (max-width: 560px) { th.c-num:nth-child(3) { width: 0; } td.c-num:nth-child(3) .cell, th.c-num:nth-child(3) .th-sort { display: none; } }
@media (max-width: 720px) {
  .toolbar { flex-wrap: wrap; }
  .list-search { flex: 1 1 100%; width: auto; }
  .list-search .field { height: 44px; font-size: 16px; }
  .count { display: none; }
  .new-btn { height: 40px; margin-left: auto; }
  .hint, .create-keys { display: none; }
  th.c-num { width: 64px; }
}
</style>
