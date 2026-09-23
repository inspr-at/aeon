<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { ListItem } from '../../lib/api'
import type { GroupBy, RowGroup, EpicRef } from '../../lib/ticketList'
import { absoluteTime, highlight, kindLabel, plural, priorityLabel, relativeTime, statusMeta, type SortField, type SortKey } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import PersonAvatar from './PersonAvatar.vue'
import PriorityIcon from './PriorityIcon.vue'
import StatusIcon from './StatusIcon.vue'

const props = defineProps<{
  groups: RowGroup[]
  group: GroupBy
  rowsById: Map<string, ListItem>
  cursorId: string | null
  openId: string | null
  query: string
  sort: SortKey[]
  density: 'comfortable' | 'compact'
  loading: boolean
  loadingMore: boolean
  error: string
  moreError: string
  hasMore: boolean
  filtered: boolean
  hidingClosed: boolean
  collapsed: Set<string>
  total: number | null
  projectKey: string
  scrollRoot: HTMLElement | null
  now: number
}>()
const emit = defineEmits<{
  open: [row: ListItem]
  cursor: [id: string]
  sort: [field: SortField, additive: boolean]
  status: [row: ListItem, anchor: HTMLElement]
  copy: [row: ListItem]
  newTab: [row: ListItem]
  toggleGroup: [key: string]
  openEpic: [epic: EpicRef]
  retry: []
  more: []
  clearFilters: []
  showClosed: []
}>()

const columns: { field: SortField | null; label: string; cls: string }[] = [
  { field: 'key', label: 'Key', cls: 'c-key' },
  { field: 'title', label: 'Title', cls: 'c-title' },
  { field: 'state', label: 'Status', cls: 'c-status' },
  { field: 'priority', label: 'Priority', cls: 'c-prio' },
  { field: null, label: 'Assignee', cls: 'c-assignee' },
  { field: 'updated_at', label: 'Updated', cls: 'c-updated' },
]
const grid = ref<HTMLTableElement>()
const sentinel = ref<HTMLElement>()
let observer: IntersectionObserver | undefined

function sortOf(field: SortField | null) {
  if (!field) return null
  const index = props.sort.findIndex(key => key.field === field)
  return index === -1 ? null : { ...props.sort[index], index }
}
function ariaSort(field: SortField | null) {
  const current = sortOf(field)
  return current ? (current.desc ? 'descending' : 'ascending') : field ? 'none' : undefined
}
function href(row: { key: string }) { return `/p/${encodeURIComponent(props.projectKey)}/${encodeURIComponent(row.key)}` }
function epicChip(row: ListItem) {
  // Direct parents only: an epic, or the ticket a task belongs to. The project itself is implied.
  if (props.group === 'epic' || !row.parent || row.parent.kind_slug === 'project') return null
  return row.parent
}
function statusClick(event: MouseEvent, row: ListItem) { emit('status', row, event.currentTarget as HTMLElement) }
function rowClick(event: MouseEvent, row: ListItem) {
  if ((event.target as HTMLElement).closest('button')) return
  if (event.metaKey || event.ctrlKey) { emit('newTab', row); return }
  emit('cursor', row.id)
  emit('open', row)
}
function linkClick(event: MouseEvent) {
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.button === 1) return // let the browser open a tab
  event.preventDefault()
}
function groupEpicRow(group: RowGroup) { return group.epic ? props.rowsById.get(group.epic.id) : undefined }
function focusGrid() { grid.value?.focus({ preventScroll: true }) }
function scrollToRow(id: string) {
  document.getElementById(`row-${id}`)?.scrollIntoView({ block: 'nearest' })
}

function observe() {
  observer?.disconnect()
  if (!sentinel.value) return
  observer = new IntersectionObserver(entries => {
    if (entries.some(entry => entry.isIntersecting) && props.hasMore && !props.loadingMore && !props.moreError) emit('more')
  }, { root: props.scrollRoot, rootMargin: '0px 0px 800px 0px' })
  observer.observe(sentinel.value)
}
onMounted(observe)
watch(() => [props.scrollRoot, props.hasMore, props.loadingMore], observe)
onBeforeUnmount(() => observer?.disconnect())
defineExpose({ focusGrid, scrollToRow, el: grid })
</script>

