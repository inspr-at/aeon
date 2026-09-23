<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import type { Approval } from '../../lib/agents'
const props = defineProps<{ title: string; approval?: Approval; requested?: boolean; busy: boolean; canDecide: boolean; independent?: boolean }>()
defineEmits<{ decide: [id: string, decision: 'approved' | 'denied', reason: string] }>()
const reason = ref('')
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => { timer = setInterval(() => { now.value = Date.now() }, 1000) })
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
const expired = computed(() => !!props.approval && (!Number.isFinite(Date.parse(props.approval.expires_at)) || Date.parse(props.approval.expires_at) <= now.value))
</script>
<template>
  <section class="j-card gate" aria-label="Human gate">
    <div class="eyebrow">Human decision</div><h2>{{ title }}</h2>
    <template v-if="approval">
      <p>{{ approval.rationale }}</p>
      <dl class="j-kv"><dt>Scope</dt><dd>{{ approval.scope }}</dd><dt>Resource</dt><dd>{{ approval.resource_kind }} · {{ approval.resource_id || 'Tenant' }}</dd><dt>Status</dt><dd>{{ expired ? 'Expired' : approval.decision || 'Awaiting decision' }}</dd><dt>Expires</dt><dd>{{ new Date(approval.expires_at).toLocaleString() }}</dd><template v-if="approval.decided_by_principal_id"><dt>Decided by</dt><dd>{{ approval.decided_by_principal_id }}</dd></template></dl>
      <p v-if="independent" class="j-note">Enterprise requires a named reviewer independent of the builder and current compliance checks. The server verifies the deciding person.</p>
      <form v-if="!approval.decision && !expired && canDecide" @submit.prevent="$emit('decide', approval.id, 'approved', reason)">
        <label class="j-label">Decision reason<textarea v-model="reason" maxlength="2048" rows="2" required /></label>
        <div class="j-actions"><button class="j-button" :disabled="busy || !reason.trim()">Approve gate</button><button type="button" class="j-button" :disabled="busy || !reason.trim()" @click="$emit('decide', approval.id, 'denied', reason)">Deny gate</button></div>
      </form>
      <p v-else-if="!canDecide" class="j-note">A person must decide this gate.</p>
    </template>
    <p v-else>{{ requested ? 'The referenced approval is unavailable. Refresh or inspect Approvals.' : 'No human gate has been proposed for this stage yet.' }}</p>
    <RouterLink class="j-link" to="/approvals">View approvals</RouterLink>
  </section>
</template>
<style scoped>
.gate { position:relative; background:radial-gradient(ellipse at 100% 0,var(--aqua-2),transparent 65%),var(--glass); }
.gate::after { content:''; position:absolute; inset:6px; border:1px solid color-mix(in srgb,var(--gold) 35%,transparent); border-radius:11px; pointer-events:none; }
.gate h2 { font-size:25px; font-weight:300; margin:6px 0 12px; padding-right:32px; }.gate::before { content:''; position:absolute; top:24px; right:24px; width:24px; height:24px; border:1.5px solid var(--gold); border-radius:50%; background:radial-gradient(circle,var(--aqua) 0 3px,var(--surface) 4px, var(--gold-wash)); }
.j-link { display:inline-block; margin-top:12px; font-size:12px; }
</style>
