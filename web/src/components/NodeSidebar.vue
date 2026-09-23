<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { APIError, getNode, updateNode, moveNode, deleteNode, type WorkNode } from '../lib/api'
import AppIcon from './AppIcon.vue'
import MarkdownBody from './MarkdownBody.vue'
const props = defineProps<{ nodeId: string }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const node = ref<WorkNode>()
const editing = ref(false)
const title = ref('')
const body = ref('')
const state = ref('')
const busy = ref(false)
const error = ref('')
const remoteChanged = ref(false)
const deleted = ref(false)
const moving = ref(false)
const parentId = ref('')
const beforeId = ref('')
let generation = 0
const dirty = computed(() => editing.value && !!node.value && (title.value !== node.value.title || body.value !== node.value.body || state.value !== node.value.state))
function draft(value: WorkNode) { node.value = value; title.value = value.title; body.value = value.body; state.value = value.state; remoteChanged.value = false }
async function refresh() {
  if (busy.value) return
  const request = ++generation
  try {
    const value = await getNode(props.nodeId)
    if (request !== generation) return
    deleted.value = false
    if (dirty.value) remoteChanged.value = value.updated_at !== node.value?.updated_at
    else draft(value)
    error.value = ''
  } catch (e) {
    if (request !== generation) return
    deleted.value = e instanceof APIError && e.status === 404
    error.value = deleted.value ? 'This node is no longer available. Your draft is kept here for copying.' : e instanceof Error ? e.message : 'Could not load this node'
  }
}
function canLeave() { return !busy.value && (!dirty.value || window.confirm('Discard your unsaved changes?')) }
function close() { if (canLeave()) emit('close') }
function cancel() { if (canLeave() && node.value) { editing.value = false; draft(node.value); void refresh() } }
async function save() {
  if (!node.value || !title.value.trim() || !state.value.trim()) return
  generation++; busy.value = true; error.value = ''
  try {
    // R1 has no conditional-write token. Check for an already-observed conflict
    // before saving; never silently overwrite a draft when SSE refreshes it.
    const latest = await getNode(props.nodeId)
    if (latest.updated_at !== node.value.updated_at) {
      remoteChanged.value = true
      error.value = 'This node changed while you were editing. Copy your draft, then cancel to load the latest version.'
      return
    }
    draft(await updateNode(props.nodeId, { title: title.value.trim(), body: body.value, state: state.value.trim() }))
    editing.value = false
    emit('changed')
  } catch (e) { error.value = e instanceof Error ? e.message : 'Save failed; your draft is kept.' }
  finally { busy.value = false }
}
async function move() {
  generation++; busy.value = true; error.value = ''
  try {
    draft(await moveNode(props.nodeId, parentId.value.trim() || null, beforeId.value.trim() || null))
    moving.value = false; emit('changed')
  } catch (e) { error.value = e instanceof Error ? e.message : 'Could not move node' }
  finally { busy.value = false }
}
async function remove() {
  if (!window.confirm(`Delete ${node.value?.key}? Nodes with children cannot be deleted.`)) return
  generation++; busy.value = true; error.value = ''
  try { await deleteNode(props.nodeId); emit('changed'); emit('close') }
  catch (e) { error.value = e instanceof Error ? e.message : 'Could not delete node' }
  finally { busy.value = false }
}
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value) { event.preventDefault(); event.returnValue = '' } }
watch(() => props.nodeId, () => { node.value = undefined; editing.value = false; moving.value = false; deleted.value = false; remoteChanged.value = false; void refresh() }, { immediate: true })
onMounted(() => window.addEventListener('beforeunload', beforeUnload))
onBeforeUnmount(() => { generation++; window.removeEventListener('beforeunload', beforeUnload) })
defineExpose({ canLeave, refresh })
</script>
<template>
  <aside class="node-sidebar" aria-label="Node details" :aria-busy="busy">
    <header><span class="node-key">{{ node?.key || 'Node details' }}</span><button class="icon-button" aria-label="Close node details" :disabled="busy" @click="close"><AppIcon name="close" /></button></header>
    <p v-if="error" class="error" role="alert">{{ error }} <button v-if="!deleted" class="quiet" :disabled="busy" @click="refresh">Retry details</button></p>
    <p v-if="remoteChanged" class="notice" role="status">Updated elsewhere. Your draft has been preserved. Cancel editing to load the latest version.</p>
    <template v-if="node">
      <form v-if="editing" @submit.prevent="save">
        <label>Title<input v-model="title" required :disabled="busy" /></label>
        <label>State<input v-model="state" required :disabled="busy" /></label>
        <label>Markdown<textarea v-model="body" rows="16" :disabled="busy" /></label>
        <div class="sidebar-actions"><button class="button" :disabled="busy || deleted || remoteChanged || !title.trim() || !state.trim()">{{ busy ? 'Saving…' : 'Save changes' }}</button><button class="quiet" type="button" :disabled="busy" @click="cancel">Cancel editing</button></div>
        <details><summary>Preview Markdown</summary><MarkdownBody :body="body" /></details>
      </form>
      <template v-else>
        <h2>{{ node.title }}</h2><p class="node-state">{{ node.state }}</p>
        <div class="sidebar-actions"><button class="quiet" :disabled="deleted || busy" @click="editing = true; moving = false"><AppIcon name="edit" />Edit node</button><button class="quiet" :disabled="deleted || busy" @click="moving = !moving; parentId = node.parent_id || ''; beforeId = ''">Move node</button><button class="quiet" :disabled="deleted || busy" @click="remove">Delete node</button></div>
        <form v-if="moving" class="move-form" @submit.prevent="move"><label>New parent ID<input v-model="parentId" placeholder="Leave empty for a root node" :disabled="busy" /></label><label>Before sibling ID<input v-model="beforeId" placeholder="Leave empty to append" :disabled="busy" /></label><button class="button" :disabled="busy">{{ busy ? 'Moving…' : 'Confirm move' }}</button></form>
        <MarkdownBody v-if="node.body" :body="node.body" /><p v-else class="empty-body">No description yet. Add Markdown to give this work context.</p>
      </template>
    </template>
    <p v-else-if="!error" role="status">Loading node…</p>
  </aside>
</template>
<style scoped>
.node-sidebar { border-left: 1px solid var(--line); padding: 24px; background: var(--surface); overflow: auto; min-width: 0; }
header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 18px; }
h2 { font-size: 24px; font-weight: 500; line-height: 1.3; overflow-wrap: anywhere; }
.node-state { margin: 8px 0; }
.markdown-body, .empty-body { margin-top: 24px; }
form, label { display: grid; gap: 8px; }
form { gap: 16px; }
textarea { font: 13px/1.6 var(--mono); resize: vertical; }
.move-form { margin-top: 16px; }
.sidebar-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.error, .notice { margin-bottom: 16px; }
.notice { color: var(--warn); }
summary { cursor: pointer; min-height: 44px; padding: 10px 0; }
@media (max-width: 900px) { .node-sidebar { position: fixed; inset: var(--header-h) 0 0; z-index: 4; border-left: 0; } }
</style>
