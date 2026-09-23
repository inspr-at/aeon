<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, ref, toRef, watch } from 'vue'
import type { ListItem } from '../../lib/api'
import { confirmAction } from '../../lib/confirm'
import { toast } from '../../lib/toast'
import { useActivity } from '../../lib/useActivity'
import { useTicket } from '../../lib/useTicket'
import { absoluteTime, kindLabel, relativeTime } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import ActivityTimeline from './ActivityTimeline.vue'
import ChildList from './ChildList.vue'
import CommentComposer from './CommentComposer.vue'
import EpicPicker from './EpicPicker.vue'
import InlineTitle from './InlineTitle.vue'
import MarkdownSection from './MarkdownSection.vue'
import OptionMenu, { type MenuOption } from './OptionMenu.vue'
import RelationList from './RelationList.vue'
import TicketHeaderBar from './TicketHeaderBar.vue'
import TicketProperties from './TicketProperties.vue'

// The ticket workspace: the same parts in the docked side panel and in the
// full page. Every write goes through useTicket (precondition, conflicts).
const props = defineProps<{
  item: ListItem | null; ticketKey: string; resolving: boolean; resolveError: string
  position: { index: number; count: number } | null; now: number; mode: 'panel' | 'full'
  project: { id: string; routeKey: string }; names: Map<string, string>
  me: { id: string; name: string } | null; canWrite: boolean; people: { id: string; name: string }[]
}>()
const emit = defineEmits<{
  close: []; prev: []; next: []; expand: []; collapse: []; newTab: []; openKey: [key: string]; status: [anchor: HTMLElement]
  removed: [item: ListItem]; created: [item: ListItem]; retry: []
}>()

const item = toRef(props, 'item')
const ticket = useTicket(item, {
  names: props.names,
  onRemoved: removed => emit('removed', removed),
  onCreated: created => emit('created', created),
})
const activity = useActivity(computed(() => props.item?.id ?? null))
const editable = computed(() => props.canWrite && !ticket.readOnly.value && !ticket.gone.value)

const root = ref<HTMLElement>()
const scroller = ref<HTMLElement>()
const title = ref<InstanceType<typeof InlineTitle>>()
const descSection = ref<InstanceType<typeof MarkdownSection>>()
const acSection = ref<InstanceType<typeof MarkdownSection>>()
const notesSection = ref<InstanceType<typeof MarkdownSection>>()
const sections = computed(() => [descSection.value, acSection.value, notesSection.value].filter(section => !!section))
const composer = ref<InstanceType<typeof CommentComposer>>()
const timeline = ref<InstanceType<typeof ActivityTimeline>>()
const menu = ref<{ kind: 'priority' | 'assignee' | 'epic'; anchor: HTMLElement } | null>(null)
const showAcceptance = ref(false)
const showNotes = ref(false)

const acceptance = computed(() => typeof props.item?.fields.acceptance_criteria === 'string' ? props.item.fields.acceptance_criteria : '')
const notes = computed(() => typeof props.item?.fields.notes === 'string' ? props.item.fields.notes : '')
const hasChildren = computed(() => !!props.item && (props.item.kind_slug === 'epic' || props.item.children_count > 0 || ticket.children.value.length > 0))
const priorityOptions: MenuOption[] = [{ value: 'high', label: 'High' }, { value: 'medium', label: 'Medium' }, { value: 'low', label: 'Low' }, { value: '', label: 'No priority' }]
const assigneeOptions = computed<MenuOption[]>(() => {
  const people = new Map(props.people.map(person => [person.id, person.name]))
  if (props.me) people.set(props.me.id, props.me.name)
  const others = [...people.entries()].filter(([id]) => id !== props.me?.id).sort((a, b) => a[1].localeCompare(b[1]))
  return [
    ...(props.me ? [{ value: props.me.id, label: props.me.name, hint: 'you' }] : []),
    ...others.map(([id, name]) => ({ value: id, label: name })),
    { value: '', label: 'Unassigned' },
  ]
})
watch(() => props.item?.id, () => {
  showAcceptance.value = false; showNotes.value = false; menu.value = null
  scroller.value?.scrollTo({ top: 0 })
})

