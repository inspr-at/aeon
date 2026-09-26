<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getNode } from '../../lib/api'
import { activeRun, launchState } from '../../lib/startAgent'
import { useAgents } from '../../stores/agents'
import AppIcon from '../AppIcon.vue'

const agents = useAgents()
const titles = ref<Record<string, string>>({})
const pending = computed(() => Object.values(agents.runs).filter(run => activeRun(run) && !agents.sessions.some(s => s.run_id === run.id && s.management_mode === 'managed')))
watch(() => pending.value.map(r => r.work_order_id), async ids => {
  for (const id of new Set(ids)) {
    if (titles.value[id]) continue
    try { titles.value[id] = (await getNode(id)).title } catch { /* Keep the durable run ID when the order is not readable. */ }
  }
}, { immediate: true })
</script>

<template>
  <section v-if="pending.length" class="run-queue" aria-label="Runs awaiting a session">
    <header><AppIcon name="clock" :size="16" /><h2>Awaiting a session</h2><span class="count mono">{{ pending.length }}</span></header>
    <p>Queued work appears as a managed session after the daemon claims and registers it.</p>
    <ul>
      <li v-for="run in pending" :key="run.id">
        <div class="run-title"><strong>{{ titles[run.work_order_id] || `Run ${run.id.slice(0, 8)}` }}</strong><span>{{ run.requested_model || 'Model not reported' }}</span></div>
        <span class="run-state"><AppIcon :name="run.status === 'queued' ? 'clock' : 'check'" :size="13" />{{ launchState(run).label }}</span>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.run-queue { border: 1px solid var(--line); border-radius: 16px; background: var(--surface-raised); padding: 18px; }
header { display: flex; align-items: center; gap: 8px; color: var(--ink-2); }
h2 { font-size: 15px; color: var(--ink); }
.count { margin-left: auto; font-size: 12px; }
.run-queue > p { font-size: 12px; color: var(--ink-2); margin: 8px 0 12px; line-height: 1.5; }
ul { list-style: none; padding: 0; margin: 0; }
li { display: flex; align-items: center; gap: 16px; padding: 12px 0; border-top: 1px solid var(--line); }
.run-title { flex: 1; min-width: 0; display: grid; gap: 4px; }
.run-title strong { font-size: 13px; overflow-wrap: anywhere; }
.run-title > span { font-size: 12px; color: var(--ink-2); overflow-wrap: anywhere; }
.run-state { display: inline-flex; align-items: center; gap: 6px; padding: 5px 9px; border-radius: 999px; background: var(--surface-sunken); color: var(--ink-2); font-size: 12px; white-space: nowrap; }
</style>
