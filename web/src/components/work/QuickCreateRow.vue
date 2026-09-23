<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script lang="ts">
export interface QuickDraft { title: string; kind: string; state: string; priority: string; epic: { id: string; key: string; title: string } | null }
</script>
<script setup lang="ts">
import { nextTick, onMounted, reactive, ref } from 'vue'
import { kindLabel, priorityLabel, statusMeta } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import EpicPicker from './EpicPicker.vue'
import OptionMenu from './OptionMenu.vue'
import PriorityIcon from './PriorityIcon.vue'
import StatusIcon from './StatusIcon.vue'
import StatusMenu from './StatusMenu.vue'

// "New ticket" at the top of the table: type a title, Tab through type,
// status, priority and epic, Enter creates and the row stays for the next one.
const props = defineProps<{ projectId: string; knownStates: string[]; showAssignee: boolean; create: (draft: QuickDraft) => Promise<boolean> }>()
const emit = defineEmits<{ close: [] }>()
const draft = reactive<QuickDraft>({ title: '', kind: 'ticket', state: 'new', priority: '', epic: null })
const busy = ref(false)
const input = ref<HTMLInputElement>()
const menu = ref<{ kind: 'type' | 'status' | 'priority' | 'epic'; anchor: HTMLElement } | null>(null)
const kindOptions = [{ value: 'ticket', label: 'Ticket' }, { value: 'task', label: 'Task' }, { value: 'epic', label: 'Epic' }]
const priorityOptions = [{ value: 'high', label: 'High' }, { value: 'medium', label: 'Medium' }, { value: 'low', label: 'Low' }, { value: '', label: 'No priority' }]

async function submit() {
  if (!draft.title.trim() || busy.value) { input.value?.focus(); return }
  busy.value = true
  const ok = await props.create({ ...draft, title: draft.title.trim() })
  busy.value = false
  if (ok) draft.title = ''
  await nextTick(); input.value?.focus()
}
function open(kind: 'type' | 'status' | 'priority' | 'epic', event: MouseEvent) { menu.value = { kind, anchor: event.currentTarget as HTMLElement } }
function close(restore: boolean) { const anchor = menu.value?.anchor; menu.value = null; if (restore) anchor?.focus() }
function choose(apply: () => void) { const anchor = menu.value?.anchor; apply(); menu.value = null; anchor?.focus() }
function keydown(event: KeyboardEvent) {
  if (menu.value) return
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); emit('close') }
  else if (event.key === 'Enter' && event.target === input.value) { event.preventDefault(); void submit() }
}
onMounted(() => input.value?.focus())
defineExpose({ focus: () => input.value?.focus(), isDirty: () => !!draft.title.trim() })
</script>

<template>
  <tr class="create-row" @keydown="keydown">
    <td class="c-key"><div class="cell"><span class="new-badge">New</span></div></td>
    <td class="c-title">
      <div class="cell create-title">
        <input ref="input" v-model="draft.title" class="create-input" :placeholder="`${kindLabel(draft.kind)} title`" aria-label="New ticket title" :disabled="busy" />
        <button type="button" class="create-chip" aria-haspopup="menu" :aria-label="`Type: ${kindLabel(draft.kind)}`" @click="open('type', $event)">
          <AppIcon :name="draft.kind === 'epic' ? 'epic' : draft.kind === 'task' ? 'task' : 'ticket'" :size="12" :class="['kind', draft.kind]" />{{ kindLabel(draft.kind) }}<AppIcon name="chevron" :size="11" class="chev" />
        </button>
      </div>
    </td>
    <td class="c-status">
      <div class="cell"><button type="button" class="create-chip" aria-haspopup="menu" :aria-label="`Status: ${statusMeta(draft.state).label}`" @click="open('status', $event)"><StatusIcon :state="draft.state" :size="12" />{{ statusMeta(draft.state).label }}<AppIcon name="chevron" :size="11" class="chev" /></button></div>
    </td>
    <td class="c-prio">
      <div class="cell"><button type="button" class="create-chip" aria-haspopup="menu" :aria-label="`Priority: ${priorityLabel(draft.priority)}`" @click="open('priority', $event)"><PriorityIcon v-if="draft.priority" :priority="draft.priority" :size="12" /><span v-else class="dash">—</span><span class="chip-text" :class="{ unset: !draft.priority }">{{ draft.priority ? priorityLabel(draft.priority) : 'No priority' }}</span><AppIcon name="chevron" :size="11" class="chev" /></button></div>
    </td>
    <td class="c-epic" :colspan="showAssignee ? 2 : 1">
      <div class="cell epic-cell"><button type="button" class="create-chip epic-chip" aria-haspopup="dialog" :aria-label="`Epic: ${draft.epic ? draft.epic.title : 'none'}`" :data-tip="draft.epic ? `${draft.epic.key}\n${draft.epic.title}` : 'Choose an epic'" @click="open('epic', $event)"><AppIcon name="epic" :size="12" class="kind" :class="{ epic: !!draft.epic }" /><span class="chip-text" :class="{ unset: !draft.epic }">{{ draft.epic ? draft.epic.title : 'No epic' }}</span></button></div>
    </td>
  </tr>
  <tr class="create-hint-row" aria-hidden="true">
    <td :colspan="showAssignee ? 6 : 5"><span class="create-hint"><kbd class="keycap"><AppIcon name="enter" /></kbd> create and keep going · <kbd class="keycap">tab</kbd> type, status, priority, epic · <kbd class="keycap">esc</kbd> close</span></td>
  </tr>
  <OptionMenu v-if="menu?.kind === 'type'" :anchor="menu.anchor" title="Type" subject="the new ticket" kind="type" :options="kindOptions" :current="draft.kind" @choose="value => choose(() => { draft.kind = value })" @close="close" />
  <StatusMenu v-if="menu?.kind === 'status'" :anchor="menu.anchor" :current="draft.state" :known-states="knownStates" ticket-key="the new ticket" @choose="value => choose(() => { draft.state = value })" @close="close" />
  <OptionMenu v-if="menu?.kind === 'priority'" :anchor="menu.anchor" title="Priority" subject="the new ticket" kind="priority" :options="priorityOptions" :current="draft.priority" @choose="value => choose(() => { draft.priority = value })" @close="close" />
  <EpicPicker v-if="menu?.kind === 'epic'" :anchor="menu.anchor" :project-id="projectId" :current="draft.epic?.id ?? null" subject="the new ticket" allow-none @choose="epic => choose(() => { draft.epic = epic })" @close="close" />