<template>
  <div class="table-card" :class="density">
    <table ref="grid" class="tickets" role="grid" aria-label="Tickets" :aria-busy="loading" tabindex="0" :aria-activedescendant="cursorId ? `row-${cursorId}` : undefined">
      <colgroup><col class="w-key" /><col /><col class="w-status" /><col class="w-prio" /><col class="w-assignee" /><col class="w-updated" /></colgroup>
      <thead>
        <tr>
          <th v-for="column in columns" :key="column.label" scope="col" :class="column.cls" :aria-sort="ariaSort(column.field)">
            <button v-if="column.field" type="button" class="th-sort" :class="{ on: sortOf(column.field) }" :data-tip="'Sort by ' + column.label.toLowerCase() + '\nShift-click adds a secondary sort'" @click="event => emit('sort', column.field!, event.shiftKey)">
              <span>{{ column.label }}</span>
              <span class="sort-mark" aria-hidden="true">
                <template v-if="sortOf(column.field)">
                  <AppIcon :name="sortOf(column.field)!.desc ? 'arrow-down' : 'arrow-up'" :size="11" />
                  <span v-if="sort.length > 1" class="sort-index">{{ sortOf(column.field)!.index + 1 }}</span>
                </template>
                <AppIcon v-else-if="!sort.length && column.field === 'updated_at'" name="arrow-down" :size="11" class="default-sort" />
              </span>
            </button>
            <span v-else class="th-label">{{ column.label }}</span>
          </th>
        </tr>
      </thead>

      <tbody v-if="loading && !groups.some(g => g.rows.length)" class="skeleton-body" aria-hidden="true">
        <tr v-for="index in 14" :key="index" class="ticket-row ghost">
          <td class="c-key"><span class="skeleton sk-key" /></td>
          <td class="c-title"><span class="skeleton sk-title" :style="{ width: `${38 + ((index * 37) % 45)}%` }" /></td>
          <td class="c-status"><span class="sk-status"><span class="sk-dot" /><span class="skeleton sk-word" /></span></td>
          <td class="c-prio"><span class="skeleton sk-word" /></td>
          <td class="c-assignee"><span class="skeleton sk-word" /></td>
          <td class="c-updated"><span class="skeleton sk-time" /></td>
        </tr>
      </tbody>

      <template v-else>
        <tbody v-for="entry in groups" :key="entry.key" :class="{ dim: loading }">
          <tr v-if="group !== 'none'" class="group-row" :class="{ collapsed: collapsed.has(entry.key) }">
            <th colspan="6" scope="rowgroup">
              <div class="group-head">
                <button type="button" class="group-toggle" :aria-expanded="!collapsed.has(entry.key)" :aria-label="`${collapsed.has(entry.key) ? 'Expand' : 'Collapse'} ${entry.epic ? entry.epic.key : entry.label}`" @click="emit('toggleGroup', entry.key)">
                  <AppIcon name="chevron" :size="14" />
                </button>
                <template v-if="group === 'status'">
                  <StatusIcon :state="entry.state ?? ''" />
                  <span class="group-label">{{ entry.label }}</span>
                  <span class="group-count mono">{{ entry.total }}</span>
                </template>
                <template v-else-if="entry.epic">
                  <AppIcon name="epic" :size="14" class="epic-glyph" />
                  <button v-if="groupEpicRow(entry)" :id="`row-${entry.epic.id}`" type="button" class="group-epic" :class="{ cursor: cursorId === entry.epic.id, open: openId === entry.epic.id }" @click="emit('cursor', entry.epic.id); emit('open', groupEpicRow(entry)!)">
                    <span class="key">{{ entry.epic.key }}</span>
                    <span class="group-label">{{ entry.epic.title }}</span>
                  </button>
                  <button v-else type="button" class="group-epic" @click="emit('openEpic', entry.epic)">
                    <span class="key">{{ entry.epic.key }}</span>
                    <span class="group-label">{{ entry.epic.title }}</span>
                  </button>
                  <StatusIcon v-if="groupEpicRow(entry)" :state="groupEpicRow(entry)!.state" :size="12" class="group-epic-status" />
                  <span class="group-count mono">{{ entry.total }}</span>
                </template>
                <template v-else>
                  <span class="group-label muted">{{ entry.label }}</span>
                  <span class="group-count mono">{{ entry.total }}</span>
                </template>
              </div>
            </th>
          </tr>
          <template v-if="!collapsed.has(entry.key)">
            <tr
              v-for="row in entry.rows" :id="`row-${row.id}`" :key="row.id" class="ticket-row"
              :class="{ cursor: cursorId === row.id, open: openId === row.id, epic: row.kind_slug === 'epic' }"
              :aria-selected="cursorId === row.id" @click="rowClick($event, row)"
            >
              <td class="c-key"><span class="key">{{ row.key }}</span></td>
              <td class="c-title">
                <div class="title-cell">
                  <AppIcon :name="row.kind_slug === 'epic' ? 'epic' : row.kind_slug === 'task' ? 'task' : 'ticket'" :size="14" class="kind-glyph" :class="row.kind_slug" :data-tip="kindLabel(row.kind_slug)" />
                  <a class="title-link" :href="href(row)" tabindex="-1" @click="linkClick">
                    <template v-for="(part, i) in highlight(row.title, query)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template>
                  </a>
                  <span v-if="row.kind_slug === 'epic' && row.children_count" class="child-count mono" :data-tip="plural(row.children_count, 'child item')">{{ row.children_count }}</span>
                  <span v-if="epicChip(row)" class="parent-chip" :class="{ epic: epicChip(row)!.kind_slug === 'epic' }" :data-tip="`${kindLabel(epicChip(row)!.kind_slug)} ${epicChip(row)!.key}\n${epicChip(row)!.title}`">
                    <AppIcon v-if="epicChip(row)!.kind_slug === 'epic'" name="epic" :size="10" />
                    <span v-else class="mono">{{ epicChip(row)!.key }}</span>
                    <span v-if="epicChip(row)!.kind_slug === 'epic'" class="parent-title">{{ epicChip(row)!.title }}</span>
                  </span>
                  <span class="row-actions">
                    <button type="button" class="icon-btn sm flat" :aria-label="`Copy ${row.key}`" :data-tip="`Copy ${row.key}`" @click.stop="emit('copy', row)"><AppIcon name="copy" :size="13" /></button>
                    <button type="button" class="icon-btn sm flat" :aria-label="`Open ${row.key} in a new tab`" data-tip="Open in new tab" @click.stop="emit('newTab', row)"><AppIcon name="external" :size="13" /></button>
                  </span>
                </div>
              </td>
              <td class="c-status">
                <button type="button" class="status-btn" :aria-label="`Status: ${statusMeta(row.state).label}. Change status of ${row.key}`" aria-haspopup="menu" @click.stop="statusClick($event, row)">
                  <StatusIcon :state="row.state" />
                  <span>{{ statusMeta(row.state).label }}</span>
                </button>
              </td>
              <td class="c-prio">
                <span v-if="row.priority && row.priority !== 'none'" class="prio-cell"><PriorityIcon :priority="row.priority" /><span>{{ priorityLabel(row.priority) }}</span></span>
                <span v-else class="empty" aria-label="No priority">—</span>
              </td>
              <td class="c-assignee">
                <span v-if="row.assignee" class="person"><PersonAvatar :name="row.assignee.name" :size="20" /><span class="person-name">{{ row.assignee.name }}</span></span>
                <span v-else class="empty" aria-label="Unassigned">—</span>
              </td>
              <td class="c-updated"><time :datetime="row.updated_at" :data-tip="absoluteTime(row.updated_at)">{{ relativeTime(row.updated_at, { now }) }}</time></td>
            </tr>
          </template>
        </tbody>
      </template>
    </table>

    <div v-if="error" class="state" role="alert">
      <span class="state-icon danger"><AppIcon name="alert" :size="18" /></span>
      <h2>Tickets could not be loaded</h2>
      <p>{{ error }}</p>
      <button type="button" class="btn" @click="emit('retry')"><AppIcon name="refresh" :size="14" />Try again</button>
    </div>
    <div v-else-if="!loading && !groups.some(g => g.rows.length || g.epic)" class="state">
      <span class="state-icon"><AppIcon :name="filtered ? 'filter' : 'inbox'" :size="18" /></span>
      <template v-if="filtered">
        <h2>No tickets match these filters</h2>
        <p>{{ query ? `Nothing with “${query}” in its key or title` : 'Try fewer filters' }}{{ hidingClosed ? ', or include closed tickets.' : '.' }}</p>
        <div class="state-actions">
          <button type="button" class="btn" @click="emit('clearFilters')">Clear filters</button>
          <button v-if="hidingClosed" type="button" class="btn ghost" @click="emit('showClosed')">Show closed</button>
        </div>
      </template>
      <template v-else-if="hidingClosed">
        <h2>Nothing open here</h2>
        <p>Every ticket in this project is closed.</p>
        <button type="button" class="btn" @click="emit('showClosed')">Show closed tickets</button>
      </template>
      <template v-else>
        <h2>No tickets yet</h2>
        <p>Tickets, epics and tasks of this project appear here.</p>
      </template>
    </div>

    <div ref="sentinel" class="sentinel" aria-hidden="true" />
    <div v-if="loadingMore" class="list-foot" role="status"><span class="spinner" aria-hidden="true" />Loading more tickets…</div>
    <div v-else-if="moreError" class="list-foot error" role="alert">More tickets could not be loaded. <button type="button" class="btn sm" @click="emit('more')">Retry</button></div>
    <div v-else-if="!loading && !error && total && !hasMore && groups.some(g => g.rows.length)" class="list-foot end">{{ plural(total, 'ticket') }}</div>
  </div>