function link() { return `${location.origin}/p/${encodeURIComponent(props.project.routeKey)}/${encodeURIComponent(props.item?.key ?? props.ticketKey)}` }
function copy(text: string, label: string) {
  navigator.clipboard.writeText(text).then(() => toast(`Copied ${label}`), () => toast(`${label} could not be copied`, { tone: 'error' }))
}
function anchorFor(shortcut: string) { return root.value?.querySelector<HTMLElement>(`[aria-keyshortcuts="${shortcut}"]`) ?? null }
function openMenu(kind: 'priority' | 'assignee' | 'epic', anchor: HTMLElement | null) { if (anchor && editable.value) menu.value = { kind, anchor } }
function closeMenu(restore: boolean) { const anchor = menu.value?.anchor; menu.value = null; if (restore) anchor?.focus() }
async function choosePriority(value: string) { const anchor = menu.value?.anchor; menu.value = null; anchor?.focus(); await ticket.setPriority(value || null) }
async function chooseAssignee(value: string) {
  const anchor = menu.value?.anchor; menu.value = null; anchor?.focus()
  const option = assigneeOptions.value.find(o => o.value === value)
  await ticket.setAssignee(value && option ? { id: value, name: option.label } : null)
}
async function chooseEpic(epic: { id: string; key: string; title: string } | null) {
  menu.value = null
  if (epic) await ticket.moveTo(epic)
}
async function remove() {
  const target = props.item
  if (!target) return
  const ok = await confirmAction({
    title: `Delete ${target.key}?`,
    body: `“${target.title}” leaves the project list. ${target.children_count ? 'Its children must be moved or deleted first.' : 'The history stays in the audit log.'}`,
    confirmLabel: `Delete ${kindLabel(target.kind_slug).toLowerCase()}`, danger: true,
  })
  if (ok) await ticket.remove()
}
async function addSection(kind: 'acceptance' | 'notes') {
  if (kind === 'acceptance') showAcceptance.value = true; else showNotes.value = true
  await nextTick()
  ;(kind === 'acceptance' ? acSection.value : notesSection.value)?.start()
}

function isDirty() {
  return !!title.value?.isDirty() || sections.value.some(section => section.isDirty()) || !!composer.value?.isDirty() || !!timeline.value?.isDirty()
}
function focus() { root.value?.focus({ preventScroll: true }) }
defineExpose({
  el: root, focus, isDirty,
  editTitle: () => title.value?.start(),
  openStatus: () => { const anchor = anchorFor('s'); if (anchor && editable.value) emit('status', anchor) },
  openPriority: () => openMenu('priority', anchorFor('p')),
  openAssignee: () => openMenu('assignee', anchorFor('a')),
  focusComposer: () => composer.value?.focus(),
})
</script>

