<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { ref } from 'vue'
import { useJourney } from '../../stores/journey'
import type { Requirement } from '../../lib/journey'
const store = useJourney()
defineProps<{ canEdit: boolean }>()
defineEmits<{ openNode: [id: string] }>()
const title = ref(''), body = ref(''), kind = ref<Requirement['kind']>('functional')
async function add() { if (await store.createRequirement(kind.value, title.value.trim(), body.value)) { title.value = ''; body.value = '' } }
</script>
<template><div class="j-columns"><div class="j-scroll j-stack">
  <section v-for="category in (['functional','nonfunctional'] as const)" :key="category" class="j-card"><div class="j-card-head"><h2>{{ category === 'functional' ? 'Functional requirements' : 'Quality & constraints' }}</h2><span class="j-meta">rev {{ store.journey?.requirements_revision }}</span></div>
    <p v-if="!store.requirements.some(r => r.kind === category)">No {{ category }} requirements yet.</p>
    <article v-for="requirement in store.requirements.filter(r => r.kind === category)" :key="requirement.node_id" class="requirement"><div><button class="j-text" @click="$emit('openNode', requirement.node_id)">{{ requirement.title }}</button><p class="j-meta">{{ requirement.status }} · revision {{ requirement.revision }}</p></div>
      <template v-if="category === 'functional'"><p v-if="!requirement.generated_ticket_ids?.length" class="j-note">No tickets yet · Aithema breaks this feature down when you ask.</p><p v-else class="j-note">{{ requirement.generated_ticket_ids.length }} generated tickets</p><button v-if="requirement.feature_node_id" class="j-button" @click="$emit('openNode', requirement.feature_node_id)">Open feature</button></template><p v-else class="j-note">Becomes knowledge or checkable acceptance criteria.</p>
    </article>
  </section>
  <form v-if="canEdit" class="j-card" @submit.prevent="add"><h2>Add a draft requirement</h2><label class="j-label">Kind<select v-model="kind"><option value="functional">Functional</option><option value="nonfunctional">Nonfunctional</option></select></label><label class="j-label">Requirement<input v-model="title" required /></label><label class="j-label">Details<textarea v-model="body" rows="2" /></label><button class="j-button" :disabled="store.busy || store.stale || !title.trim()">Add requirement</button><p class="j-note">Changing agreed scope requires a fresh agreement before build.</p></form>
</div><div class="j-scroll"><slot /></div></div></template>
<style scoped>.requirement { border-top:1px solid var(--line); padding:14px 0; }.requirement .j-note { margin-top:8px; }</style>
