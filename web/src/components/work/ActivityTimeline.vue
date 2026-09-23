<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { commentEditable, describeChange, type TimelineEntry } from '../../lib/activity'
import { confirmAction } from '../../lib/confirm'
import { absoluteTime, relativeTime } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import MarkdownBody from '../MarkdownBody.vue'
import MarkdownEditor from './MarkdownEditor.vue'
import PersonAvatar from './PersonAvatar.vue'
import StatusIcon from './StatusIcon.vue'

// The ticket's history, oldest first: comments as readable cards, field
// changes as one compact line per person and moment.
const props = defineProps<{
  entries: TimelineEntry[]; loading: boolean; loadingOlder: boolean; hasOlder: boolean; error: string
  me: string | undefined; now: number; canWrite: boolean
  edit: (id: string, body: string) => Promise<boolean>; remove: (id: string) => Promise<boolean>
}>()
const emit = defineEmits<{ older: []; retry: [] }>()
const editingId = ref<string | null>(null)
const draft = ref('')
const saving = ref(false)
const editor = ref<InstanceType<typeof MarkdownEditor>[]>()

async function startEdit(entry: Extract<TimelineEntry, { kind: 'comment' }>) {
  editingId.value = entry.id; draft.value = entry.body
  await nextTick(); editor.value?.[0]?.focus()
}
async function saveEdit() {
  if (!editingId.value) return
  saving.value = true
  const ok = await props.edit(editingId.value, draft.value)
  saving.value = false
  if (ok) editingId.value = null
}
async function cancelEdit(original: string) {
  if (draft.value !== original && !(await confirmAction({ title: 'Discard your comment changes?', confirmLabel: 'Discard', danger: true }))) return
  editingId.value = null
}
async function removeComment(id: string) {
  if (await confirmAction({ title: 'Delete this comment?', body: 'It leaves the timeline for everyone. The audit log keeps a record.', confirmLabel: 'Delete comment', danger: true })) await props.remove(id)
}
function isDirty() { return editingId.value !== null }
defineExpose({ isDirty })
</script>

<template>
  <section class="activity" aria-label="Activity">
    <h3 class="eyebrow">Activity</h3>
    <button v-if="hasOlder" type="button" class="older" :disabled="loadingOlder" @click="emit('older')">
      <AppIcon name="chevron-up" :size="13" />{{ loadingOlder ? 'Loading older activity…' : 'Show older activity' }}
    </button>
    <div v-if="loading && !entries.length" class="activity-skeleton" aria-hidden="true">
      <span class="skeleton" /><span class="skeleton short" /><span class="skeleton" />
    </div>
    <p v-else-if="error" class="activity-error" role="alert">Activity could not be loaded. <button type="button" class="btn sm" @click="emit('retry')">Try again</button></p>
    <ol v-else class="timeline">
      <li v-for="entry in entries" :key="entry.id" class="entry" :class="entry.kind">
        <template v-if="entry.kind === 'comment'">
          <PersonAvatar :name="entry.author.name" :size="26" class="avatar" />
          <div class="comment">
            <header class="comment-head">
              <strong>{{ entry.author.name }}</strong>
              <time :datetime="entry.at" :data-tip="absoluteTime(entry.at)">{{ relativeTime(entry.at, { now, long: true }) }}</time>
              <span v-if="canWrite && commentEditable(entry, me, now) && editingId !== entry.id" class="comment-actions">
                <button type="button" class="icon-btn sm flat" aria-label="Edit comment" data-tip="Edit · within 15 minutes" @click="startEdit(entry)"><AppIcon name="edit" :size="13" /></button>
                <button type="button" class="icon-btn sm flat danger-icon" aria-label="Delete comment" data-tip="Delete" @click="removeComment(entry.id)"><AppIcon name="trash" :size="13" /></button>
              </span>
            </header>
            <MarkdownEditor v-if="editingId === entry.id" ref="editor" v-model="draft" label="Comment" compact :min-rows="3" :saving="saving" @save="saveEdit" @cancel="cancelEdit(entry.body)" />
            <MarkdownBody v-else :body="entry.body" class="comment-body" />
          </div>
        </template>
        <template v-else-if="entry.kind === 'changes'">
          <span class="dot" aria-hidden="true" />
          <p class="change-line">
            <strong>{{ entry.author.name }}</strong>
            <template v-for="(change, index) in entry.changes" :key="change.field">
              <span v-if="index > 0" class="sep">,</span>
              {{ ' ' }}{{ describeChange(change).label }}
              <template v-if="describeChange(change).from">
                <span class="value"><StatusIcon v-if="change.field === 'status' && change.from" :state="change.from" :size="11" />{{ describeChange(change).from }}</span>
                <AppIcon name="arrow" :size="11" class="arrow" /><span class="sr-only"> to </span>
              </template>
              <span v-if="describeChange(change).to" class="value"><StatusIcon v-if="change.field === 'status' && change.to" :state="change.to" :size="11" />{{ describeChange(change).to }}</span>
            </template>
            <span class="sep">·</span>
            <time :datetime="entry.at" :data-tip="absoluteTime(entry.at)">{{ relativeTime(entry.at, { now }) }}</time>
          </p>
        </template>
        <template v-else>
          <span class="dot created" aria-hidden="true" />
          <p class="change-line"><strong>{{ entry.author.name }}</strong> created this <span class="sep">·</span> <time :datetime="entry.at" :data-tip="absoluteTime(entry.at)">{{ relativeTime(entry.at, { now }) }}</time></p>
        </template>
      </li>
      <li v-if="!entries.length && !loading" class="entry empty"><p class="change-line">No activity yet.</p></li>
    </ol>
  </section>
