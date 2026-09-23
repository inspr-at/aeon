<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import AgentWorkspace from '../components/AgentWorkspace.vue'
import { listAccounts, message, setAccountState, timestamp, type AgentAccount } from '../lib/agents'
import { useAgentLive } from '../lib/agentLive'
import { useSession } from '../stores/session'
const { data: accounts, error, busy, live, refresh } = useAgentLive(listAccounts)
const session = useSession()
const admin = computed(() => session.identity?.principal.kind === 'person' && session.identity.principal.roles?.includes('admin') === true)
const pending = ref('')
const actionError = ref('')
async function change(account: AgentAccount, state: AgentAccount['state']) {
  if (pending.value) return
  pending.value = account.id; actionError.value = ''
  try { await setAccountState(account.id, state); await refresh() }
  catch (cause) { actionError.value = message(cause) }
  finally { pending.value = '' }
}
</script>
<template>
  <AgentWorkspace title="Agents" :live="live" :busy="busy" :error="error" @refresh="refresh">
    <p>Registered daemon accounts and their agent identities. A successful probe reports availability; it does not prove ownership of a running session.</p>
    <p v-if="actionError" class="error" role="alert">{{ actionError }}</p>
    <p v-if="accounts?.length === 0" class="glass-card agent-empty">No daemon accounts registered yet.</p>
    <div class="agent-grid">
      <article v-for="account in accounts" :key="account.id" class="glass-card agent-card" :aria-label="account.label">
        <div class="agent-card-heading"><h2>{{ account.label }}</h2><span class="agent-badge">{{ account.state }}</span></div>
        <dl><div><dt>Agent principal</dt><dd>{{ account.registered_by_principal_id }}</dd></div><div><dt>Harness</dt><dd>{{ account.harness }}</dd></div><div><dt>Daemon</dt><dd>{{ account.daemon_id }}</dd></div><div><dt>Parallel run limit</dt><dd>{{ account.max_parallel_runs ?? 'Not reported' }}</dd></div><div><dt>Last probe</dt><dd>{{ timestamp(account.last_probe_at) }}</dd></div><div><dt>Probe result</dt><dd>{{ account.last_probe_ok == null ? 'Unknown' : account.last_probe_ok ? 'Successful' : 'Unavailable' }}</dd></div></dl>
        <div v-if="admin" class="agent-actions"><button v-for="state in (['available', 'draining', 'unavailable'] as const)" :key="state" class="button secondary" :disabled="!!pending || state === account.state || !!error" @click="change(account, state)">{{ state === 'available' ? 'Activate' : state === 'draining' ? 'Drain' : 'Disable' }}</button></div>
      </article>
    </div>
  </AgentWorkspace>
</template>
