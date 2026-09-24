<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import type { SessionControl } from '../../lib/agents'
import { GROUPS, controlBlocked, elapsed, type SessionGroup } from '../../lib/agentState'
import { relativeTime } from '../../lib/work'
import type { Availability, SessionView } from '../../stores/agents'
import AppIcon from '../AppIcon.vue'
import FloatingPanel from '../work/FloatingPanel.vue'
import ConnectHint from './ConnectHint.vue'
import LiveDot from './LiveDot.vue'

// Live sessions as rows, grouped by what they need: Markus, work, nothing, or history.
const props = defineProps<{
  groups: Record<SessionGroup, SessionView[]>; now: number; cursor: string; selected: string; state: Availability; error: string
  loaded: boolean; controls: Record<string, SessionControl>; canControl: boolean
}>()
const emit = defineEmits<{ open: [id: string]; control: [view: SessionView, kind: SessionControl['kind']]; focusRow: [id: string]; retry: [] }>()
const showStopped = ref(false)
const total = computed(() => GROUPS.reduce((sum, g) => sum + props.groups[g.id].length, 0))
const live = computed(() => total.value - props.groups.stopped.length)
const visible = (group: SessionGroup) => group === 'stopped' && !showStopped.value ? [] : props.groups[group].slice(0, group === 'stopped' ? 30 : undefined)

const controlBlock = (view: SessionView, kind: SessionControl['kind']) => controlBlocked(view.session, kind, view.name, props.canControl, props.controls[view.session.id])
// Phones get one overflow button per row; the menu offers the same two controls.
const menu = ref<{ view: SessionView; anchor: HTMLElement } | null>(null)
function openMenu(view: SessionView, event: MouseEvent) { menu.value = menu.value?.view.session.id === view.session.id ? null : { view, anchor: event.currentTarget as HTMLElement } }
function pick(kind: SessionControl['kind']) {
  const view = menu.value?.view
  menu.value = null
  if (view && !controlBlock(view, kind)) emit('control', view, kind)
}
function pendingLabel(view: SessionView) {
  const c = props.controls[view.session.id]
  if (!c || c.state === 'completed') return ''
  return c.kind === 'stop' ? (c.state === 'claimed' ? 'Stopping…' : 'Stop sent') : (c.state === 'claimed' ? 'Interrupting…' : 'Interrupt sent')
}
function rowClick(event: MouseEvent, id: string) {
  if ((event.target as HTMLElement).closest('a, button')) return
  emit('open', id)
}
</script>

