<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { ref } from 'vue'
import type { ListItem } from '../../lib/api'
import { absoluteTime, kindLabel, priorityLabel, relativeTime, statusMeta } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import MarkdownBody from '../MarkdownBody.vue'
import PersonAvatar from './PersonAvatar.vue'
import PriorityIcon from './PriorityIcon.vue'
import StatusIcon from './StatusIcon.vue'

const props = defineProps<{
  item: ListItem | null
  ticketKey: string
  loading: boolean
  error: string
  position: { index: number; count: number } | null
  now: number
}>()
const emit = defineEmits<{ close: []; prev: []; next: []; newTab: []; copy: []; status: [anchor: HTMLElement]; openParent: [key: string]; retry: [] }>()
const root = ref<HTMLElement>()
const scroller = ref<HTMLElement>()
function focus() { root.value?.focus({ preventScroll: true }) }
function resetScroll() { scroller.value?.scrollTo({ top: 0 }) }
defineExpose({ focus, resetScroll, el: root })
</script>

<template>
  <aside ref="root" class="ticket-panel" aria-label="Ticket details" tabindex="-1">
    <header class="panel-bar">
      <button type="button" class="key-chip" :aria-label="`Copy ${item?.key ?? ticketKey}`" :data-tip="`Copy ${item?.key ?? ticketKey}`" @click="emit('copy')">
        <AppIcon v-if="item" :name="item.kind_slug === 'epic' ? 'epic' : item.kind_slug === 'task' ? 'task' : 'ticket'" :size="12" :class="item.kind_slug" />
        <span>{{ item?.key ?? ticketKey }}</span>
      </button>
      <span v-if="position" class="position mono">{{ position.index + 1 }} / {{ position.count }}</span>
      <div class="nav">
        <button type="button" class="icon-btn sm flat" aria-label="Previous ticket" aria-keyshortcuts="k" data-tip="Previous · k" :disabled="!position || position.index === 0" @click="emit('prev')"><AppIcon name="chevron-up" :size="15" /></button>
        <button type="button" class="icon-btn sm flat" aria-label="Next ticket" aria-keyshortcuts="j" data-tip="Next · j" :disabled="!position || position.index >= position.count - 1" @click="emit('next')"><AppIcon name="chevron" :size="15" /></button>
      </div>
      <span class="spacer" />
      <button type="button" class="icon-btn sm flat" aria-label="Open in a new tab" data-tip="Open in new tab" @click="emit('newTab')"><AppIcon name="external" :size="14" /></button>
      <button type="button" class="icon-btn sm flat" aria-label="Close ticket details" aria-keyshortcuts="Escape" data-tip="Close · Esc" @click="emit('close')"><AppIcon name="close" :size="15" /></button>
    </header>

    <div ref="scroller" class="panel-body">
      <div v-if="error && !item" class="panel-state" role="alert">
        <AppIcon name="alert" :size="18" />
        <p>{{ error }}</p>
        <button type="button" class="btn sm" @click="emit('retry')">Try again</button>
      </div>
      <div v-else-if="!item" class="panel-loading" role="status" aria-label="Loading ticket">
        <span class="skeleton w40" /><span class="skeleton title-skel" /><span class="skeleton w70" />
        <span class="skeleton w90" /><span class="skeleton w80" /><span class="skeleton w60" />
      </div>
      <template v-else>
        <button v-if="item.parent && item.parent.kind_slug !== 'project'" type="button" class="parent-link" @click="emit('openParent', item.parent.key)">
          <AppIcon :name="item.parent.kind_slug === 'epic' ? 'epic' : 'ticket'" :size="12" :class="item.parent.kind_slug" />
          <span class="mono">{{ item.parent.key }}</span>
          <span class="parent-title">{{ item.parent.title }}</span>
        </button>
        <h2 class="panel-title">{{ item.title }}</h2>
        <div class="facts">
          <button type="button" class="fact status-fact" aria-haspopup="menu" :aria-label="`Status: ${statusMeta(item.state).label}. Change status`" @click="emit('status', $event.currentTarget as HTMLElement)">
            <StatusIcon :state="item.state" />{{ statusMeta(item.state).label }}<AppIcon name="chevron" :size="12" class="fact-chevron" />
          </button>
          <span class="fact">
            <PriorityIcon v-if="item.priority && item.priority !== 'none'" :priority="item.priority" />
            <span v-else class="dash">—</span>{{ priorityLabel(item.priority) }}
          </span>
          <span class="fact">
            <PersonAvatar v-if="item.assignee" :name="item.assignee.name" :size="18" />
            <AppIcon v-else name="user" :size="13" class="unassigned" />{{ item.assignee?.name ?? 'Unassigned' }}
          </span>
          <span class="fact quiet">{{ kindLabel(item.kind_slug) }}<template v-if="item.children_count"> · {{ item.children_count }} inside</template></span>
        </div>
        <p class="meta">
          Updated <time :datetime="item.updated_at" :data-tip="absoluteTime(item.updated_at)">{{ relativeTime(item.updated_at, { now, long: true }) }}</time>
          · Created <time :datetime="item.created_at" :data-tip="absoluteTime(item.created_at)">{{ relativeTime(item.created_at, { now, long: true }) }}</time>
        </p>
        <div class="divider" />
        <MarkdownBody v-if="item.body.trim()" :body="item.body" />
        <p v-else class="no-body">No description yet.</p>
        <p v-if="loading" class="refreshing" role="status">Refreshing…</p>
      </template>
    </div>
  </aside>
