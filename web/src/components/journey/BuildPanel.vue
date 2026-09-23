<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount } from 'vue'
import { getNode, type WorkNode } from '../../lib/api'
import { useJourney } from '../../stores/journey'
import JourneyIcon from './JourneyIcon.vue'
defineProps<{ canEdit: boolean }>()
const rejection = ref('')
const store = useJourney()
const observed = ref<WorkNode[]>([]), loading = ref(false), error = ref('')
const done = computed(() => observed.value.filter(node => node.state === 'done').length)
let generation = 0
async function refresh() {
  const request = ++generation, selected = [...store.selected]
  loading.value = true; error.value = ''
  const nodes: WorkNode[] = []
  let failures = 0
  // Keep large plans from opening unbounded parallel requests.
  for (let i = 0; i < selected.length; i += 8) {
    const batch = await Promise.allSettled(selected.slice(i,i+8).map(ticket => getNode(ticket.ticket_node_id)))
    if (request !== generation) return
    for (const result of batch) { if (result.status === 'fulfilled') nodes.push(result.value); else failures++ }
  }
  if (request !== generation) return
  observed.value = nodes; loading.value = false
  if (failures) error.value = `${failures} ticket states could not be loaded.`
}
watch(() => store.walker, () => { observed.value = []; void refresh() }, { immediate: true })
onBeforeUnmount(() => { generation++ })
defineEmits<{ walk: [ticket?: string] }>()
</script>
<template><div class="j-columns"><div class="j-scroll"><section class="j-card"><div class="j-card-head"><div><div class="eyebrow">Paimos · {{ store.walker?.state || 'Waiting' }}</div><h2>{{ store.release?.title || 'Build' }}</h2></div><button class="j-button" :disabled="!store.walker" @click="$emit('walk')"><JourneyIcon name="expand" />Full screen</button></div><p v-if="store.walker?.state === 'building'">The crew is building. You review the candidate when the recorded work is ready.</p><div v-if="store.selected.length" class="build-progress" aria-label="Observed build progress"><progress v-if="!loading" :value="done" :max="store.selected.length" /><span v-if="!loading">{{ done }} of {{ store.selected.length }} recorded done</span><span v-if="loading" role="status">Loading ticket states…</span></div><p v-if="error" class="error" role="alert">{{ error }} <button class="j-button" @click="refresh">Retry states</button></p><p v-if="!store.selected.length">No tickets selected for this release.</p><button v-for="ticket in store.selected" :key="ticket.ticket_node_id" class="build-ticket" @click="$emit('walk',ticket.ticket_node_id)"><span class="j-meta">{{ ticket.key }}</span><span>{{ ticket.title }}</span><span class="j-meta">{{ observed.find(n => n.id === ticket.ticket_node_id)?.state || 'Not reported' }}</span><JourneyIcon name="right" /></button><p class="j-note">Open a ticket for its recorded state, evidence and linked screens.</p></section></div><div class="j-scroll j-stack"><slot /><form v-if="canEdit && store.walker?.state === 'candidate'" class="j-card" @submit.prevent="store.action('reject_candidate',rejection)"><h2>Send the candidate back</h2><label class="j-label">What should change?<textarea v-model="rejection" rows="3" required maxlength="2048" /></label><button class="j-button" :disabled="store.busy || store.loading || store.stale || !rejection.trim()">Reject candidate</button></form></div></div></template>
<style scoped>.build-progress { display:flex; flex-wrap:wrap; gap:12px; align-items:center; margin:16px 0; font-size:12px; color:var(--ink-2); }.build-progress progress { accent-color:var(--teal); height:8px; }.build-ticket { display:flex; gap:14px; align-items:center; width:100%; border:0; border-top:1px solid var(--line); background:none; padding:14px 0; text-align:left; font-size:13px; }.build-ticket span:nth-child(2) { flex:1; }.build-ticket:hover { color:var(--teal); }</style>
