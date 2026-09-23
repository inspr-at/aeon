<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { useJourney } from '../../stores/journey'
import HandoffCard from './HandoffCard.vue'
const store = useJourney()
</script>
<template><div class="j-columns"><div class="j-scroll j-stack"><section class="j-card"><div class="eyebrow">Pharos · deployment</div><h2>{{ store.release?.title || 'Deployment' }}</h2><p>Pharos checks the reviewed artifact, backup and readiness evidence before applying the release.</p><p v-if="!store.handoffs.some(h => h.stage === 'deploy')" class="j-note">No deployment has been requested yet.</p></section><HandoffCard v-for="handoff in store.handoffs.filter(h => h.stage === 'deploy')" :key="handoff.id" :handoff="handoff" /></div><div class="j-scroll"><slot /></div></div></template>