</template>

<style scoped>
.ticket-panel {
  position: fixed; z-index: 15; top: calc(var(--header-h) + 10px); right: 10px; bottom: 10px; width: min(560px, calc(100vw - 20px));
  display: flex; flex-direction: column; border-radius: var(--radius); border: 1px solid var(--glass-edge); outline: none;
  background: linear-gradient(165deg, var(--surface-raised), var(--surface-raised-2)); box-shadow: var(--shadow-pop), var(--shadow);
  backdrop-filter: blur(20px) saturate(1.15); -webkit-backdrop-filter: blur(20px) saturate(1.15);
}
.ticket-panel:focus-visible { box-shadow: var(--shadow-pop), var(--focus-ring); }
.panel-bar { display: flex; align-items: center; gap: 6px; height: 52px; padding: 0 10px 0 14px; border-bottom: 1px solid var(--line); flex-shrink: 0; }
.key-chip { display: inline-flex; align-items: center; gap: 6px; height: 26px; padding: 0 10px; border: 0; border-radius: 7px; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); font: 600 12px/1 var(--mono); letter-spacing: .03em; font-variant-ligatures: none; }
.key-chip:hover { filter: brightness(1.03); box-shadow: inset 0 0 0 1px var(--teal); }
.key-chip:active { filter: brightness(.97); }
.key-chip:focus-visible, .parent-link:focus-visible, .status-fact:focus-visible { box-shadow: var(--focus-ring); }
.key-chip .epic { color: var(--gold); }
.position { margin-left: 6px; font-size: 11.5px; color: var(--ink-3); }
.nav { display: inline-flex; gap: 2px; margin-left: 2px; }
.nav .icon-btn:disabled { opacity: .35; }
.spacer { flex: 1; }
.panel-body { flex: 1; min-height: 0; overflow: auto; padding: 18px 24px 28px; overscroll-behavior: contain; }
.parent-link { display: inline-flex; align-items: center; gap: 7px; max-width: 100%; height: 24px; margin: 0 0 8px -8px; padding: 0 8px; border: 0; border-radius: 6px; background: transparent; color: var(--ink-2); font-size: 12.5px; }
.parent-link:hover { background: var(--row-hover); color: var(--ink); }
.parent-link .epic { color: var(--gold); }
.parent-link .mono { font-size: 11px; }
.parent-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.panel-title { font: 500 21px/1.3 var(--serif); letter-spacing: -.012em; color: var(--ink); overflow-wrap: anywhere; }
.facts { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 14px; }
.fact { display: inline-flex; align-items: center; gap: 7px; height: 28px; padding: 0 11px 0 9px; border: 0; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); color: var(--ink); font-size: 12.5px; white-space: nowrap; }
.fact.quiet { color: var(--ink-2); padding-left: 11px; }
.status-fact:hover, .status-fact[aria-expanded="true"] { box-shadow: inset 0 0 0 1px var(--glass-rim); background: var(--row-hover); }
.status-fact:active { background: var(--row-selected); }
.fact-chevron { color: var(--ink-3); margin-left: -2px; }
.dash, .unassigned { color: var(--ink-3); }
.meta { margin-top: 12px; font-size: 12.5px; color: var(--ink-3); }
.meta time { color: var(--ink-2); }
.divider { height: 1px; margin: 18px 0 20px; background: linear-gradient(90deg, var(--line-2), transparent); }
.no-body { font-size: 13.5px; color: var(--ink-3); }
.refreshing { margin-top: 12px; font-size: 12px; color: var(--ink-3); }
.panel-state { display: grid; justify-items: center; gap: 10px; padding: 48px 12px; text-align: center; color: var(--ink-2); }
.panel-state svg { color: var(--danger); }
.panel-loading { display: grid; gap: 14px; padding-top: 6px; }
.panel-loading .w40 { width: 40%; } .panel-loading .w70 { width: 70%; } .panel-loading .w90 { width: 90%; } .panel-loading .w80 { width: 80%; } .panel-loading .w60 { width: 60%; }
.panel-loading .title-skel { width: 85%; height: 18px; border-radius: 8px; }
@media (min-width: 1100px) { .ticket-panel { width: var(--panel-w); } }
@media (prefers-reduced-motion: no-preference) {
  .ticket-panel { animation: panel-in .22s cubic-bezier(.2, .7, .2, 1); }
  @keyframes panel-in { from { opacity: 0; transform: translateX(24px); } to { opacity: 1; transform: none; } }
}
@media (max-width: 720px) {
  .ticket-panel { z-index: 40; inset: 0; width: auto; border-radius: 0; border: 0; background: var(--canvas); }
  .panel-bar { height: 56px; padding: 0 6px 0 12px; }
  .panel-bar .icon-btn { width: 44px; height: 44px; }
  .panel-body { padding: 16px 18px 32px; }
  .panel-title { font-size: 20px; }
  @media (prefers-reduced-motion: no-preference) { .ticket-panel { animation-name: sheet-in; } @keyframes sheet-in { from { transform: translateY(24px); opacity: 0; } to { transform: none; opacity: 1; } } }
}
</style>
