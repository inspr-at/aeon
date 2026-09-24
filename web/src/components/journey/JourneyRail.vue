<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ACTION_LONG, STAGE_LABEL, STAGES, type Journey, type Stage } from '../../lib/journey'
import type { NextState } from '../../lib/journeyContext'
import AppIcon from '../AppIcon.vue'

// The stage rail: Inspire → … → Live from the server's projection. Done stages
// carry a gold check, the current stage its number and the one primary button
// (the next action), later ones their number. Hovering the rail turns the button
// gold and shows what it does.
const props = defineProps<{ journey: Journey; viewed: Stage; projectTitle: string; releaseLabel: string; action: NextState }>()
const emit = defineEmits<{ view: [stage: Stage]; act: [] }>()

const stateOf = (stage: Stage) => props.journey.stages.find(s => s.key === stage)?.state ?? 'later'
const next = computed(() => props.journey.next_action)
const current = computed(() => props.journey.stage)
// Passive: nothing to do now (the build runs, or the step cannot be taken yet).
const passive = computed(() => next.value.key === 'wait_for_build' || (props.action.disabled && next.value.key !== 'continue_intake'))
const why = computed(() => props.action.disabled ? props.action.tip || next.value.reason || '' : '')
// Beside the current stage, or somewhere else: then the button leads there first.
const onStage = computed(() => props.viewed === next.value.stage)
const hint = computed(() => {
  const who = passive.value ? 'Now' : onStage.value ? 'Your call' : 'Next'
  return `${who} · ${props.action.label}. ${why.value || ACTION_LONG[next.value.key]}`
})
const phone = ref(false)
const query = window.matchMedia('(max-width: 720px)')
const onQuery = () => { phone.value = query.matches }
onMounted(() => { onQuery(); query.addEventListener('change', onQuery) })
onBeforeUnmount(() => query.removeEventListener('change', onQuery))
function stepLabel(stage: Stage, index: number) {
  const state = stateOf(stage)
  const suffix = state === 'done' ? 'done' : state === 'skipped' ? 'not needed' : state === 'blocked' ? 'blocked' : state === 'current' ? 'now' : 'later'
  return `${index + 1}. ${STAGE_LABEL[stage]}, ${suffix}`
}
function cta() { if (onStage.value) emit('act'); else emit('view', next.value.stage) }
</script>

<template>
  <nav class="rail" :class="{ passive }" aria-label="Project journey">
    <div class="rail-title">
      <span class="tt"><b>{{ projectTitle }}</b><template v-if="releaseLabel"> · {{ releaseLabel }}</template> · {{ STAGE_LABEL[current] }}</span>
      <span class="hint" :title="hint">{{ hint }}</span>
    </div>
    <ol class="stages">
      <li v-for="(stage, index) in STAGES" :key="stage" :class="[stateOf(stage), { here: stage === current, viewed: stage === viewed }]">
        <button type="button" class="step" :aria-current="stage === viewed ? 'step' : undefined" :aria-label="stepLabel(stage, index)" :data-tip="stage === current ? `${STAGE_LABEL[stage]} · now` : undefined" @click="emit('view', stage)">
          <span class="n" aria-hidden="true">
            <AppIcon v-if="stateOf(stage) === 'done'" name="check" :size="12" />
            <AppIcon v-else-if="stateOf(stage) === 'skipped'" name="minus" :size="12" />
            <AppIcon v-else-if="stateOf(stage) === 'blocked'" name="alert" :size="12" />
            <template v-else>{{ index + 1 }}</template>
          </span>
          <span v-if="stage !== current || phone" class="t">{{ STAGE_LABEL[stage] }}</span>
        </button>
        <button
          v-if="stage === current && !phone" type="button" class="stage-cta" :class="{ quiet: passive }" :disabled="action.busy"
          :aria-label="`${action.label}${why ? ` (not yet: ${why})` : ''}`" :data-tip="why || undefined" @click="cta"
        ><span class="led" :class="{ on: !passive }" aria-hidden="true" /><span class="short">{{ action.label }}</span></button>
        <span v-if="index < STAGES.length - 1" class="link" aria-hidden="true" />
      </li>
    </ol>
    <button
      v-if="phone" type="button" class="stage-cta wide" :class="{ quiet: passive }" :disabled="action.busy"
      :aria-label="`${action.label}${why ? ` (not yet: ${why})` : ''}`" @click="cta"
    ><span class="led" :class="{ on: !passive }" aria-hidden="true" /><span class="short">{{ action.label }}</span><span v-if="why" class="why">{{ why }}</span></button>
  </nav>
</template>

