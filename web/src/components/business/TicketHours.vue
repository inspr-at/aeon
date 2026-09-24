<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getTimeTotals, type TimeTotals } from '../../lib/business'
import { useBusiness } from '../../stores/business'
import { formatSpan } from './duration'
import { formatAmount } from './money'
import AppIcon from './BizIcon.vue'

// Hours logged on a ticket and everything below it (an epic's tickets and their
// tasks), for the ticket panel's properties. Renders one `div.prop` with a dt
// and dd, so it drops into TicketProperties' <dl> in either layout:
//   <TicketHours :node-id="item.id" :kind="item.kind_slug" :layout="layout" />
// It shows nothing while the hours plugin is closed or nothing was logged.
const props = defineProps<{ nodeId: string; kind?: string; layout?: 'row' | 'column' }>()
const business = useBusiness()
const all = ref<TimeTotals | null>(null)
const approved = ref<TimeTotals | null>(null)
let generation = 0
watch(() => [props.nodeId, business.open.hours] as const, async ([id, open]) => {
  const request = ++generation
  all.value = null; approved.value = null
  if (!id) return
  await business.loadPlugins()
  if (!open && !business.open.hours) return
  try {
    const [total, sealed] = await Promise.all([getTimeTotals(id), getTimeTotals(id, true)])
    if (request === generation) { all.value = total; approved.value = sealed }
  } catch { /* hours stay hidden */ }
}, { immediate: true })
const shown = computed(() => !!all.value && all.value.duration_seconds > 0)
const below = computed(() => props.kind === 'epic' || props.kind === 'project' ? ', including everything below it' : '')
const tip = computed(() => {
  if (!all.value) return ''
  const money = all.value.amounts.map(a => { try { return `${formatAmount(a.amount, a.currency)} ${a.currency}` } catch { return `${a.amount} ${a.currency}` } }).join(' · ')
  const sealed = approved.value ? `${formatSpan(approved.value.duration_seconds)} approved` : ''
  return [`${formatSpan(all.value.duration_seconds)} logged${below.value}`, money, sealed].filter(Boolean).join('\n')
})
</script>

<template>
  <div v-if="shown && all" class="prop ticket-hours" :class="layout ?? 'row'">
    <dt>Logged</dt>
    <dd><RouterLink class="hours-chip" to="/business/hours" :data-tip="tip" :aria-label="tip.replace(/\n/g, '. ')"><AppIcon name="clock" :size="13" class="clock" /><span class="mono">{{ formatSpan(all.duration_seconds) }}</span></RouterLink></dd>
  </div>
</template>

<style scoped>
.ticket-hours.row dt { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
.ticket-hours dd { margin: 0; }
.ticket-hours.column { display: grid; grid-template-columns: 92px minmax(0, 1fr); align-items: center; min-height: 34px; }
.ticket-hours.column dt { font: 500 10.5px/1 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.hours-chip { display: inline-flex; align-items: center; gap: 7px; height: 28px; padding: 0 11px 0 9px; border-radius: 999px; color: var(--ink); font-size: 12.5px; text-decoration: none; white-space: nowrap; }
.row .hours-chip { background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); }
.column .hours-chip { margin-left: -9px; }
@media (hover: hover) { .hours-chip:hover { background: var(--row-hover); box-shadow: inset 0 0 0 1px var(--glass-rim); } }
.hours-chip:focus-visible { box-shadow: var(--focus-ring); }
.clock { color: var(--ink-3); }
.mono { font-family: var(--mono); font-size: 11.5px; font-variant-numeric: tabular-nums; font-variant-ligatures: none; }
@media (max-width: 720px) { .row .hours-chip { height: 34px; } }
</style>