</template>

<style scoped>
.table-card {
  --row-h: 36px;
  position: relative; border-radius: var(--radius); border: 1px solid var(--glass-edge); overflow: clip;
  background: linear-gradient(165deg, var(--surface-raised-2), var(--glass) 60%); box-shadow: var(--shadow);
}
.table-card.compact { --row-h: 30px; }
.tickets { width: 100%; border-collapse: separate; border-spacing: 0; table-layout: fixed; font-size: 13.5px; }
.tickets:focus-visible { box-shadow: none; }
.w-key { width: 118px; } .w-status { width: 138px; } .w-prio { width: 110px; } .w-assignee { width: 156px; } .w-updated { width: 104px; }
thead th {
  position: sticky; top: var(--toolbar-h, 0px); z-index: 2; height: 34px; padding: 0 12px; text-align: left; font-weight: 500;
  background: var(--surface-raised-2); border-bottom: 1px solid var(--line-2);
  backdrop-filter: blur(14px) saturate(1.15); -webkit-backdrop-filter: blur(14px) saturate(1.15);
}
thead th:first-child { padding-left: 18px; }
.th-sort, .th-label { display: inline-flex; align-items: center; gap: 6px; height: 26px; margin: 0 -6px; padding: 0 6px; border: 0; border-radius: 6px; background: transparent; font: 500 10.5px/1 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.th-sort:hover { color: var(--ink); background: var(--row-hover); }
.th-sort.on { color: var(--teal-ink); }
.sort-mark { display: inline-flex; align-items: center; gap: 1px; min-width: 11px; }
.sort-index { font-size: 9px; letter-spacing: 0; }
.default-sort { opacity: .55; }
.c-updated { text-align: right; }
thead .c-updated .th-sort { flex-direction: row-reverse; }

.ticket-row { height: var(--row-h); cursor: default; }
.ticket-row td { height: var(--row-h); padding: 0 12px; border-bottom: 1px solid var(--line); vertical-align: middle; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.ticket-row td:first-child { padding-left: 18px; }
.ticket-row:hover td { background: var(--row-hover); }
.ticket-row.cursor td, .ticket-row.open td { background: var(--row-selected); }
.ticket-row.cursor td:first-child, .ticket-row.open td:first-child { box-shadow: inset 3px 0 0 var(--row-accent); }
.tickets:focus-visible .ticket-row.cursor td { background: var(--row-selected); }
.tickets:focus-visible .ticket-row.cursor td:first-child { box-shadow: inset 3px 0 0 var(--row-accent), inset 0 0 0 0 transparent; }
.ticket-row { scroll-margin-top: calc(var(--toolbar-h, 0px) + 40px); scroll-margin-bottom: 24px; }
tbody.dim { opacity: .55; }
tbody:last-of-type .ticket-row:last-child td { border-bottom: 0; }

.key { font: 500 11.5px/1 var(--mono); color: var(--ink-2); letter-spacing: .01em; font-variant-ligatures: none; }
.ticket-row.open .key, .ticket-row.cursor .key { color: var(--teal-ink); }
.title-cell { display: flex; align-items: center; gap: 8px; min-width: 0; }
.kind-glyph { color: var(--ink-3); }
.kind-glyph.epic { color: var(--gold); }
.title-link { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; color: var(--ink); text-decoration: none; }
.ticket-row.epic .title-link { font-weight: 650; }
.title-link:focus-visible { box-shadow: none; }
.child-count { flex-shrink: 0; height: 17px; padding: 0 6px; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); font-size: 10.5px; line-height: 17px; color: var(--ink-2); }
.parent-chip { display: inline-flex; align-items: center; gap: 5px; flex: 0 1 auto; min-width: 0; max-width: 42%; height: 20px; padding: 0 8px; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); color: var(--ink-2); font-size: 11.5px; }
.parent-chip.epic svg { color: var(--gold); }
.parent-chip .mono { font-size: 10.5px; }
.parent-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row-actions { display: inline-flex; gap: 2px; margin-left: auto; flex-shrink: 0; visibility: hidden; }
.ticket-row:hover .row-actions, .ticket-row.cursor .row-actions { visibility: visible; }
.row-actions .icon-btn { width: 24px; height: 24px; color: var(--ink-3); }
.row-actions .icon-btn:hover { color: var(--teal-ink); }
.compact .row-actions .icon-btn { width: 22px; height: 22px; }

