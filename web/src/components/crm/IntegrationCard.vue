<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import '../../styles/crm.css'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { configureProvider, errorText, getProviderSyncStatus, importProvider, listProviders, searchProviders, syncProvider, websiteHost, type Customer, type Provider, type ProviderSyncStatus, type RemoteCustomer } from '../../lib/crm'
import AppIcon from '../AppIcon.vue'
import BizIcon from '../business/BizIcon.vue'

const props = defineProps<{ admin: boolean; customer?: Customer | null; bare?: boolean }>()
const emit = defineEmits<{ imported: [customer: Customer]; synced: [customer: Customer] }>()
const providers = ref<Provider[] | null>(null)
const providerError = ref('')
const query = ref('')
const matches = ref<RemoteCustomer[] | null>(null)
const searchError = ref('')
const busy = ref('')
const secretRefs = reactive<Record<string, string>>({})
const syncStatus = ref<ProviderSyncStatus | null>(null)
const syncError = ref('')
async function loadProviders() {
  if (!props.admin) return
  try { providers.value = await listProviders(); providerError.value = '' }
  catch (e) { providers.value = []; providerError.value = errorText(e, 'Providers could not be loaded.') }
}
async function loadSyncStatus() {
  if (!props.admin || !props.customer?.external_provider) { syncStatus.value = null; return }
  try { syncStatus.value = await getProviderSyncStatus(props.customer.id) }
  catch { syncStatus.value = null }
}
onMounted(() => { void loadProviders(); void loadSyncStatus() })
watch(() => [props.customer?.id, props.customer?.external_provider], () => { void loadSyncStatus() })
const connected = computed(() => (providers.value ?? []).filter(p => p.enabled && p.configured))
const linked = computed(() => !!props.customer?.external_provider)
async function search() {
  if (!query.value.trim() || busy.value) return
  busy.value = 'search'; matches.value = null; searchError.value = ''
  try { matches.value = await searchProviders(query.value.trim()) }
  catch (e) { searchError.value = errorText(e, 'Provider search failed.') }
  finally { busy.value = '' }
}
async function importMatch(match: RemoteCustomer) {
  if (busy.value) return
  busy.value = `${match.provider}:${match.external_id}`; searchError.value = ''
  try {
    const customer = await importProvider(match.provider, match.external_id)
    matches.value = matches.value?.filter(item => item !== match) ?? []
    emit('imported', customer)
  } catch (e) { searchError.value = errorText(e, 'Import failed.') }
  finally { busy.value = '' }
}
async function sync() {
  if (!props.customer || busy.value) return
  busy.value = 'sync'; syncError.value = ''
  try { emit('synced', await syncProvider(props.customer.id)) }
  catch (e) { syncError.value = errorText(e, 'Sync failed.') }
  finally { busy.value = ''; await loadSyncStatus() }
}
async function configure(provider: Provider, enabled: boolean) {
  if (busy.value) return
  busy.value = provider.id; providerError.value = ''
  try {
    const next = await configureProvider(provider, enabled, enabled ? (secretRefs[provider.id] ?? '').trim() : '')
    providers.value = providers.value?.map(p => p.id === provider.id ? next : p) ?? [next]
    secretRefs[provider.id] = ''
  } catch (e) { providerError.value = errorText(e, 'Provider settings could not be saved. Refresh and try again.') }
  finally { busy.value = '' }
}
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
    <div v-if="admin && customer?.external_provider" class="provider-actions">
      <button type="button" class="btn sm" :disabled="!!busy || !connected.some(p => p.id === customer!.external_provider)" @click="sync"><AppIcon name="refresh" :size="13" />{{ busy === 'sync' ? 'Syncing…' : 'Sync now' }}</button>
      <span v-if="syncStatus?.state === 'ok'" role="status">Synced {{ new Date(syncStatus.synced_at!).toLocaleString() }}</span>
      <span v-else-if="syncStatus?.state === 'error'" role="status">Last sync failed{{ syncStatus.attempted_at ? ` · ${new Date(syncStatus.attempted_at).toLocaleString()}` : '' }}</span>
      <span v-else role="status">Not synced yet</span>
      <p v-if="syncError" role="alert">{{ syncError }}</p>
    </div>
    <form v-if="admin && !customer && connected.length" class="provider-search" @submit.prevent="search">
      <label><span>Search connected CRM</span><input v-model="query" class="field" type="search" maxlength="200" autocomplete="off" /></label>
      <button type="submit" class="btn sm" :disabled="!query.trim() || !!busy"><AppIcon name="search" :size="13" />{{ busy === 'search' ? 'Searching…' : 'Search' }}</button>
    </form>
    <p v-if="searchError" role="alert">{{ searchError }}</p>
    <p v-if="matches && !matches.length" class="hint">No new customers matched.</p>
    <ul v-if="matches?.length" class="matches" aria-label="External customers">
      <li v-for="match in matches" :key="`${match.provider}:${match.external_id}`">
        <span>{{ match.name }} <small>{{ match.provider }}</small></span>
        <button type="button" class="btn sm" :disabled="!!busy" @click="importMatch(match)">Import</button>
      </li>
    </ul>
    <details v-if="admin && providers?.length" class="provider-config">
      <summary><AppIcon name="chevron-right" :size="12" class="disclosure-chev" />Provider settings</summary>
      <p>Use an operator-provisioned <code>secret://</code> reference. Credentials never go in this form.</p>
      <div v-for="provider in providers" :key="provider.id" class="config-row">
        <strong>{{ provider.id }}</strong><span>{{ provider.enabled ? 'Enabled' : provider.configured ? 'Configured, off' : 'Off' }}</span>
        <template v-if="!provider.enabled">
          <input v-model="secretRefs[provider.id]" class="field" :aria-label="`Secret reference for ${provider.id}`" placeholder="secret://…" autocomplete="off" />
          <button type="button" class="btn sm" :disabled="!!busy || !secretRefs[provider.id]?.startsWith('secret://')" @click="configure(provider, true)">Enable</button>
        </template>
        <button v-else type="button" class="btn sm ghost" :disabled="!!busy" @click="configure(provider, false)">Disable</button>
      </div>
      <p v-if="providerError" role="alert">{{ providerError }}</p>
    </details>
    <p class="hint">
      <template v-if="connected.length">Manual customers remain available alongside connected providers.</template>
      <template v-else-if="admin">No CRM provider is connected. Customers can still be managed by hand.</template>
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
.provider-actions, .provider-search { display: flex; align-items: end; flex-wrap: wrap; gap: 8px; margin-top: 12px; font-size: 12px; }
.provider-search label { display: grid; gap: 4px; min-width: 180px; flex: 1; }
.provider-search label span { color: var(--ink-2); }
.provider-actions [role="alert"], .provider-config [role="alert"] { width: 100%; color: var(--danger); }
.matches { list-style: none; margin: 10px 0 0; padding: 0; display: grid; gap: 6px; }
.matches li { display: flex; justify-content: space-between; align-items: center; gap: 8px; padding: 8px 10px; border-radius: 8px; background: var(--surface-2); }
.matches small { color: var(--ink-3); }
.provider-config { margin-top: 12px; font-size: 12px; }
.provider-config summary { cursor: pointer; font-weight: 600; }
.provider-config p { margin: 8px 0; color: var(--ink-2); }
.config-row { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.config-row .field { min-width: 160px; flex: 1; }
</style>