<template>
  <component :is="mode === 'panel' ? 'aside' : 'article'" ref="root" class="ticket-ws" :class="mode" aria-label="Ticket details" tabindex="-1">
    <TicketHeaderBar
      :ticket-key="item?.key ?? ticketKey" :kind="item?.kind_slug ?? null" :position="position" :mode="mode" :can-write="editable"
      :can-move="item?.kind_slug === 'ticket'"
      @copy-key="copy(item?.key ?? ticketKey, item?.key ?? ticketKey)" @copy-link="copy(link(), 'link')" @prev="emit('prev')" @next="emit('next')"
      @expand="emit('expand')" @collapse="emit('collapse')" @new-tab="emit('newTab')" @close="emit('close')"
      @move="anchor => openMenu('epic', anchor)" @delete="remove"
    />

    <div ref="scroller" class="ws-scroll">
      <div v-if="ticket.gone.value" class="ws-state" role="alert">
        <span class="state-icon"><AppIcon name="archive" :size="18" /></span>
        <h2>{{ item?.key ?? ticketKey }} is no longer here</h2>
        <p>It was deleted or moved out of this project.</p>
        <button type="button" class="btn" @click="emit('close')">Back to the list</button>
      </div>
      <div v-else-if="resolveError && !item" class="ws-state" role="alert">
        <span class="state-icon danger"><AppIcon name="alert" :size="18" /></span>
        <h2>This ticket could not be opened</h2>
        <p>{{ resolveError }}</p>
        <button type="button" class="btn" @click="emit('retry')"><AppIcon name="refresh" :size="14" />Try again</button>
      </div>
      <div v-else-if="!item" class="ws-skeleton" role="status" aria-label="Loading ticket">
        <span class="skeleton w30" /><span class="skeleton title-skel" /><span class="skeleton title-skel short" />
        <span class="chips"><span class="skeleton chip" /><span class="skeleton chip" /><span class="skeleton chip" /></span>
        <span class="skeleton w90" /><span class="skeleton w80" /><span class="skeleton w60" /><span class="skeleton w85" />
      </div>

      <div v-else class="ws-grid">
        <div class="ws-main">
          <p v-if="!editable" class="read-only" role="note"><AppIcon name="alert" :size="13" />You can read this {{ kindLabel(item.kind_slug).toLowerCase() }} but not change it.</p>
          <InlineTitle ref="title" :value="item.title" :editable="editable" :large="mode === 'full'" :save="ticket.setTitle" />
          <TicketProperties
            class="ws-props" :class="{ 'only-narrow': mode === 'full' }" :item="item" :editable="editable" layout="row" :now="now"
            @status="anchor => emit('status', anchor)" @priority="anchor => openMenu('priority', anchor)" @assignee="anchor => openMenu('assignee', anchor)"
            @epic="anchor => openMenu('epic', anchor)" @open-parent="key => emit('openKey', key)"
          />
          <p class="meta" :class="{ 'only-narrow': mode === 'full' }">
            Updated <time :datetime="item.updated_at" :data-tip="absoluteTime(item.updated_at)">{{ relativeTime(item.updated_at, { now, long: true }) }}</time>
            · Created <time :datetime="item.created_at" :data-tip="absoluteTime(item.created_at)">{{ relativeTime(item.created_at, { now, long: true }) }}</time>
          </p>
          <div class="divider" />

          <div class="sections">
            <MarkdownSection ref="descSection" title="Description" :value="item.body" :editable="editable" :save="ticket.setBody" empty-text="Add a description" />
            <MarkdownSection v-if="acceptance.trim() || showAcceptance" ref="acSection" title="Acceptance criteria" :value="acceptance" :editable="editable" :save="value => ticket.setField('acceptance_criteria', value)" />
            <MarkdownSection v-if="notes.trim() || showNotes" ref="notesSection" title="Notes" :value="notes" :editable="editable" :save="value => ticket.setField('notes', value)" />
            <div v-if="editable && (!(acceptance.trim() || showAcceptance) || !(notes.trim() || showNotes))" class="add-sections">
              <button v-if="!(acceptance.trim() || showAcceptance)" type="button" class="add-section" @click="addSection('acceptance')"><AppIcon name="plus" :size="12" />Acceptance criteria</button>
              <button v-if="!(notes.trim() || showNotes)" type="button" class="add-section" @click="addSection('notes')"><AppIcon name="plus" :size="12" />Notes</button>
            </div>
          </div>

          <ChildList
            v-if="hasChildren" class="ws-block" :children="ticket.children.value" :loading="ticket.childrenLoading.value" :editable="editable"
            :child-label="item.kind_slug === 'epic' ? 'ticket' : 'task'" :progress="ticket.childProgress()" :add="title => ticket.addChild(title, project.routeKey)"
            @open="key => emit('openKey', key)"
          />
          <RelationList class="ws-block" :class="{ 'only-narrow': mode === 'full' }" :related="ticket.related.value" @open="key => emit('openKey', key)" />

          <ActivityTimeline
            ref="timeline" class="ws-block" :entries="activity.timeline.value" :loading="activity.loading.value" :loading-older="activity.loadingOlder.value"
            :has-older="!!activity.cursor.value" :error="activity.error.value" :me="me?.id" :now="now" :can-write="editable"
            :edit="activity.edit" :remove="activity.remove" @older="activity.loadOlder" @retry="activity.load"
          />
          <CommentComposer v-if="mode === 'full'" ref="composer" class="ws-block inline-composer" :me="me?.name ?? '?'" :post="activity.add" :disabled="!editable" />
        </div>

        <aside v-if="mode === 'full'" class="ws-side" aria-label="Properties">
          <div class="side-card">
            <TicketProperties
              :item="item" :editable="editable" layout="column" :now="now"
              @status="anchor => emit('status', anchor)" @priority="anchor => openMenu('priority', anchor)" @assignee="anchor => openMenu('assignee', anchor)"
              @epic="anchor => openMenu('epic', anchor)" @open-parent="key => emit('openKey', key)"
            />
          </div>
          <div v-if="ticket.related.value.length" class="side-card"><RelationList :related="ticket.related.value" @open="key => emit('openKey', key)" /></div>
        </aside>
      </div>
    </div>

    <footer v-if="mode === 'panel' && item && !ticket.gone.value" class="ws-composer">
      <CommentComposer ref="composer" :me="me?.name ?? '?'" :post="activity.add" :disabled="!editable" />
    </footer>

    <OptionMenu v-if="menu?.kind === 'priority' && item" :anchor="menu.anchor" title="Priority" :subject="item.key" kind="priority" :options="priorityOptions" :current="item.priority ?? ''" @choose="choosePriority" @close="closeMenu" />
    <OptionMenu v-if="menu?.kind === 'assignee' && item" :anchor="menu.anchor" title="Assignee" :subject="item.key" kind="assignee" :options="assigneeOptions" :current="item.assignee?.id ?? ''" searchable @choose="chooseAssignee" @close="closeMenu" />
    <EpicPicker v-if="menu?.kind === 'epic' && item" :anchor="menu.anchor" :project-id="project.id" :current="item.parent?.kind_slug === 'epic' ? item.parent.id : null" :subject="item.key" @choose="chooseEpic" @close="closeMenu" />
  </component>
