<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import AgentWorkspace from '../components/AgentWorkspace.vue'
import { decideApproval, listApprovals, message, revokeApproval, timestamp, type Approval } from '../lib/agents'
import { useAgentLive } from '../lib/agentLive'
import { useSession } from '../stores/session'
const { data: approvals, error, busy, live, refresh } = useAgentLive(listApprovals)
const session = useSession()
const person = computed(() => session.identity?.principal.kind === 'person')
const now = ref(Date.now())
const showHistory = ref(false)
const selected = ref('')
const reason = ref('')
const pending = ref(false)
const actionError = ref('')
const notice = ref('')
const revoked = ref<string[]>([])
const expired = (a: Approval) => !Number.isFinite(Date.parse(a.expires_at)) || Date.parse(a.expires_at) <= now.value
const visible = computed(() => (approvals.value ?? []).filter(a => showHistory.value || (!a.decision && !expired(a))))
let clock: ReturnType<typeof setInterval>
onMounted(() => { clock = setInterval(() => { now.value = Date.now() }, 1000) })
onBeforeUnmount(() => clearInterval(clock))
function review(a: Approval) { selected.value = a.id; reason.value = ''; actionError.value = ''; notice.value = '' }
async function decide(a: Approval, decision: 'approved' | 'denied' | 'revoke') {
  if (pending.value || !person.value || error.value || expired(a)) return
  pending.value = true; actionError.value = ''; notice.value = ''
  try {
    if (decision === 'revoke') { await revokeApproval(a.id); revoked.value.push(a.id); notice.value = 'Revocation recorded.' }
    else { await decideApproval(a.id, decision, reason.value); notice.value = `Decision recorded: ${decision}.` }
    selected.value = ''
    await refresh()
  } catch (cause) { actionError.value = message(cause); await refresh() }
  finally { pending.value = false }
}
</script>
<template>
  <AgentWorkspace title="Approvals" :live="live" :busy="busy" :error="error" @refresh="refresh">
    <p>Review the agent, permission and resource before deciding. A grant remains bounded by the agent’s API key scopes.</p>
    <label><input v-model="showHistory" type="checkbox" style="width:auto; min-height:auto" /> Include decided and expired requests</label>
    <p v-if="!person" class="agent-notice">A signed-in person is required to decide requests.</p>
    <p v-if="actionError" class="error" role="alert">{{ actionError }}</p><p v-if="notice" role="status">{{ notice }}</p>
    <p v-if="approvals?.length === 200" class="agent-notice">Showing up to 200 requests. Older requests may be outside this view.</p>
    <p v-if="approvals && !visible.length" class="glass-card agent-empty">{{ showHistory ? 'No approval requests.' : 'No pending approvals.' }}</p>
    <div class="agent-grid">
      <article v-for="approval in visible" :key="approval.id" class="glass-card agent-card" :aria-label="`Request ${approval.scope}`">
        <div class="agent-card-heading"><h2>{{ approval.scope }}</h2><span class="agent-badge">{{ approval.decision ?? (expired(approval) ? 'expired' : 'pending') }}</span></div>
        <p>{{ approval.rationale }}</p>
        <dl><div><dt>Agent principal</dt><dd>{{ approval.agent_principal_id }}</dd></div><div><dt>Resource</dt><dd>{{ approval.resource_kind }} · {{ approval.resource_id ?? 'Current tenant' }}</dd></div><div><dt>Proposed</dt><dd>{{ timestamp(approval.proposed_at) }}</dd></div><div><dt>Expires</dt><dd>{{ timestamp(approval.expires_at) }}</dd></div></dl>
        <RouterLink v-if="approval.run_id" :to="`/runs/${approval.run_id}`">Open related run</RouterLink>
        <p v-if="revoked.includes(approval.id)">Revocation recorded.</p>
        <template v-if="person && !expired(approval) && approval.decision !== 'denied' && !revoked.includes(approval.id)">
          <button v-if="selected !== approval.id" class="button secondary" :disabled="pending || !!error" @click="review(approval)">{{ approval.decision === 'approved' ? 'Review revocation' : 'Review request' }}</button>
          <div v-else class="agent-form">
            <label v-if="!approval.decision">Decision reason (optional)<textarea v-model="reason" rows="2" :disabled="pending" /></label>
            <p v-if="approval.decision === 'approved'">Revoke any grant from this request. The recorded approval decision remains in history.</p>
            <div class="agent-actions">
              <template v-if="!approval.decision"><button class="button" :disabled="pending || !!error" @click="decide(approval, 'approved')">Approve permission</button><button class="button secondary" :disabled="pending || !!error" @click="decide(approval, 'denied')">Deny request</button></template>
              <button v-else class="button" :disabled="pending || !!error" @click="decide(approval, 'revoke')">Revoke grant</button>
              <button class="button secondary" :disabled="pending" @click="selected = ''">Cancel</button>
            </div>
          </div>
        </template>
      </article>
    </div>
  </AgentWorkspace>
</template>