<style scoped>
.rail {
  --rail-gold: var(--gold);
  position: relative; display: grid; gap: 8px; padding: 12px 18px 14px; border-radius: var(--radius);
  background: var(--glass); box-shadow: var(--shadow); backdrop-filter: blur(18px) saturate(1.1); -webkit-backdrop-filter: blur(18px) saturate(1.1);
}
.rail-title { display: flex; align-items: baseline; gap: 16px; min-width: 0; font-size: 13px; color: var(--ink-2); }
.rail-title .tt { flex-shrink: 0; white-space: nowrap; }
.rail-title b { color: var(--ink); font-weight: 600; }
/* What the button does: fades in while the rail is hovered or focused. */
.hint { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: right; color: var(--gold-ink); opacity: 0; transition: opacity .5s ease-in-out; }
.rail:hover .hint, .rail:focus-within .hint { opacity: 1; }
.stages { display: flex; align-items: center; gap: 0; margin: 0; padding: 0; list-style: none; }
.stages li { display: flex; align-items: center; flex: 1 1 0; min-width: 0; gap: 6px; }
.stages li:last-child, .stages li.here { flex: 0 0 auto; }
.stages li.here + li, .stages li:last-child { min-width: 0; }
.step { display: inline-flex; align-items: center; gap: 8px; min-width: 0; height: 34px; padding: 0 10px 0 4px; border: 0; border-radius: 999px; background: transparent; color: var(--ink-2); font-size: 13.5px; cursor: pointer; }
.step:hover { background: var(--row-hover); color: var(--ink); }
.step:focus-visible { box-shadow: var(--focus-ring); }
.t { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
li.viewed:not(.here) .step { background: var(--row-selected); color: var(--ink); }
li.viewed:not(.here) .t { font-weight: 600; }
.n {
  display: inline-grid; place-items: center; flex-shrink: 0; width: 26px; height: 26px; border-radius: 8px;
  background: var(--surface); color: var(--ink-3); font: 500 10.5px/1 var(--mono); font-variant-numeric: tabular-nums;
  box-shadow: 0 0 0 1px var(--line-2), 0 2px 4px -2px rgba(32, 60, 61, .25);
}
li.done .n { color: var(--gold-ink); background: linear-gradient(160deg, var(--surface), var(--gold-wash)); box-shadow: 0 0 0 1px rgba(214, 155, 49, .7), 0 0 10px -2px rgba(214, 155, 49, .45); }
li.skipped .step { opacity: .55; }
li.blocked .n { color: var(--danger); box-shadow: 0 0 0 1px var(--danger-line); background: var(--danger-bg); }
li.here .n { color: var(--teal-ink); background: radial-gradient(circle at 40% 35%, var(--surface), var(--aqua)); box-shadow: 0 0 0 1px var(--aqua), 0 0 14px rgba(164, 229, 223, .75); }
li.here .step { padding-right: 2px; }
/* Connectors: a faint gold hairline ahead, a bright one (with a dot) after a done stage. */
.link { position: relative; flex: 1 1 auto; min-width: 10px; height: 1px; margin: 0 6px; background: linear-gradient(90deg, rgba(214, 155, 49, .12), rgba(214, 155, 49, .5), rgba(214, 155, 49, .12)); }
li.done .link { background: linear-gradient(90deg, var(--gold-2), var(--gold), var(--gold-2)); box-shadow: 0 0 6px rgba(214, 155, 49, .35); }
li.done .link::after { content: ''; position: absolute; right: -2px; top: -2px; width: 5px; height: 5px; border-radius: 50%; background: var(--gold); }
/* The one primary button. */
.stage-cta {
  display: inline-flex; align-items: center; gap: 8px; height: 34px; padding: 0 16px 0 12px; border: 0; border-radius: 999px; white-space: nowrap;
  background: linear-gradient(180deg, #1a8683, #0e6f6c 60%, #0b5c59); color: #fff; font-size: 13.5px; font-weight: 700; cursor: pointer;
  box-shadow: 0 0 0 1px rgba(14, 111, 108, .5), 0 10px 24px -10px rgba(14, 111, 108, .7), inset 0 1px 0 rgba(255, 255, 255, .3), 0 0 26px -8px rgba(164, 229, 223, .9);
  transition: background .5s ease-in-out, box-shadow .5s ease-in-out, transform .15s ease;
}
.stage-cta:focus-visible { outline: 2px solid var(--gold); outline-offset: 2px; }
.rail:hover .stage-cta:not(.quiet), .stage-cta:not(.quiet):focus-visible {
  background: linear-gradient(180deg, #e6b356, var(--rail-gold) 60%, #b8842a); color: #1f1606;
  box-shadow: 0 0 0 1px rgba(214, 155, 49, .6), 0 10px 24px -10px rgba(214, 155, 49, .8), inset 0 1px 0 rgba(255, 255, 255, .4), 0 0 26px -8px rgba(232, 192, 122, .9);
}
.stage-cta:active:not(:disabled) { transform: translateY(1px); }
.stage-cta:disabled { opacity: .6; cursor: progress; }
.stage-cta.quiet { background: var(--surface); color: var(--ink); box-shadow: 0 0 0 1px var(--line-2), 0 6px 16px -12px rgba(32, 60, 61, .5); font-weight: 600; }
.led { width: 7px; height: 7px; border-radius: 50%; background: var(--ink-3); flex-shrink: 0; }
.led.on { background: #a4e5df; box-shadow: 0 0 8px #a4e5df; }
.rail:hover .stage-cta:not(.quiet) .led.on { background: #fff3d6; box-shadow: 0 0 8px #fff3d6; }
@media (prefers-reduced-motion: no-preference) { .led.on { animation: led 2.4s ease-in-out infinite; } }
@keyframes led { 50% { opacity: .55; } }
@media (max-width: 1180px) { .stages li:not(.here):not(.viewed) .t { display: none; } .stages li .step { padding-right: 4px; } }
@media (max-width: 720px) {
  .rail { padding: 10px 12px 12px; }
  .rail-title .hint { display: none; }
  .stages { overflow-x: auto; scrollbar-width: none; margin: 0 -12px; padding: 0 12px 2px; gap: 2px; }
  .stages li { flex: 0 0 auto; }
  .link { flex: 0 0 8px; min-width: 8px; margin: 0 1px; }
  .stages li .step { flex-direction: column; height: auto; gap: 3px; padding: 4px 4px; border-radius: 10px; font-size: 11px; }
  .stages li .t { display: inline !important; max-width: 64px; }
  .stage-cta.wide { justify-content: center; width: 100%; height: 44px; flex-wrap: wrap; row-gap: 0; }
  .why { flex-basis: 100%; font-size: 11px; font-weight: 500; opacity: .8; white-space: normal; }
  .stage-cta.wide:has(.why) { height: auto; min-height: 44px; padding: 6px 14px; }
}
</style>
