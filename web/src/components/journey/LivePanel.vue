<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import { getReleaseHistory } from '../../lib/journey'
import type { WorkNode } from '../../lib/api'
import ReleaseVersion from './ReleaseVersion.vue'
import { useJourney } from '../../stores/journey'
import HandoffCard from './HandoffCard.vue'
const store = useJourney()
const history = ref<WorkNode[]>([]), error = ref(''), loading = ref(false)
let generation = 0
async function loadHistory() {
  const request = ++generation
  loading.value = true; error.value = ''
  try { const value = await getReleaseHistory(store.projectId); if (request === generation) history.value = value }
  catch { if (request === generation) error.value = 'Release history could not be loaded.' }
  finally { if (request === generation) loading.value = false }
}
watch(() => store.projectId, () => { history.value = []; void loadHistory() }, { immediate: true })
onBeforeUnmount(() => { generation++ })
defineEmits<{ walk: [ticket?: string]; openNode: [id: string] }>()
</script>
<template><div class="j-columns"><div class="j-scroll j-stack"><section class="j-card"><div class="eyebrow">{{ store.journey?.stages.find(s => s.key === 'live')?.state === 'done' || store.journey?.stage === 'live' ? 'Live' : 'Release history' }}</div><h2>{{ store.release?.title || 'Releases' }}</h2><p v-if="store.walker?.state === 'released'">This release is live.</p><p v-else>Earlier releases keep their history when a new plan starts.</p><button v-if="store.release" class="j-button" @click="$emit('openNode',store.release.id)">Open release record</button><button v-for="ticket in store.selected" :key="ticket.ticket_node_id" class="live-ticket" @click="$emit('walk',ticket.ticket_node_id)"><span class="j-meta">{{ ticket.key }}</span>{{ ticket.title }}</button></section><section class="j-card"><h2>Release history</h2><p v-if="loading" role="status">Loading releases…</p><p v-else-if="error" class="error" role="alert">{{ error }} <button class="j-button" @click="loadHistory">Retry history</button></p><p v-else-if="!history.length">No release records available.</p><article v-for="record in history" :key="record.id" class="release-record"><button class="j-text" @click="$emit('openNode',record.id)">{{ record.key }} · {{ record.title }}</button><span class="j-meta">{{ record.state }}</span><ReleaseVersion v-if="typeof record.fields.version === 'string' && typeof record.fields.version_scheme === 'string'" :version="record.fields.version" :scheme="record.fields.version_scheme" /></article></section><HandoffCard v-for="handoff in store.handoffs" :key="handoff.id" :handoff="handoff" /></div><div class="j-scroll"><section class="j-card"><div class="eyebrow">Next release</div><h2>{{ store.tickets.filter(t => !t.included).length }} tickets in the backlog</h2><p>The next release starts from a new plan. Prior release evidence stays recorded.</p><ul><li v-for="ticket in store.tickets.filter(t => !t.included)" :key="ticket.ticket_node_id">{{ ticket.key }} · {{ ticket.title }}</li></ul></section><slot /></div></div></template>
<style scoped>.release-record { display:flex; flex-wrap:wrap; gap:12px; align-items:center; padding:12px 0; border-top:1px solid var(--line); }.release-record .j-text { flex:1; }.live-ticket { display:flex; gap:14px; width:100%; text-align:left; border:0; border-top:1px solid var(--line); background:none; padding:12px 0; font-size:13px; }.live-ticket:hover { color:var(--teal); }</style>
