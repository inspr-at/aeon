<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AgentWorkspace from '../components/AgentWorkspace.vue'
import { getRun, timestamp, uuidPattern } from '../lib/agents'
import { useAgentLive } from '../lib/agentLive'
const route = useRoute()
const router = useRouter()
const runId = ref(String(route.params.runId ?? ''))
const read = () => route.params.runId ? getRun(String(route.params.runId)) : Promise.resolve(null)
const { data: run, error, busy, live, refresh } = useAgentLive(read)
watch(() => route.params.runId, () => { runId.value = String(route.params.runId ?? ''); run.value = null; void refresh() })
async function openRun() { await router.push(`/runs/${encodeURIComponent(runId.value.trim())}`) }
</script>
<template>
  <AgentWorkspace title="Sessions & runs" :live="live" :busy="busy" :error="error" @refresh="refresh">
    <p>Follow a durable AEON run across daemon sessions. Vendor session identifiers remain local to the daemon.</p>
    <form class="agent-form glass-card agent-card" @submit.prevent="openRun"><label>Run ID<input v-model="runId" required :pattern="uuidPattern" placeholder="Run UUID" /></label><button class="button" :disabled="busy">Open run</button></form>
    <p v-if="!route.params.runId" class="agent-notice">Open a run by ID or follow a run link from an approval. A workspace-wide session list is not available yet.</p>
    <article v-if="run" class="glass-card agent-card" aria-label="Run telemetry">
      <div class="agent-card-heading"><h2>Run telemetry</h2><span class="agent-badge">{{ run.status }}</span></div>
      <dl><div><dt>Run</dt><dd>{{ run.id }}</dd></div><div><dt>Agent principal</dt><dd>{{ run.agent_principal_id }}</dd></div><div><dt>Work order</dt><dd>{{ run.work_order_id }}</dd></div><div><dt>Account</dt><dd>{{ run.account_id ?? 'Not assigned' }}</dd></div><div><dt>Requested model</dt><dd>{{ run.requested_model ?? 'Not reported' }}</dd></div><div><dt>Effective model</dt><dd>{{ run.effective_model ?? 'Not reported' }} ({{ run.model_evidence === 'vendor_reported' ? 'Vendor reported' : 'Unverified' }})</dd></div><div><dt>Profile pin</dt><dd>{{ run.model_profile_id ?? 'Not reported' }}</dd></div><div><dt>Created</dt><dd>{{ timestamp(run.created_at) }}</dd></div><div><dt>Started</dt><dd>{{ timestamp(run.started_at) }}</dd></div><div><dt>Ended</dt><dd>{{ timestamp(run.ended_at) }}</dd></div></dl>
      <div class="agent-metrics"><div><p>Input tokens</p><strong>{{ run.input_tokens.toLocaleString() }}</strong></div><div><p>Output tokens</p><strong>{{ run.output_tokens.toLocaleString() }}</strong></div><div><p>Cost (micros)</p><strong>{{ run.cost_micros.toLocaleString() }}</strong></div></div>
      <p v-if="run.status === 'ownership_lost'" class="agent-notice">Daemon ownership was lost. A heartbeat alone does not authorize further control.</p>
    </article>
  </AgentWorkspace>
</template>
