<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { ListItem } from '../../lib/api'
import { absoluteTime, kindLabel, priorityLabel, relativeTime, statusMeta } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import PersonAvatar from './PersonAvatar.vue'
import PriorityIcon from './PriorityIcon.vue'
import StatusIcon from './StatusIcon.vue'
import LiveDot from '../agents/LiveDot.vue'
import { useAgents } from '../../stores/agents'

// Status, priority, assignee (editable popovers), type (read-only), the parent
// epic, and estimate, dates and release only when they have values.
const props = defineProps<{ item: ListItem; editable: boolean; layout: 'row' | 'column'; now: number }>()
const emit = defineEmits<{ status: [anchor: HTMLElement]; priority: [anchor: HTMLElement]; assignee: [anchor: HTMLElement]; epic: [anchor: HTMLElement]; openParent: [key: string] }>()

function text(value: unknown): string { return typeof value === 'string' ? value.trim() : '' }
function day(value: unknown): string {
  const raw = text(value)
  if (!raw) return ''
  const time = Date.parse(raw.length === 10 ? `${raw}T12:00:00` : raw)
  return Number.isNaN(time) ? raw : new Date(time).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}
const estimate = computed(() => {
  const hours = props.item.fields.estimate_hours
  const points = props.item.fields.estimate_lp
  if (typeof hours === 'number' && hours > 0) return `${hours} h`
  if (typeof points === 'number' && points > 0) return `${points} pt`
  return ''
})
const start = computed(() => day(props.item.fields.start_date))
const due = computed(() => day(props.item.fields.end_date))
const release = computed(() => {
  const value = props.item.fields.release
  return value && typeof value === 'object' && typeof (value as { label?: unknown }).label === 'string' ? (value as { label: string }).label : ''
})
// Agent sessions bound to this ticket, with their live state; refreshed while shown.
const agents = useAgents()
const bound = computed(() => agents.forTicket(props.item.id))
let refresh: ReturnType<typeof setInterval> | undefined
watch(() => props.item.id, id => { void agents.ensureTicket(id) }, { immediate: true })
onMounted(() => { refresh = setInterval(() => void agents.ensureTicket(props.item.id), 20_000) })
onBeforeUnmount(() => clearInterval(refresh))
const epicParent = computed(() => props.item.parent && props.item.parent.kind_slug !== 'project' ? props.item.parent : null)
const target = (event: Event) => event.currentTarget as HTMLElement
// The phone chip row scrolls sideways; its fades show which way there is more.
const scrolledX = ref(0)
const atEnd = ref(false)
function onScroll(event: Event) {
  const el = event.currentTarget as HTMLElement
  scrolledX.value = el.scrollLeft
  atEnd.value = el.scrollLeft + el.clientWidth >= el.scrollWidth - 4
}
</script>