</template>

<style scoped>
.ticket-ws { display: flex; flex-direction: column; min-height: 0; outline: none; }
.ticket-ws.panel {
  position: fixed; z-index: 15; top: calc(var(--header-h) + 10px); right: 10px; bottom: 10px; width: min(560px, calc(100vw - 20px));
  border-radius: var(--radius); border: 1px solid var(--glass-edge);
  background: linear-gradient(165deg, var(--surface-raised), var(--surface-raised-2)); box-shadow: var(--shadow-pop), var(--shadow);
  backdrop-filter: blur(20px) saturate(1.15); -webkit-backdrop-filter: blur(20px) saturate(1.15);
}
.ticket-ws.panel:focus-visible { box-shadow: var(--shadow-pop), var(--focus-ring); }
.ws-scroll { flex: 1; min-height: 0; overflow: auto; overscroll-behavior: contain; }
.panel .ws-scroll { padding: 18px 24px 24px; }
.ws-props { margin-top: 14px; }
.meta { margin-top: 12px; font-size: 12.5px; color: var(--ink-3); }
.meta time { color: var(--ink-2); }
.divider { height: 1px; margin: 18px 0 20px; background: linear-gradient(90deg, var(--line-2), transparent); }
.read-only { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; padding: 8px 12px; border-radius: 10px; background: var(--code-bg); font-size: 12.5px; color: var(--ink-2); }
.add-sections { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 14px; }
.add-section { display: inline-flex; align-items: center; gap: 5px; height: 26px; padding: 0 10px; border: 0; border-radius: 999px; background: transparent; box-shadow: inset 0 0 0 1px var(--line); color: var(--ink-3); font-size: 12px; }
@media (hover: hover) { .add-section:hover { color: var(--teal-ink); box-shadow: inset 0 0 0 1px var(--glass-rim); background: var(--row-hover); } }
.add-section:focus-visible { box-shadow: var(--focus-ring); }
.ws-block { margin-top: 28px; }
.ws-composer { flex-shrink: 0; padding: 10px 16px 12px; border-top: 1px solid var(--line); background: var(--surface-raised-2); border-radius: 0 0 var(--radius) var(--radius); }
.ws-state { display: grid; justify-items: center; gap: 8px; padding: 56px 16px; text-align: center; }
.ws-state h2 { font-size: 17px; }
.ws-state p { font-size: 13.5px; }
.ws-state .btn { margin-top: 8px; }
.state-icon { display: grid; place-items: center; width: 44px; height: 44px; margin-bottom: 4px; border-radius: 50%; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.state-icon.danger { background: var(--danger-bg); box-shadow: inset 0 0 0 1px var(--danger-line); color: var(--danger); }
.ws-skeleton { display: grid; gap: 14px; padding-top: 4px; }
.panel .ws-skeleton { padding: 0; }
.ws-skeleton .w30 { width: 30%; } .ws-skeleton .w90 { width: 90%; } .ws-skeleton .w80 { width: 80%; } .ws-skeleton .w60 { width: 60%; } .ws-skeleton .w85 { width: 85%; }
.ws-skeleton .title-skel { width: 88%; height: 18px; border-radius: 8px; }
.ws-skeleton .title-skel.short { width: 52%; }
.ws-skeleton .chips { display: flex; gap: 8px; margin: 4px 0 12px; }
.ws-skeleton .chip { width: 86px; height: 26px; border-radius: 999px; }

/* Full page: content left at a readable measure, properties and relations right. */
.ticket-ws.full { width: 100%; max-width: 1160px; min-height: 100%; margin: 0 auto; }
.full .ws-scroll { overflow: visible; }
.full .ws-grid { display: grid; grid-template-columns: minmax(0, 1fr) 320px; gap: 40px; align-items: start; padding: 26px 0 40px; }
.full .ws-main { min-width: 0; max-width: 820px; }
.full .sections :deep(.markdown-body), .full .inline-composer, .full .activity, .full .children { max-width: 72ch; }
.full .ws-side { position: sticky; top: 16px; display: grid; gap: 14px; }
.side-card { padding: 14px 18px; border-radius: var(--radius); border: 1px solid var(--glass-edge); background: linear-gradient(165deg, var(--surface-raised-2), var(--glass) 60%); box-shadow: var(--shadow); }
.side-card :deep(.relation-label) { width: 84px; }
.full .panel-bar { border-bottom: 0; padding: 0; height: 44px; }
@media (min-width: 1100px) { .ticket-ws.panel { width: var(--panel-w); } }
.full .only-narrow { display: none; }
@media (max-width: 980px) {
  .full .ws-grid { grid-template-columns: minmax(0, 1fr); padding-top: 14px; }
  .full .ws-side { display: none; }
  .full .only-narrow { display: revert; }
  .full .ws-props.only-narrow { display: flex; }
}
@media (prefers-reduced-motion: no-preference) {
  .ticket-ws.panel { animation: panel-in .22s cubic-bezier(.2, .7, .2, 1); }
  @keyframes panel-in { from { opacity: 0; transform: translateX(24px); } to { opacity: 1; transform: none; } }
}
@media (max-width: 720px) {
  .ticket-ws.panel { z-index: 40; inset: 0; width: auto; height: 100dvh; border-radius: 0; border: 0; background: var(--canvas); }
  .panel .ws-scroll { padding: 16px 18px 24px; }
  .ws-composer { border-radius: 0; padding: 8px 12px calc(8px + env(safe-area-inset-bottom)); background: var(--surface-raised); }
  @media (prefers-reduced-motion: no-preference) { .ticket-ws.panel { animation-name: sheet-in; } @keyframes sheet-in { from { transform: translateY(24px); opacity: 0; } to { transform: none; opacity: 1; } } }
}
</style>
