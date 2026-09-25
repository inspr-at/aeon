<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { STAGE_LABEL, type Stage } from '../../lib/journey'
import { useJourneyContext } from '../../lib/journeyContext'
import AppIcon from '../AppIcon.vue'
import GateCard from './GateCard.vue'

// A stage the journey has not reached: (the stage's own line is the subtitle
// above) what is known about it, and the way to where the journey is now, so a
// later stage is never a dead end.
const props = defineProps<{ stage: Stage; detail?: string; eyebrow?: string; title?: string }>()
const ctx = useJourneyContext()
const now = computed(() => ctx.journey.value.stage)
const action = computed(() => ctx.journey.value.next_action)
// "The journey is at Plan: open release 1 comes first." A running build is not a step to take.
const line = computed(() => action.value.key === 'wait_for_build' ? 'the crew is building the release.' : `${action.value.label.charAt(0).toLowerCase()}${action.value.label.slice(1)} comes first.`)
</script>

<template>
  <GateCard :eyebrow="props.eyebrow ?? 'Later'" :title="props.title ?? 'Not yet'" tone="record">
    <p v-if="props.detail">{{ props.detail }}</p>
    <p class="now">The journey is at <b>{{ STAGE_LABEL[now] }}</b>: {{ line }}</p>
    <button type="button" class="btn sm go" @click="ctx.view(now)">Go to {{ STAGE_LABEL[now] }}<AppIcon name="arrow" :size="13" /></button>
  </GateCard>
</template>

<style scoped>
.now { font-size: 13px; color: var(--ink-2); }
.now b { color: var(--ink); font-weight: 600; }
.go { justify-self: start; }
</style>