<template>
  <dl class="props" :class="[layout, { scrolled: scrolledX > 4, 'at-end': atEnd }]" @scroll.passive="onScroll">
    <div class="prop">
      <dt>Status</dt>
      <dd><button type="button" class="prop-btn" :disabled="!editable" aria-haspopup="menu" aria-keyshortcuts="s" :aria-label="`Status: ${statusMeta(item.state).label}. Change status`" @click="emit('status', target($event))"><StatusIcon :state="item.state" />{{ statusMeta(item.state).label }}<AppIcon v-if="editable" name="chevron" :size="12" class="chev" /></button></dd>
    </div>
    <div v-if="bound.length" class="prop agents-prop">
      <dt>Agents</dt>
      <dd class="agent-chips">
        <RouterLink
          v-for="view in bound" :key="view.session.id" class="prop-btn agent-chip" :class="view.status.group" :to="`/agents/${view.session.id}`"
          :aria-label="`${view.harness} ${view.name}: ${view.status.label}. Open the session`" :data-tip="`${view.harness} · ${view.status.label}`"
        ><LiveDot :tone="view.status.tone" :size="8" /><span class="agent-name">{{ view.name }}</span></RouterLink>
      </dd>
    </div>
    <div class="prop">
      <dt>Priority</dt>
      <dd><button type="button" class="prop-btn" :disabled="!editable" aria-haspopup="menu" aria-keyshortcuts="p" :aria-label="`Priority: ${priorityLabel(item.priority)}. Change priority`" @click="emit('priority', target($event))"><PriorityIcon v-if="item.priority" :priority="item.priority" /><span v-else class="dash">—</span><span :class="{ unset: !item.priority }">{{ item.priority ? priorityLabel(item.priority) : 'No priority' }}</span><AppIcon v-if="editable" name="chevron" :size="12" class="chev" /></button></dd>
    </div>
    <div class="prop">
      <dt>Assignee</dt>
      <dd><button type="button" class="prop-btn" :disabled="!editable" aria-haspopup="menu" aria-keyshortcuts="a" :aria-label="`Assignee: ${item.assignee?.name ?? 'nobody'}. Change assignee`" @click="emit('assignee', target($event))"><PersonAvatar v-if="item.assignee" :name="item.assignee.name" :size="18" /><AppIcon v-else name="user" :size="13" class="faint" /><span :class="{ unset: !item.assignee }">{{ item.assignee?.name ?? 'Unassigned' }}</span><AppIcon v-if="editable" name="chevron" :size="12" class="chev" /></button></dd>
    </div>
    <div class="prop">
      <dt>Type</dt>
      <dd><span class="prop-static"><AppIcon :name="item.kind_slug === 'epic' ? 'epic' : item.kind_slug === 'task' ? 'task' : 'ticket'" :size="13" :class="['kind', item.kind_slug]" />{{ kindLabel(item.kind_slug) }}</span></dd>
    </div>
    <div v-if="item.kind_slug !== 'epic'" class="prop">
      <dt>{{ epicParent && epicParent.kind_slug !== 'epic' ? 'Parent' : 'Epic' }}</dt>
      <dd class="epic-cell">
        <button v-if="epicParent" type="button" class="prop-btn epic-chip" :data-tip="`Open ${epicParent.key}\n${epicParent.title}`" @click="emit('openParent', epicParent.key)">
          <AppIcon :name="epicParent.kind_slug === 'epic' ? 'epic' : 'ticket'" :size="12" :class="['kind', epicParent.kind_slug]" /><span class="mono">{{ epicParent.key }}</span><span class="epic-title">{{ epicParent.title }}</span>
        </button>
        <button v-else-if="editable" type="button" class="prop-btn ghost" aria-label="No epic. Choose an epic" @click="emit('epic', target($event))"><AppIcon name="epic" :size="12" class="faint" /><span class="unset">No epic</span></button>
        <span v-else class="prop-static faint">No epic</span>
      </dd>
    </div>
    <div v-if="estimate" class="prop"><dt>Estimate</dt><dd><span class="prop-static"><span v-if="layout === 'row'" class="inline-label">Estimate</span><span class="mono">{{ estimate }}</span></span></dd></div>
    <div v-if="start" class="prop"><dt>Start</dt><dd><span class="prop-static"><span v-if="layout === 'row'" class="inline-label">Start</span>{{ start }}</span></dd></div>
    <div v-if="due" class="prop"><dt>Due</dt><dd><span class="prop-static"><span v-if="layout === 'row'" class="inline-label">Due</span>{{ due }}</span></dd></div>
    <div v-if="release" class="prop"><dt>Release</dt><dd><span class="prop-static"><span v-if="layout === 'row'" class="inline-label">Release</span><span class="mono">{{ release }}</span></span></dd></div>
    <div v-if="layout === 'column'" class="prop"><dt>Updated</dt><dd><time class="prop-static" :datetime="item.updated_at" :data-tip="absoluteTime(item.updated_at)">{{ relativeTime(item.updated_at, { now, long: true }) }}</time></dd></div>
    <div v-if="layout === 'column'" class="prop"><dt>Created</dt><dd><time class="prop-static" :datetime="item.created_at" :data-tip="absoluteTime(item.created_at)">{{ relativeTime(item.created_at, { now, long: true }) }}</time></dd></div>
  </dl>
</template>