<template>
  <section class="sessions glass-card" aria-labelledby="sessions-title">
    <header class="card-head">
      <h2 id="sessions-title">Sessions</h2>
      <span v-if="state === 'ready' && loaded && total" class="sub">{{ live }} live{{ groups.stopped.length ? ` · ${groups.stopped.length} stopped` : '' }}</span>
    </header>

    <div v-if="state === 'forbidden'" class="state">
      <AppIcon name="agent" :size="20" />
      <h3>Sessions are visible to workspace members with agent access</h3>
      <p>Ask a workspace admin to give your account access to agent sessions.</p>
    </div>
    <div v-else-if="state === 'error'" class="state" role="alert">
      <AppIcon name="alert" :size="20" />
      <h3>Sessions could not be loaded</h3>
      <p>{{ error }}</p>
      <button type="button" class="btn" @click="emit('retry')"><AppIcon name="refresh" :size="14" />Try again</button>
    </div>
    <div v-else-if="!loaded" class="skeleton-rows" role="status" aria-label="Loading sessions">
      <div v-for="i in 5" :key="i" class="sk-row"><span class="skeleton dot" /><span class="skeleton" :style="{ width: `${18 + (i * 7) % 16}%` }" /><span class="skeleton key" /><span class="skeleton" style="width: 12%" /></div>
    </div>
    <ConnectHint v-else-if="!total" />

    <div v-else class="table" role="table" aria-label="Agent sessions">
      <div class="thead" role="row">
        <span role="columnheader">State</span><span role="columnheader">Agent</span><span role="columnheader">Ticket</span>
        <span role="columnheader" class="c-account">Account</span><span role="columnheader" class="c-model">Model</span>
        <span role="columnheader" class="right">Heartbeat</span><span role="columnheader" class="right c-elapsed">Running</span><span role="columnheader"><span class="sr-only">Actions</span></span>
      </div>
      <template v-for="group in GROUPS" :key="group.id">
        <div v-if="groups[group.id].length" class="group-row" :class="group.id" role="row">
          <span role="rowheader" class="group-label">
            <button v-if="group.id === 'stopped'" type="button" class="group-toggle" :aria-expanded="showStopped" @click="showStopped = !showStopped">
              <AppIcon name="chevron-right" :size="12" class="chev" :class="{ turned: showStopped }" />{{ group.label }}<span class="mono">{{ groups[group.id].length }}</span>
            </button>
            <template v-else>{{ group.label }}<span class="mono">{{ groups[group.id].length }}</span></template>
          </span>
        </div>
        <div
          v-for="view in visible(group.id)" :key="view.session.id" class="row" role="row" :data-row="`s:${view.session.id}`" tabindex="-1"
          :class="[view.status.group, { active: cursor === `s:${view.session.id}`, selected: selected === view.session.id }]" @click="rowClick($event, view.session.id)"
        >
          <span role="cell" class="c-state"><LiveDot :tone="view.status.tone" /><span class="state-label">{{ pendingLabel(view) || view.status.label }}</span></span>
          <span role="cell" class="c-agent">
            <RouterLink class="agent-link" :to="`/agents/${view.session.id}`" :aria-label="`${view.harness} ${view.name}, ${view.status.label}`">
              <span class="harness" :class="view.session.harness">{{ view.harness }}</span><span class="agent-name">{{ view.name }}</span>
            </RouterLink>
            <span v-if="view.session.role === 'coordinator'" class="role" data-tip="Coordinates other sessions">Lead</span>
          </span>
          <span role="cell" class="c-ticket">
            <RouterLink v-if="view.ticket" class="ticket-chip" :to="view.ticket.href" :data-tip="view.ticket.title">{{ view.ticket.key }}</RouterLink>
            <span v-else class="faint">{{ view.projectKey || '—' }}</span>
          </span>
          <span role="cell" class="c-account">{{ view.account || '—' }}</span>
          <span role="cell" class="c-model mono-cell" :data-tip="view.model || undefined">{{ view.model || '—' }}</span>
          <span role="cell" class="right c-beat">
            <time v-if="view.session.heartbeat_at" :datetime="view.session.heartbeat_at">{{ relativeTime(view.session.heartbeat_at, { now }) }}</time>
            <span v-else class="faint">never</span>
          </span>
          <span role="cell" class="right c-elapsed mono-cell">{{ elapsed(view.session, now) }}</span>
          <span role="cell" class="c-actions">
            <template v-if="view.status.group !== 'stopped'">
              <button
                type="button" class="icon-btn sm flat act" :aria-label="`Interrupt ${view.name}`" :data-tip="controlBlock(view, 'interrupt') || 'Interrupt: stop the current turn, keep the session'"
                :aria-disabled="!!controlBlock(view, 'interrupt')" @click="!controlBlock(view, 'interrupt') && emit('control', view, 'interrupt')"
              ><AppIcon name="interrupt" :size="16" /></button>
              <button
                type="button" class="icon-btn sm flat act stop" :aria-label="`Stop ${view.name}`" :data-tip="controlBlock(view, 'stop') || 'Stop: end this session'"
                :aria-disabled="!!controlBlock(view, 'stop')" @click="!controlBlock(view, 'stop') && emit('control', view, 'stop')"
              ><AppIcon name="halt" :size="16" /></button>
              <button
                type="button" class="icon-btn flat more" :aria-label="`Actions for ${view.name}`" aria-haspopup="menu"
                :aria-expanded="menu?.view.session.id === view.session.id" @click="openMenu(view, $event)"
              ><AppIcon name="more" :size="16" /></button>
            </template>
          </span>
        </div>
      </template>
    </div>
    <FloatingPanel v-if="menu" :anchor="menu.anchor" align="end" :width="248" :label="`Actions for ${menu.view.name}`" @close="menu = null">
      <div role="menu" :aria-label="`Actions for ${menu.view.name}`">
        <button type="button" role="menuitem" class="menu-item" :aria-disabled="!!controlBlock(menu.view, 'interrupt')" data-autofocus @click="pick('interrupt')">
          <AppIcon name="interrupt" :size="16" /><span class="mi-text"><span>Interrupt</span><small>{{ controlBlock(menu.view, 'interrupt') || 'Stop the current turn, keep the session' }}</small></span>
        </button>
        <button type="button" role="menuitem" class="menu-item danger" :aria-disabled="!!controlBlock(menu.view, 'stop')" @click="pick('stop')">
          <AppIcon name="halt" :size="16" /><span class="mi-text"><span>Stop session…</span><small>{{ controlBlock(menu.view, 'stop') || 'End this session; asks first' }}</small></span>
        </button>
      </div>
    </FloatingPanel>
  </section>
