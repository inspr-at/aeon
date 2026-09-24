<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import { span, type ReleaseStats } from '../../lib/releases'
import { absoluteTime } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import CalendarVersion from '../CalendarVersion.vue'

// The header of the release history: what runs here, and the cadence at a glance.
// Compact (phones): what runs here, today and this week; the rest behind More stats.
const props = defineProps<{ stats: ReleaseStats; current: string; liveSince: string | null; now: number; compact?: boolean }>()
const more = ref(false)
const since = computed(() => props.stats.last === null ? '—' : span(props.now - props.stats.last))
const median = computed(() => props.stats.median === null ? '—' : span(props.stats.median))
const live = computed(() => {
  if (!props.liveSince) return ''
  const at = Date.parse(props.liveSince)
  if (Number.isNaN(at)) return ''
  return `${new Date(at).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })} · ${span(Math.max(60_000, props.now - at))}`
})
const peak = computed(() => Math.max(1, ...props.stats.cadence))
const cadenceLabel = computed(() => {
  const total = props.stats.cadence.reduce((a, b) => a + b, 0)
  return `${total} ${total === 1 ? 'release' : 'releases'} in the last ${props.stats.cadence.length} days, at most ${peak.value} a day`
})
</script>

<template>
  <div class="stats" :class="{ compact }" role="group" aria-label="Release cadence">
    <div class="tile running">
      <p class="label">Running here</p>
      <p class="value"><CalendarVersion v-if="current" :value="current" /></p>
      <p v-if="live" class="sub" :data-tip="liveSince ? `Live on this server since ${absoluteTime(liveSince)}` : undefined">Live since {{ live }}</p>
    </div>
    <div class="tile">
      <p class="label">Today</p>
      <p class="value num">{{ stats.today }}</p>
      <p class="sub">{{ stats.today === 1 ? 'release' : 'releases' }}</p>
    </div>
    <div class="tile">
      <p class="label">This week</p>
      <p class="value num">{{ stats.week }}</p>
      <p class="sub">since Monday</p>
    </div>
    <button v-if="compact" type="button" class="more" :aria-expanded="more" aria-controls="release-more-stats" @click="more = !more">
      {{ more ? 'Fewer stats' : 'More stats' }}<AppIcon name="chevron" :size="13" class="more-chev" />
    </button>
    <div v-if="!compact || more" id="release-more-stats" class="rest">
    <div class="tile">
      <p class="label">Since the last</p>
      <p class="value">{{ since }}</p>
      <p class="sub">release</p>
    </div>
    <div class="tile">
      <p class="label">Median gap</p>
      <p class="value">{{ median }}</p>
      <p class="sub">between releases</p>
    </div>
    <div class="tile cadence">
      <p class="label">Last {{ stats.cadence.length }} days</p>
      <svg class="spark" :viewBox="`0 0 ${stats.cadence.length * 8} 30`" preserveAspectRatio="none" role="img" :aria-label="cadenceLabel">
        <rect v-for="(n, i) in stats.cadence" :key="i" :x="i * 8 + 1" :width="6" rx="1.5"
          :y="n ? 30 - Math.max(4, (n / peak) * 28) : 28" :height="n ? Math.max(4, (n / peak) * 28) : 2" :class="{ zero: !n, today: i === stats.cadence.length - 1 }" />
      </svg>
      <p class="sub"><span>{{ stats.cadence.length - 1 }} days ago</span><span>today</span></p>
    </div>
    </div>
  </div>
</template>

<style scoped>
.stats { display: grid; grid-template-columns: minmax(220px, 1.6fr) repeat(4, minmax(100px, 1fr)) minmax(170px, 1.3fr); gap: 10px; }
.tile {
  display: grid; align-content: start; gap: 2px; min-width: 0; padding: 11px 14px 10px; border-radius: 14px;
  background: var(--glass); border: 1px solid var(--glass-edge); box-shadow: 0 0 0 1px var(--line);
}
.label { font: 500 10px/1.5 var(--mono); letter-spacing: .16em; text-transform: uppercase; color: var(--ink-3); white-space: nowrap; }
.value { font: 600 17px/1.35 var(--font); color: var(--ink); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; letter-spacing: -.01em; }
.value.num { font: 600 20px/1.25 var(--mono); font-variant-numeric: tabular-nums; }
.running .value { font: 500 16px/1.45 var(--mono); }
.sub { display: flex; justify-content: space-between; gap: 8px; font-size: 11.5px; color: var(--ink-3); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.spark { width: 100%; height: 30px; margin: 3px 0 1px; overflow: visible; }
.spark rect { fill: var(--teal); opacity: .55; }
.spark rect.today { opacity: 1; }
.spark rect.zero { fill: var(--line-2); opacity: 1; }
@media (max-width: 1280px) { .stats { grid-template-columns: minmax(200px, 1.5fr) repeat(4, minmax(90px, 1fr)) minmax(140px, 1.1fr); } .tile { padding: 10px 12px 9px; } }
@media (max-width: 1100px) { .stats { grid-template-columns: repeat(3, minmax(0, 1fr)); } .running { grid-column: span 2; } }
/* The rest sit in the same grid as the first three. */
.rest { display: contents; }
/* Phones: a two-column grid inside the gutter; nothing scrolls sideways. */
.stats.compact { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.compact .running, .compact .cadence, .compact .more { grid-column: 1 / -1; }
.compact .tile { padding: 9px 12px 8px; }
.compact .value.num { font-size: 18px; }
.compact .rest { display: grid; grid-column: 1 / -1; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.more { display: inline-flex; align-items: center; justify-content: center; gap: 6px; height: 44px; border: 0; border-radius: 12px; background: transparent; color: var(--teal-ink); font-size: 13px; font-weight: 600; }
.more:active { background: var(--row-selected); }
.more:focus-visible { box-shadow: var(--focus-ring); }
.more-chev { transition: transform .18s ease; }
.more[aria-expanded="true"] .more-chev { transform: rotate(180deg); }
@media (prefers-reduced-motion: reduce) { .more-chev { transition: none; } }
</style>