</template>

<style scoped>
.activity .eyebrow { margin: 0 0 10px; }
.older { display: inline-flex; align-items: center; gap: 6px; height: 28px; margin: 0 0 8px -8px; padding: 0 10px; border: 0; border-radius: 999px; background: transparent; color: var(--teal-ink); font-size: 12.5px; font-weight: 600; }
.older:hover { background: var(--row-hover); }
.older:focus-visible { box-shadow: var(--focus-ring); }
.timeline { position: relative; margin: 0; padding: 0; list-style: none; }
.timeline::before { content: ''; position: absolute; left: 12px; top: 6px; bottom: 6px; width: 1px; background: var(--line); }
.entry { position: relative; display: flex; gap: 12px; }
.entry + .entry { margin-top: 10px; }
.entry.comment + .entry.comment { margin-top: 14px; }
.avatar { position: relative; z-index: 1; margin-top: 2px; box-shadow: 0 0 0 3px var(--surface-raised), 0 0 0 4px var(--glass-rim); }
.comment { flex: 1; min-width: 0; padding: 10px 14px 4px; border-radius: 12px; background: var(--surface-sunken); box-shadow: inset 0 0 0 1px var(--line); }
.comment-head { display: flex; align-items: center; gap: 8px; min-height: 24px; font-size: 12.5px; }
.comment-head strong { color: var(--ink); font-weight: 650; }
.comment-head time { color: var(--ink-3); }
.comment-actions { display: inline-flex; gap: 2px; margin-left: auto; }
.comment-actions .icon-btn { width: 24px; height: 24px; color: var(--ink-3); }
.danger-icon:hover { color: var(--danger) !important; }
.comment-body { margin-top: 4px; font-size: 13.5px; }
.comment .md-editor { margin: 6px 0 10px; }
.dot { position: relative; z-index: 1; flex-shrink: 0; width: 25px; display: grid; place-items: center; }
.dot::after { content: ''; width: 7px; height: 7px; margin-top: 7px; border-radius: 50%; background: var(--surface-raised); box-shadow: inset 0 0 0 1.5px var(--line-2), 0 0 0 3px var(--surface-raised); }
.dot.created::after { background: var(--aqua); box-shadow: 0 0 0 3px var(--surface-raised); }
.change-line { display: flex; align-items: center; flex-wrap: wrap; gap: 0 4px; min-height: 22px; font-size: 12.5px; color: var(--ink-2); }
.change-line strong { color: var(--ink); font-weight: 600; }
.value { display: inline-flex; align-items: center; gap: 4px; color: var(--ink); }
.arrow { color: var(--ink-3); }
.sep { color: var(--ink-3); }
.change-line time { color: var(--ink-3); }
.empty { padding-left: 37px; }
.activity-skeleton { display: grid; gap: 12px; padding: 6px 0; }
.activity-skeleton .short { width: 55%; }
.activity-error { display: flex; align-items: center; gap: 8px; font-size: 13px; color: var(--danger); }
</style>
