<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { ref } from 'vue'
import type { Action } from '../../lib/journey'
import { useJourney } from '../../stores/journey'
import MarkdownBody from '../MarkdownBody.vue'
defineProps<{ canEdit: boolean }>()
const store = useJourney(), tab = ref<'brief' | 'decision'>('brief'), outcome = ref<Action>('go'), reason = ref('')
async function record() { if (await store.action(outcome.value, reason.value)) reason.value = '' }
</script>
<template>
  <div class="j-columns"><div class="j-scroll"><section class="j-card">
    <div class="j-tabs" role="tablist" aria-label="Shape"><button class="j-button" role="tab" :aria-selected="tab === 'brief'" @click="tab = 'brief'">Brief</button><button class="j-button" role="tab" :aria-selected="tab === 'decision'" @click="tab = 'decision'">Decision</button></div>
    <template v-if="store.journey?.stages.find(s => s.key === 'shape')?.state === 'skipped'"><h2>Not needed on Personal</h2><p>Own time, own machine. The conversation goes straight to requirements.</p></template>
    <template v-else-if="tab === 'brief'"><h2>The brief</h2><template v-for="draft in store.intake.drafts.filter(d => d.kind === 'brief')" :key="draft.id"><p class="j-meta">{{ draft.status }} · {{ draft.title }}</p><MarkdownBody :body="draft.body" /></template><p v-if="!store.intake.drafts.some(d => d.kind === 'brief')">No cited brief yet. Continue the conversation in Inspire.</p><p class="j-note">Budget, users, scope and constraints belong in the brief. Your edits are preserved when new drafts arrive.</p></template>
    <template v-else><h2>A human scope decision</h2><p>Go when the approved scope fits the budget. Reduce scope, park, or drop while keeping the reason.</p>
      <form v-if="store.journey?.next_action.key === 'decide' && canEdit" @submit.prevent="record"><label class="j-label">Outcome<select v-model="outcome"><option value="go">Go · to requirements</option><option value="reduce_scope">Reduce scope</option><option value="park">Park · resume later</option><option value="drop">Drop · keep the reason</option></select></label><label class="j-label">Reason<textarea v-model="reason" required maxlength="2048" rows="3" /></label><button class="j-button" :disabled="store.busy || store.stale || !store.journey.next_action.available || !reason.trim()">Record decision</button><p v-if="store.journey.next_action.reason" class="j-note">{{ store.journey.next_action.reason }}</p></form>
      <p v-else class="j-note">{{ store.journey?.next_action.reason || 'The current decision is reflected in the journey below.' }}</p>
    </template>
  </section></div><div class="j-scroll"><slot /></div></div>
</template>