<style scoped>
.props { margin: 0; }
.props.row { display: flex; flex-wrap: wrap; gap: 6px; }
.props.row dt { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
.props.row dd { margin: 0; }
.props.column { display: grid; gap: 2px; }
.props.column .prop { display: grid; grid-template-columns: 92px minmax(0, 1fr); align-items: center; min-height: 34px; }
.props.column dt { font: 500 10.5px/1 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.props.column dd { margin: 0; min-width: 0; }
.prop-btn, .prop-static { display: inline-flex; align-items: center; gap: 7px; max-width: 100%; height: 28px; padding: 0 11px 0 9px; border: 0; border-radius: 999px; font-size: 12.5px; color: var(--ink); white-space: nowrap; }
.row .prop-btn, .row .prop-static { background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); }
.column .prop-btn, .column .prop-static { margin-left: -9px; background: transparent; }
.prop-btn { cursor: pointer; }
@media (hover: hover) { .prop-btn:not(:disabled):hover { background: var(--row-hover); box-shadow: inset 0 0 0 1px var(--glass-rim); } }
.prop-btn:active:not(:disabled) { background: var(--row-selected); }
.prop-btn:focus-visible { box-shadow: var(--focus-ring); }
.prop-btn:disabled { cursor: default; opacity: 1; }
.prop-btn[aria-expanded="true"] { background: var(--row-selected); }
.prop-btn.ghost { color: var(--ink-3); }
.chev { color: var(--ink-3); margin-left: -2px; }
.dash, .faint, .unset { color: var(--ink-3); }
.kind { color: var(--ink-3); }
.kind.epic { color: var(--gold); }
.agent-chips { display: flex; flex-wrap: wrap; gap: 6px; }
.row .agent-chips { flex-wrap: nowrap; }
.agent-chip { text-decoration: none; }
.agent-chip.needs { box-shadow: inset 0 0 0 1px rgba(214, 155, 49, .45); }
.agent-name { max-width: 16ch; overflow: hidden; text-overflow: ellipsis; font-weight: 600; }
.column .agent-chips { gap: 2px 10px; }
.epic-chip { max-width: 100%; }
.epic-chip .mono { font-size: 11px; color: var(--ink-2); }
.epic-title { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
.row .epic-chip { max-width: 280px; }
.mono { font-family: var(--mono); font-size: 11.5px; font-variant-numeric: tabular-nums; font-variant-ligatures: none; }
.inline-label { font: 500 9.5px/1 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
@media (max-width: 720px) {
  .props.row {
    flex-wrap: nowrap; overflow-x: auto; margin: 14px -18px 0; padding: 2px 18px 4px; scrollbar-width: none; scroll-padding-inline: 18px;
    -webkit-mask-image: linear-gradient(to right, #000 0, #000 calc(100% - 72px), rgba(0, 0, 0, .15) calc(100% - 14px), transparent);
    mask-image: linear-gradient(to right, #000 0, #000 calc(100% - 72px), rgba(0, 0, 0, .15) calc(100% - 14px), transparent);
  }
  /* Once scrolled, the start fades too; a trailing spacer lets the last chip clear the fade. */
  .props.row.scrolled {
    -webkit-mask-image: linear-gradient(to right, transparent, rgba(0, 0, 0, .15) 14px, #000 56px, #000 calc(100% - 72px), rgba(0, 0, 0, .15) calc(100% - 14px), transparent);
    mask-image: linear-gradient(to right, transparent, rgba(0, 0, 0, .15) 14px, #000 56px, #000 calc(100% - 72px), rgba(0, 0, 0, .15) calc(100% - 14px), transparent);
  }
  .props.row.at-end {
    -webkit-mask-image: linear-gradient(to right, transparent, rgba(0, 0, 0, .15) 14px, #000 56px);
    mask-image: linear-gradient(to right, transparent, rgba(0, 0, 0, .15) 14px, #000 56px);
  }
  .props.row::after { content: ''; flex: 0 0 40px; }
  .props.row::-webkit-scrollbar { display: none; }
  .props.row .prop { flex-shrink: 0; }
  .row .prop-btn, .row .prop-static { height: 34px; }
}
</style>
