<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { stages, stageLabel, journeyPath, type Journey, type Stage } from '../../lib/journey'
import JourneyIcon from './JourneyIcon.vue'
const props = defineProps<{ journey: Journey; viewed: Stage; title: string; disabled: boolean }>()
defineEmits<{ action: [] }>()
const passive = computed(() => props.journey.next_action.key === 'wait_for_build')
const state = (key: Stage) => props.journey.stages.find(s => s.key === key)?.state ?? 'later'
</script>
<template>
  <nav class="journey-bar" aria-label="Project journey">
    <div class="journey-title"><span><b>{{ title }}</b> · {{ stageLabel(journey.stage) }}</span><span class="journey-hint" :title="journey.next_action.reason">{{ passive ? 'Now' : viewed === journey.next_action.stage ? 'Your call' : 'Next' }} · {{ journey.next_action.reason || journey.next_action.label }}</span></div>
    <ol class="stages">
      <li v-for="(key, index) in stages" :key="key" :class="[state(key), { here: key === journey.stage }]">
        <RouterLink :to="journeyPath(journey.project_node_id, key)" :aria-current="viewed === key ? 'page' : undefined" :aria-label="`${stageLabel(key)} · ${state(key)}`" :title="`${stageLabel(key)} · ${state(key)}`" class="stage-link">
          <span class="stage-number"><JourneyIcon v-if="state(key) === 'done'" name="check" /><JourneyIcon v-else-if="state(key) === 'skipped'" name="minus" /><template v-else>{{ index + 1 }}</template></span>
          <span v-if="key !== journey.stage">{{ stageLabel(key) }}</span>
          <span v-else class="sr-only">{{ stageLabel(key) }}</span>
        </RouterLink>
        <template v-if="key === journey.stage">
          <span v-if="passive" class="stage-cta passive" role="status"><span class="led" />{{ journey.next_action.label }}</span>
          <RouterLink v-else-if="viewed !== journey.next_action.stage" class="stage-cta" :to="journeyPath(journey.project_node_id, journey.next_action.stage)"><span class="led" />{{ journey.next_action.label }}</RouterLink>
          <button v-else class="stage-cta" :disabled="disabled || !journey.next_action.available" :title="journey.next_action.reason" @click="$emit('action')"><span class="led" />{{ journey.next_action.label }}</button>
        </template>
        <span v-if="index < stages.length - 1" class="stage-line" aria-hidden="true" />
      </li>
    </ol>
  </nav>
</template>
<style scoped>
.journey-bar { min-width: 0; padding: 14px 22px 18px; background: linear-gradient(180deg,var(--glass-2),var(--glass)); border-top: 1px solid var(--glass-edge); box-shadow: 0 -1px 0 var(--line); backdrop-filter: blur(18px); }
.journey-title { display:flex; justify-content:space-between; gap:20px; font-size:12px; margin-bottom:12px; color:var(--ink-2); }
.journey-title b { color:var(--ink); }.journey-hint { font:10px var(--mono); letter-spacing:.06em; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; max-width:50%; align-self:center; }
.stages { display:flex; list-style:none; margin:0; padding:3px; overflow-x:auto; align-items:center; }
.stages li { display:flex; flex:1; min-width:0; align-items:center; }.stages li:last-child { flex:0 0 auto; }.stages li.here { flex:0 0 auto; }
.stage-link { display:inline-flex; align-items:center; gap:7px; padding:4px 6px; border-radius:999px; color:var(--ink-2); font-size:12px; white-space:nowrap; }
.stage-link:hover,.stage-link[aria-current] { background:var(--surface); color:var(--teal-ink); }
.stage-number { display:grid; place-items:center; width:26px; height:26px; border-radius:50%; background:var(--glass); box-shadow:0 0 0 1px var(--line-2); font:11px var(--mono); flex:none; }.stage-number svg { width:14px; height:14px; }
.done .stage-number { color:var(--gold-ink); background:var(--gold-wash); box-shadow:0 0 0 1px var(--gold); }.here .stage-number { color:var(--teal-ink); background:var(--aqua); }.skipped .stage-link { opacity:.6; }
.stage-line { height:1px; flex:1; min-width:6px; margin:0 4px; background:linear-gradient(90deg,var(--gold-wash),var(--gold-2),var(--gold-wash)); }.done .stage-line { background:var(--gold); }
.stage-cta { display:inline-flex; gap:7px; align-items:center; justify-content:center; white-space:nowrap; min-height:36px; padding:6px 12px; border:1px solid var(--glass-rim); border-radius:999px; background:var(--teal); color:var(--button-ink); font-size:12px; font-weight:600; }.stage-cta:hover { box-shadow:0 0 18px var(--glass-rim); }.stage-cta:disabled { cursor:not-allowed; }.passive { background:var(--aqua-3); color:var(--teal-ink); }
.led { width:6px; height:6px; border-radius:50%; background:currentColor; flex:none; }.blocked .stage-number { box-shadow:0 0 0 1px var(--warn); }
@media(max-width:1050px) { .stages li { flex:0 0 auto; }.stage-line { width:10px; }.journey-bar { padding-inline:12px; } }
</style>
