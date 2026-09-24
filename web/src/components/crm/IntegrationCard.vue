<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import '../../styles/crm.css'
import { computed, onMounted, ref } from 'vue'
import { listProviders, websiteHost, type Customer, type Provider } from '../../lib/crm'
import AppIcon from '../AppIcon.vue'
import BizIcon from '../business/BizIcon.vue'

// Another CRM (search, import, sync), said honestly: this build has no provider
// connected, and connecting one is an operator's job on the server (a secret
// reference, never a key typed here). Admins see what the server reports.
const props = defineProps<{ admin: boolean; customer?: Customer | null; bare?: boolean }>()
const providers = ref<Provider[] | null>(null)
// A failed read (or a server without providers) reads as not connected.
onMounted(async () => {
  if (!props.admin) return
  try { providers.value = await listProviders() } catch { providers.value = [] }
})
const connected = computed(() => (providers.value ?? []).filter(p => p.enabled && p.configured))
const linked = computed(() => !!props.customer?.external_provider)
</script>

<template>
  <component :is="bare ? 'div' : 'section'" :class="bare ? 'integration bare' : 'crm-card glass-card integration'" :aria-labelledby="bare ? undefined : 'integration-title'">
    <header v-if="!bare" class="card-head">
      <span class="card-icon" aria-hidden="true"><BizIcon name="plug" :size="15" /></span>
      <div class="card-titles">
        <h2 id="integration-title">Other CRM</h2>
        <p class="card-lead">Search, import and sync.</p>
      </div>
    </header>
    <div class="state-row">
      <span class="dot" :class="{ on: connected.length > 0 }" aria-hidden="true" />
      <span class="state-text">{{ connected.length ? `Connected on the server: ${connected.map(p => p.id).join(', ')}` : 'Not connected' }}</span>
    </div>
    <p v-if="linked" class="linked">
      <AppIcon name="link" :size="13" />
      <span class="dot-list">
        <span>Linked to {{ customer!.external_provider }}</span><span class="mono">{{ customer!.external_id }}</span>
        <a v-if="customer!.external_url" :href="customer!.external_url" target="_blank" rel="noopener" class="ext">{{ websiteHost(customer!.external_url) }}<AppIcon name="external" :size="11" /></a>
      </span>
    </p>
    <p class="hint">
      <template v-if="connected.length">Searching, importing and syncing from here arrive in a later version. Until then customers are kept by hand.</template>
      <template v-else-if="admin">No CRM provider is connected, so customers are kept by hand. An operator connects one on the server with a secret reference; nothing leaves this workspace until then.</template>
      <template v-else>Customers are kept by hand here. A workspace admin can have another CRM connected.</template>
    </p>
  </component>
</template>

<style scoped>
.state-row { display: flex; align-items: center; gap: 8px; font-size: 13.5px; font-weight: 600; color: var(--ink); }
.dot { width: 8px; height: 8px; border-radius: 50%; background: transparent; box-shadow: inset 0 0 0 1.5px var(--ink-3); flex-shrink: 0; }
.dot.on { background: var(--ok); box-shadow: none; }
.linked { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin-top: 8px; font-size: 12.5px; color: var(--ink-2); }
.linked svg { color: var(--ink-3); }
.mono { font-family: var(--mono); font-variant-ligatures: none; }
.ext { display: inline-flex; align-items: center; gap: 4px; color: var(--teal-ink); text-decoration: none; }
.ext:hover { text-decoration: underline; }
.hint { margin-top: 8px; font-size: 12.5px; line-height: 1.5; color: var(--ink-2); }
</style>
