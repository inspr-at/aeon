<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import type { WorkNode } from '../../lib/api'
import type { WalkerTicket, WalkerFeature } from '../../lib/journey'
import MarkdownBody from '../MarkdownBody.vue'
defineProps<{ ticket: WalkerTicket; node?: WorkNode; feature?: WalkerFeature; loading?: boolean; error?: string }>()
defineEmits<{ openNode: [id: string] }>()
</script>
<template><aside class="ticket-details" aria-label="Ticket details"><div class="j-card-head"><span class="j-meta">{{ ticket.key }}</span><button class="j-text" @click="$emit('openNode', ticket.ticket_node_id)">Open ticket</button></div><h2>{{ ticket.title }}</h2><p class="j-meta">{{ ticket.included ? 'In release' : 'Backlog' }} · {{ ticket.estimated_hours == null ? 'Not estimated' : `${ticket.estimated_hours} h estimate` }}</p>
  <div class="j-card-head"><span class="eyebrow">Feature</span><button v-if="feature" class="j-text j-meta" @click="$emit('openNode', feature.feature_node_id)">{{ feature.epic_key }}</button></div><p>{{ feature?.title || 'Added, not tied to a feature' }}</p>
  <p v-if="loading" role="status">Loading ticket…</p><p v-if="error" role="alert" class="error">{{ error }}</p>
  <template v-if="node"><div class="eyebrow">Recorded state · {{ node.state }}</div><MarkdownBody :body="node.body" /><div class="eyebrow">Evidence</div><ul v-if="Array.isArray(node.fields.evidence) && node.fields.evidence.length"><li v-for="(evidence,index) in node.fields.evidence" :key="index">{{ typeof evidence === 'string' ? evidence : JSON.stringify(evidence) }}</li></ul><p v-else class="j-note">No evidence recorded on this ticket.</p></template>
</aside></template>
<style scoped>.ticket-details { padding:20px; overflow:auto; min-width:0; background:var(--glass); border-left:1px solid var(--line); font-size:13px; }.ticket-details h2 { font-size:19px; margin:8px 0; }.ticket-details .eyebrow { margin-top:22px; }.ticket-details .j-card-head { align-items:baseline; }.ticket-details ul { padding-left:18px; overflow-wrap:anywhere; }</style>