</template>

<style scoped>
.create-row td { height: 44px; padding: 0 12px; background: var(--row-selected); border-bottom: 0; vertical-align: middle; }
.create-row td:first-child { padding-left: 18px; box-shadow: inset 3px 0 0 var(--row-accent); }
.cell { display: flex; align-items: center; gap: 8px; min-width: 0; }
.new-badge { display: inline-flex; align-items: center; height: 20px; padding: 0 8px; border-radius: 6px; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); font: 600 10.5px/1 var(--mono); letter-spacing: .1em; text-transform: uppercase; font-variant-ligatures: none; }
.create-title { gap: 10px; }
.create-input { flex: 1; min-width: 0; height: 30px; padding: 0 10px; border: 1px solid var(--glass-edge); border-radius: 8px; background: var(--field-bg); color: var(--ink); box-shadow: var(--field-inset), 0 0 0 1px var(--line); font-size: 13.5px; }
.create-input:focus { box-shadow: var(--focus-ring); }
.create-chip { display: inline-flex; align-items: center; gap: 6px; max-width: 100%; height: 26px; padding: 0 8px 0 7px; border: 0; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); color: var(--ink); font-size: 12.5px; white-space: nowrap; }
@media (hover: hover) { .create-chip:hover { box-shadow: inset 0 0 0 1px var(--glass-rim); } }
.create-chip:focus-visible { box-shadow: var(--focus-ring); }
.chip-text { overflow: hidden; text-overflow: ellipsis; }
.chip-text.unset { color: var(--ink-3); }
.chev, .dash { color: var(--ink-3); }
.kind { color: var(--ink-3); }
.kind.epic { color: var(--gold); }
.epic-cell { justify-content: flex-end; }
.epic-chip { max-width: 180px; }
.create-hint-row td { padding: 0 18px 8px; background: var(--row-selected); border-bottom: 1px solid var(--line-2); }
.create-hint { display: inline-flex; align-items: center; flex-wrap: wrap; gap: 4px; font-size: 11.5px; color: var(--ink-3); }
@media (max-width: 720px) {
  .create-row { display: flex; flex-wrap: wrap; gap: 8px; padding: 12px 14px 14px; background: var(--row-selected); box-shadow: inset 3px 0 0 var(--row-accent); border-bottom: 1px solid var(--line-2); }
  .create-row td { display: block; height: auto; padding: 0 !important; background: none; box-shadow: none !important; }
  .c-key { display: none !important; }
  .c-title { flex: 1 0 100%; }
  .epic-chip { max-width: 140px; }
  .create-input { height: 44px; font-size: 16px; }
  .create-chip { height: 36px; }
  .epic-cell { justify-content: flex-start; }
  .create-hint-row { display: none; }
}
</style>
