<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import AgentWorkspace from '../components/AgentWorkspace.vue'
import { createWindow, listAccounts, message, paceFraction, timestamp, type AllowanceWindow, type AllowanceWrite } from '../lib/agents'
import { useAgentLive } from '../lib/agentLive'
import { useSession } from '../stores/session'
const { data: accounts, error, busy, live, refresh } = useAgentLive(listAccounts)
const session = useSession()
const admin = computed(() => session.identity?.principal.kind === 'person' && session.identity.principal.roles?.includes('admin') === true)
const accountId = ref('')
const starts = ref('')
const ends = ref('')
const allowance = ref(1000)
const unit = ref<AllowanceWrite['unit']>('tokens')
const model = ref<AllowanceWrite['pace_model']>('steady')
const burst = ref(0.1)
const saving = ref(false)
const actionError = ref('')
const created = ref<AllowanceWindow>()
const preview = computed(() => [0, 0.25, 0.5, 0.75, 1].map(f => ({ elapsed: Math.round(f * 100), fraction: paceFraction(model.value, f, Number(burst.value)) })))
async function save() {
  if (saving.value || !admin.value || error.value) return
  actionError.value = ''; created.value = undefined
  const start = new Date(starts.value); const end = new Date(ends.value)
  if (!accountId.value || !Number.isFinite(+start) || !Number.isFinite(+end) || +end <= +start || !Number.isSafeInteger(allowance.value) || allowance.value < 1 || !Number.isFinite(burst.value) || burst.value < 0 || burst.value > 1) {
    actionError.value = 'Choose an account, an end after the start, a positive whole allowance, and a burst between 0 and 1.'; return
  }
  saving.value = true
  try { created.value = await createWindow(accountId.value, { starts_at: start.toISOString(), ends_at: end.toISOString(), allowance: allowance.value, unit: unit.value, pace_model: model.value, burst_ratio: burst.value }) }
  catch (cause) { actionError.value = message(cause) }
  finally { saving.value = false }
}
</script>
<template>
  <AgentWorkspace title="Pacing" :live="live" :busy="busy" :error="error" @refresh="refresh">
    <p>Set a hard allowance and control how quickly it becomes available. Reservations count toward the limit alongside settled usage.</p>
    <p class="agent-notice">Existing allowance windows and live usage are not available in this view yet. The preview shows the policy you are configuring.</p>
    <p v-if="!admin" class="agent-notice">An administrator session is required to create allowance windows.</p>
    <div class="agent-grid">
      <form class="glass-card agent-card agent-form" @submit.prevent="save">
        <h2>New allowance window</h2>
        <label>Account<select v-model="accountId" required :disabled="saving"><option disabled value="">Choose an account</option><option v-for="account in accounts" :key="account.id" :value="account.id">{{ account.label }} · {{ account.harness }}</option></select></label>
        <div class="agent-form-row"><label>Starts (local time)<input v-model="starts" type="datetime-local" required :disabled="saving" /></label><label>Ends (local time)<input v-model="ends" type="datetime-local" required :disabled="saving" /></label></div>
        <div class="agent-form-row"><label>Allowance<input v-model.number="allowance" type="number" min="1" step="1" max="9007199254740991" required :disabled="saving" /></label><label>Unit<select v-model="unit" :disabled="saving"><option value="tokens">Tokens</option><option value="requests">Requests</option><option value="cost_micros">Cost (micros)</option></select></label></div>
        <label>Pacing model<select v-model="model" :disabled="saving"><option value="steady">Steady</option><option value="frontload">Frontload</option><option value="unrestricted">Unrestricted</option></select></label>
        <label>Burst ratio<input v-model.number="burst" type="number" min="0" max="1" step="0.01" required :disabled="saving" /></label>
        <p v-if="actionError" class="error" role="alert">{{ actionError }}</p>
        <button class="button" :disabled="saving || !admin || !!error || !accounts?.length">{{ saving ? 'Creating…' : 'Create allowance window' }}</button>
      </form>
      <article class="glass-card agent-card" aria-label="Pacing preview">
        <h2>Cumulative allowance preview</h2><p>{{ model === 'steady' ? 'Releases allowance evenly over the window.' : model === 'frontload' ? 'Releases more allowance early, then slows.' : 'Makes the full allowance available throughout the window.' }} Burst adds headroom, capped at the hard limit.</p>
        <table class="agent-table"><thead><tr><th scope="col">Window elapsed</th><th scope="col">Allowed fraction</th></tr></thead><tbody><tr v-for="point in preview" :key="point.elapsed"><td>{{ point.elapsed }}%</td><td>{{ Math.round(point.fraction * 100) }}%<progress :value="point.fraction" max="1" :aria-label="`Allowed at ${point.elapsed}% elapsed`" /></td></tr></tbody></table>
        <p>Dispatch also requires a recent successful daemon probe, free parallel capacity, and enough remaining allowance in every active window.</p>
      </article>
    </div>
    <article v-if="created" class="glass-card agent-card" aria-label="Created allowance window">
      <h2 role="status">Allowance window created</h2><p>Creation snapshot. Refresh does not update these counters.</p>
      <dl><div><dt>Window</dt><dd>{{ created.id }}</dd></div><div><dt>Account</dt><dd>{{ created.account_id }}</dd></div><div><dt>Starts</dt><dd>{{ timestamp(created.starts_at) }}</dd></div><div><dt>Ends</dt><dd>{{ timestamp(created.ends_at) }}</dd></div><div><dt>Allowance</dt><dd>{{ created.allowance }} {{ created.unit }}</dd></div><div><dt>Used / reserved at creation</dt><dd>{{ created.used }} / {{ created.reserved }}</dd></div></dl>
    </article>
  </AgentWorkspace>
</template>
