<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import { createView, updateView, deleteView, type SavedView, type ViewWrite } from '../lib/api'
import { useSession } from '../stores/session'
const props = defineProps<{ views: SavedView[]; configuration: Omit<ViewWrite, 'name' | 'shared'> }>()
const emit = defineEmits<{ apply: [view: SavedView]; changed: [] }>()
const session = useSession()
const selected = ref('')
const name = ref('')
const shared = ref(false)
const busy = ref(false)
const error = ref('')
const current = computed(() => props.views.find(view => view.id === selected.value))
const owned = computed(() => current.value?.owner_principal_id === session.identity?.principal.id)
function apply() {
  if (!current.value) return
  name.value = current.value.name; shared.value = current.value.shared
  emit('apply', current.value)
}
async function save(replace = false) {
  if (!name.value.trim()) return
  busy.value = true; error.value = ''
  try {
    const value = { ...props.configuration, name: name.value.trim(), shared: shared.value }
    const saved = replace && current.value ? await updateView(current.value.id, value) : await createView(value)
    selected.value = saved.id
    emit('changed')
  } catch (e) { error.value = e instanceof Error ? e.message : 'Could not save view' }
  finally { busy.value = false }
}
async function remove() {
  if (!current.value || !window.confirm(`Delete saved view “${current.value.name}”?`)) return
  busy.value = true; error.value = ''
  try { await deleteView(current.value.id); selected.value = ''; name.value = ''; emit('changed') }
  catch (e) { error.value = e instanceof Error ? e.message : 'Could not delete view' }
  finally { busy.value = false }
}
</script>
<template>
  <details class="saved-views">
    <summary>Saved views</summary>
    <div class="saved-content">
      <label>Open a view<select v-model="selected" :disabled="busy" @change="apply"><option value="">Choose a saved view</option><option v-for="view in views" :key="view.id" :value="view.id">{{ view.name }}{{ view.shared ? ' (shared)' : '' }}</option></select></label>
      <form @submit.prevent="save()">
        <label>View name<input v-model="name" required :disabled="busy" placeholder="Name this list" /></label>
        <label class="check"><input v-model="shared" type="checkbox" :disabled="busy" />Share with workspace</label>
        <div class="saved-actions"><button class="quiet" :disabled="busy || !name.trim()">Save as new view</button><button v-if="owned" type="button" class="quiet" :disabled="busy || !name.trim()" @click="save(true)">Update view</button><button v-if="owned" type="button" class="quiet" :disabled="busy" @click="remove">Delete view</button></div>
      </form>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
    </div>
  </details>
</template>
<style scoped>
.saved-views { border-top: 1px solid var(--line); padding: 0 20px; }
summary { cursor: pointer; min-height: 44px; padding: 12px 0; font-size: 13px; }
.saved-content, form, label { display: grid; gap: 8px; }
.saved-content { padding-bottom: 18px; gap: 16px; }
.saved-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.check { display: flex; gap: 8px; align-items: center; }
</style>
