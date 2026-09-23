<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { useJourney } from '../../stores/journey'
import { featureState } from '../../lib/journey'
import JourneyIcon from './JourneyIcon.vue'
defineProps<{ canEdit: boolean }>()
defineEmits<{ walk: [ticket?: string]; openNode: [id: string] }>()
const store = useJourney()
const hours = computed(() => store.selected.reduce((sum,t) => sum + (t.estimated_hours ?? 0),0))
const missing = computed(() => store.selected.filter(t => t.estimated_hours == null).length)
function reorder(id: string, direction: number) {
  const order = store.tickets.map(t => t.ticket_node_id), from = order.indexOf(id), to = from + direction
  if (to < 0 || to >= order.length) return
  order.splice(from,1); order.splice(to,0,id)
  void store.savePlan(store.selected.map(t => t.ticket_node_id), order)
}
</script>
<template><div class="j-columns"><div class="j-scroll"><section class="j-card">
  <div class="j-card-head"><div><div class="eyebrow">Tickets · release and backlog</div><h2>Choose what ships</h2></div><button class="j-button" :disabled="!store.walker" @click="$emit('walk')"><JourneyIcon name="expand" />Full screen</button></div>
  <p v-if="!store.walker">No release plan yet. Agree the requirements to create the first release.</p>
  <section v-for="group in store.groups" :key="group.id" class="feature-group"><div class="j-card-head">
    <div class="feature-title"><button v-if="group.tickets.length" role="checkbox" class="j-check" :aria-checked="featureState(group.tickets) === 'some' ? 'mixed' : featureState(group.tickets) === 'all'" :aria-label="`Select ${group.feature?.title || 'No feature'}`" :disabled="!canEdit || !store.planning" @click="store.toggleFeature(group.id)"><JourneyIcon v-if="featureState(group.tickets) === 'all'" name="check" /><JourneyIcon v-else-if="featureState(group.tickets) === 'some'" name="minus" /></button><b>{{ group.feature?.title || 'No feature' }}</b></div>
    <button v-if="group.feature" class="j-text j-meta" @click="$emit('openNode', group.id)">{{ group.feature.epic_key }}</button></div>
    <p v-if="!group.tickets.length" class="j-note">No tickets · no accepted breakdown yet.</p>
    <div v-for="ticket in group.tickets" :key="ticket.ticket_node_id" class="ticket-row" :class="{ deferred: !ticket.included }"><input type="checkbox" :checked="ticket.included" :disabled="!canEdit || !store.planning" :aria-label="`Include ${ticket.key}`" @click.prevent="store.toggleTicket(ticket.ticket_node_id)" /><button class="ticket-title" @click="$emit('walk',ticket.ticket_node_id)"><span class="j-meta">{{ ticket.key }}</span><span>{{ ticket.title }}</span></button><span class="j-meta estimate">{{ ticket.estimated_hours == null ? 'Unestimated' : `${ticket.estimated_hours} h` }}</span><div v-if="canEdit && store.walker?.state === 'planning'" class="reorder"><button class="j-icon" :disabled="!store.planning || store.tickets[0]?.ticket_node_id === ticket.ticket_node_id" :aria-label="`Move ${ticket.key} earlier`" @click="reorder(ticket.ticket_node_id,-1)"><JourneyIcon name="left" /></button><button class="j-icon" :disabled="!store.planning || store.tickets.at(-1)?.ticket_node_id === ticket.ticket_node_id" :aria-label="`Move ${ticket.key} later`" @click="reorder(ticket.ticket_node_id,1)"><JourneyIcon name="right" /></button></div></div>
  </section>
</section></div><div class="j-scroll j-stack"><section class="j-card"><div class="eyebrow">{{ store.walker?.state || 'Plan' }}</div><h2>{{ store.release?.title || 'Release plan' }}</h2><div class="plan-sum"><div><b>{{ store.selected.length }}</b><span>tickets</span></div><div><b>{{ hours }} h</b><span>known estimates</span></div><div><b>{{ store.tickets.length - store.selected.length }}</b><span>backlog</span></div></div><p v-if="missing">{{ missing }} selected tickets have no estimate.</p><p class="j-note">Estimates are guesses. The server checks the approved budget cap and current requirements before build.</p><ul class="coverage"><li v-for="group in store.groups.filter(g => g.feature && !g.tickets.some(t => t.included))" :key="group.id">{{ group.feature?.title }} has no ticket in this release.</li></ul></section><slot /></div></div></template>
<style scoped>
.feature-group { border-top:1px solid var(--line); padding:16px 0 4px; }.feature-title { display:flex; align-items:center; gap:10px; font-size:13px; min-width:0; }.ticket-row { display:flex; align-items:center; gap:10px; padding:9px 0; border-top:1px solid var(--line); }.ticket-title { flex:1; min-width:0; display:flex; gap:12px; text-align:left; border:0; background:none; font-size:13px; }.ticket-title .j-meta { flex:none; }.ticket-title:hover { color:var(--teal); }.deferred .ticket-title { color:var(--ink-2); }.estimate { white-space:nowrap; }.reorder { display:flex; }.reorder .j-icon { width:26px; height:28px; }.plan-sum { display:flex; gap:28px; margin:18px 0; }.plan-sum div { display:flex; flex-direction:column; }.plan-sum b { font-size:30px; font-weight:300; }.plan-sum span { font-size:11px; color:var(--ink-2); }.coverage { padding-left:18px; font-size:12px; color:var(--warn); }
</style>
