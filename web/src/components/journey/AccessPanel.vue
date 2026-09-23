<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { useJourney } from '../../stores/journey'
import HandoffCard from './HandoffCard.vue'
const store = useJourney()
</script>
<template><div class="j-columns"><div class="j-scroll j-stack"><section class="j-card"><div class="eyebrow">Janus · permit</div><template v-if="store.journey?.stages.find(s => s.key === 'access')?.state === 'skipped'"><h2>Not needed in this release</h2><p>No explicit access change. The existing permit stays.</p></template><template v-else><h2>Bounded access</h2><p>A person approves the permit before Janus applies access. Authorization and credential readiness come from observed results.</p><p v-if="!store.handoffs.some(h => h.stage === 'access')" class="j-note">No access operation has been requested yet.</p></template></section><HandoffCard v-for="handoff in store.handoffs.filter(h => h.stage === 'access')" :key="handoff.id" :handoff="handoff" /></div><div class="j-scroll"><slot /></div></div></template>