.status-btn { display: inline-flex; align-items: center; gap: 8px; max-width: 100%; height: 26px; margin-left: -8px; padding: 0 8px; border: 0; border-radius: 999px; background: transparent; color: var(--ink); font-size: 13px; white-space: nowrap; }
.status-btn span { overflow: hidden; text-overflow: ellipsis; }
.status-btn:hover, .status-btn[aria-expanded="true"] { background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); }
.compact .status-btn { height: 22px; }
.prio-cell, .person { display: inline-flex; align-items: center; gap: 8px; max-width: 100%; color: var(--ink-2); font-size: 13px; }
.person-name { overflow: hidden; text-overflow: ellipsis; color: var(--ink); }
.empty { color: var(--ink-3); }
.c-updated time { color: var(--ink-2); font-size: 12.5px; font-variant-numeric: tabular-nums; }

.group-row th { position: sticky; top: calc(var(--toolbar-h, 0px) + 35px); z-index: 1; height: 36px; padding: 0 12px 0 8px; text-align: left; font-weight: 400; background: var(--surface-raised-2); border-bottom: 1px solid var(--line); backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px); }
tbody + tbody .group-row th { border-top: 1px solid var(--line); }
.group-head { display: flex; align-items: center; gap: 8px; min-width: 0; }
.group-toggle { display: grid; place-items: center; width: 24px; height: 24px; padding: 0; border: 0; border-radius: 6px; background: transparent; color: var(--ink-3); }
.group-toggle:hover { background: var(--row-hover); color: var(--ink); }
.group-row.collapsed .group-toggle svg { transform: rotate(-90deg); }
.group-label { font-size: 13px; font-weight: 650; color: var(--ink); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.group-label.muted { color: var(--ink-2); }
.group-count { font-size: 11px; color: var(--ink-3); }
.epic-glyph { color: var(--gold); }
.group-epic { display: inline-flex; align-items: center; gap: 10px; min-width: 0; height: 26px; padding: 0 8px; margin-left: -4px; border: 0; border-radius: 6px; background: transparent; }
.group-epic:hover { background: var(--row-hover); }
.group-epic.cursor, .group-epic.open { background: var(--row-selected); box-shadow: inset 3px 0 0 var(--row-accent); }
.group-epic .key { color: var(--ink-2); }

.ghost td { border-bottom-color: var(--line); }
.sk-key { width: 70px; }
.sk-title { height: 10px; }
.sk-status { display: inline-flex; align-items: center; gap: 8px; }
.sk-dot { width: 12px; height: 12px; border-radius: 50%; box-shadow: inset 0 0 0 1.6px var(--skeleton); }
.sk-word { width: 58px; }
.sk-time { width: 44px; margin-left: auto; }

.state { display: grid; justify-items: center; gap: 8px; padding: 56px 24px 64px; text-align: center; }
.state-icon { display: grid; place-items: center; width: 44px; height: 44px; margin-bottom: 6px; border-radius: 50%; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.state-icon.danger { background: var(--danger-bg); box-shadow: inset 0 0 0 1px var(--danger-line); color: var(--danger); }
.state h2 { font-size: 17px; color: var(--ink); }
.state p { max-width: 420px; font-size: 13.5px; color: var(--ink-2); }
.state .btn, .state-actions { margin-top: 10px; }
.state-actions { display: flex; gap: 8px; }
.state-actions .btn { margin: 0; }
.sentinel { height: 1px; }
.list-foot { display: flex; align-items: center; justify-content: center; gap: 10px; min-height: 44px; font-size: 12.5px; color: var(--ink-2); border-top: 1px solid var(--line); }
.list-foot.end { font: 500 10.5px/1 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.list-foot.error { color: var(--danger); }
.spinner { width: 14px; height: 14px; border-radius: 50%; border: 1.8px solid var(--line-2); border-top-color: var(--teal); }
@media (prefers-reduced-motion: no-preference) { .spinner { animation: spin .8s linear infinite; } @keyframes spin { to { transform: rotate(360deg); } } }

@media (max-width: 1100px) { .w-assignee { width: 120px; } .w-prio { width: 96px; } }
@media (max-width: 720px) {
  .table-card { border-radius: 14px; }
  .tickets, .tickets tbody { display: block; }
  .tickets colgroup, .tickets thead { display: none; }
  .ticket-row {
    display: grid; grid-template-columns: auto auto minmax(0, 1fr) auto; grid-template-areas: "key status prio updated" "title title title title";
    align-items: center; gap: 5px 10px; height: auto; padding: 10px 14px 11px; border-bottom: 1px solid var(--line);
  }
  .ticket-row td { display: block; height: auto; padding: 0; border: 0; background: none !important; box-shadow: none !important; }
  .ticket-row td:first-child { padding-left: 0; }
  .ticket-row.cursor, .ticket-row.open { background: var(--row-selected); box-shadow: inset 3px 0 0 var(--row-accent); }
  .c-key { grid-area: key; } .c-status { grid-area: status; } .c-prio { grid-area: prio; } .c-updated { grid-area: updated; }
  .c-title { grid-area: title; white-space: normal; }
  .c-assignee { display: none !important; }
  .title-cell { align-items: flex-start; flex-wrap: wrap; gap: 4px 8px; }
  .title-cell .kind-glyph { margin-top: 2px; }
  .title-link { flex: 1 1 calc(100% - 30px); white-space: normal; font-size: 14.5px; line-height: 1.35; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
  .parent-chip { max-width: 100%; margin-left: 22px; }
  .row-actions { display: none; }
  .status-btn { height: 24px; margin-left: 0; padding: 0 8px 0 6px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); font-size: 12px; }
  .prio-cell { font-size: 12px; }
  .prio-cell span { display: none; }
  .ghost { display: grid; }
  .group-row, .group-row th { display: block; }
  .group-row th { top: var(--toolbar-h, 0px); padding: 0 10px; }
  .group-head { height: 40px; }
}
</style>