</template>

<style scoped>
.sessions { overflow: clip; container: sessions / inline-size; }
.card-head { display: flex; align-items: baseline; gap: 10px; padding: 14px 18px 10px; }
.card-head h2 { font-size: 15px; font-weight: 650; }
.sub { font-size: 12.5px; color: var(--ink-3); }
.table { display: grid; grid-template-columns: 132px minmax(170px, 1.3fr) minmax(96px, .8fr) minmax(90px, .8fr) minmax(110px, .9fr) 84px 72px 76px; padding: 0 0 8px; }
.thead, .row, .group-row { display: grid; grid-template-columns: subgrid; grid-column: 1 / -1; align-items: center; column-gap: 0; }
.thead { height: 32px; padding: 0 12px; border-top: 1px solid var(--line); border-bottom: 1px solid var(--line); font: 500 10.5px/1 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; white-space: nowrap; }
.thead > span, .row > span { padding: 0 8px; min-width: 0; }
.right { text-align: right; justify-content: flex-end; }
.group-row { margin: 10px 6px 2px; padding: 0 12px; }
.group-label { grid-column: 1 / -1; display: inline-flex; align-items: center; gap: 8px; height: 26px; font: 500 10.5px/1 var(--mono); letter-spacing: .16em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.group-row.needs .group-label { color: var(--gold-ink); }
.group-label .mono { letter-spacing: 0; color: var(--ink-3); }
.group-toggle { display: inline-flex; align-items: center; gap: 8px; height: 26px; margin-left: -6px; padding: 0 8px 0 6px; border: 0; border-radius: 8px; background: transparent; font: inherit; letter-spacing: inherit; text-transform: inherit; color: inherit; }
.group-toggle:hover { background: var(--row-hover); color: var(--ink); }
.group-toggle:focus-visible { box-shadow: var(--focus-ring); }
.chev.turned { transform: rotate(90deg); }
.row { position: relative; min-height: 48px; margin: 0 6px; padding: 0 4px; border-radius: 10px; outline: none; cursor: pointer; font-size: 13px; }
@media (hover: hover) { .row:hover { background: var(--row-hover); } }
.row.active { background: var(--row-selected); box-shadow: inset 3px 0 0 var(--row-accent), 0 0 0 1px var(--glass-rim); }
.row.selected { background: var(--row-selected); }
.row.stopped { color: var(--ink-2); }
.c-state { display: inline-flex; align-items: center; gap: 9px; }
.state-label { font-size: 12.5px; color: var(--ink-2); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row.needs .state-label { color: var(--gold-ink); font-weight: 600; }
.c-agent { display: inline-flex; align-items: center; gap: 8px; }
.agent-link { display: inline-flex; align-items: center; gap: 8px; min-width: 0; color: var(--ink); text-decoration: none; }
.agent-link:focus-visible { box-shadow: var(--focus-ring); border-radius: 6px; }
.agent-name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row:hover .agent-name { color: var(--teal-ink); }
.harness { flex-shrink: 0; display: inline-flex; align-items: center; height: 20px; padding: 0 7px; border-radius: 6px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); font: 500 10.5px/1 var(--mono); letter-spacing: .03em; color: var(--ink-2); font-variant-ligatures: none; }
.role { flex-shrink: 0; height: 18px; padding: 0 6px; border-radius: 999px; background: var(--gold-wash); color: var(--gold-ink); font: 600 10px/18px var(--mono); letter-spacing: .06em; text-transform: uppercase; font-variant-ligatures: none; }
.ticket-chip { display: inline-flex; align-items: center; height: 22px; padding: 0 8px; border-radius: 6px; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); font: 600 11.5px/1 var(--mono); text-decoration: none; font-variant-ligatures: none; white-space: nowrap; }
.ticket-chip:hover { filter: brightness(1.04); text-decoration: underline; }
.ticket-chip:focus-visible { box-shadow: var(--focus-ring); }
.row .c-account, .row .c-model { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--ink-2); }
.row .c-model { font-size: 12px; }
.row .c-beat { font-size: 12.5px; color: var(--ink-2); white-space: nowrap; }
.row .c-elapsed { font-size: 12px; color: var(--ink-2); white-space: nowrap; font-variant-numeric: tabular-nums; }
.faint { color: var(--ink-3); }
.mono-cell { font-family: var(--mono); font-variant-ligatures: none; }
/* Controls appear on the row the pointer or keyboard is on; the layout never shifts. */
.c-actions { display: inline-flex; justify-content: flex-end; gap: 2px; }
.act { opacity: 0; }
.row:hover .act, .row.active .act, .row.selected .act, .row:focus-within .act { opacity: 1; }
.act[aria-disabled="true"] { color: var(--ink-3); cursor: not-allowed; }
.row:hover .act[aria-disabled="true"], .row.active .act[aria-disabled="true"], .row.selected .act[aria-disabled="true"] { opacity: .45; }
.stop:not([aria-disabled="true"]):hover { color: var(--danger); background: var(--danger-bg); }
.more { display: none; }
@media (hover: none) { .act { display: none; } .more { display: grid; } }
.menu-item { display: flex; align-items: flex-start; gap: 10px; width: 100%; padding: 8px 10px; border: 0; border-radius: 8px; background: transparent; color: var(--ink); font-size: 13.5px; text-align: left; }
.menu-item > svg { margin-top: 2px; color: var(--ink-2); flex-shrink: 0; }
.menu-item:hover:not([aria-disabled="true"]) { background: var(--row-hover); }
.menu-item:focus-visible { background: var(--row-selected); box-shadow: inset 0 0 0 1px var(--glass-rim); }
.menu-item.danger:not([aria-disabled="true"]), .menu-item.danger:not([aria-disabled="true"]) > svg { color: var(--danger); }
.menu-item[aria-disabled="true"] { color: var(--ink-3); cursor: not-allowed; }
.mi-text { display: grid; gap: 2px; min-width: 0; }
.mi-text small { font-size: 11.5px; color: var(--ink-3); line-height: 1.35; }
.state { display: grid; justify-items: center; gap: 8px; padding: 48px 24px 56px; text-align: center; color: var(--ink-2); border-top: 1px solid var(--line); }
.state > svg { color: var(--teal); margin-bottom: 4px; }
.state h3 { color: var(--ink); font-size: 16px; }
.state p { max-width: 52ch; font-size: 13.5px; }
.state .btn { margin-top: 8px; }
.skeleton-rows { display: grid; gap: 4px; padding: 8px 18px 16px; border-top: 1px solid var(--line); }
.sk-row { display: flex; align-items: center; gap: 18px; height: 40px; }
.sk-row .dot { width: 10px; height: 10px; border-radius: 50%; }
.sk-row .key { width: 70px; height: 20px; border-radius: 6px; }
@container sessions (max-width: 920px) {
  .table { grid-template-columns: 118px minmax(150px, 1.3fr) minmax(96px, .8fr) minmax(100px, .9fr) 80px 64px 76px; }
  .c-account { display: none; }
}
@container sessions (max-width: 760px) {
  .table { grid-template-columns: 112px minmax(140px, 1fr) minmax(90px, auto) 78px 76px; }
  .c-model, .c-elapsed { display: none; }
}
/* Phones: two lines per session, actions live in the session panel. */
@container sessions (max-width: 560px) {
  .table { display: block; }
  .thead { display: none; }
  .group-row { display: block; margin: 12px 8px 2px; padding: 0 8px; }
  .row { display: grid; grid-template-columns: auto minmax(0, 1fr) auto 44px; grid-template-areas: "agent agent beat actions" "state ticket ticket actions"; row-gap: 6px; column-gap: 0; min-height: 64px; margin: 0 6px; padding: 10px 4px 10px 10px; }
  .row > span { padding: 0; }
  .c-agent { grid-area: agent; }
  .c-state { grid-area: state; margin-right: 10px; }
  .c-ticket { grid-area: ticket; justify-self: start; }
  .c-beat { grid-area: beat; }
  .c-account, .c-model, .c-elapsed { display: none; }
  .c-actions { grid-area: actions; align-self: center; justify-content: center; }
  .act { display: none; }
  .more { display: grid; width: 44px; height: 44px; }
}
</style>
