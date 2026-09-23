<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useSession } from '../../stores/session'
import {
  BusinessIcon,
  BusinessShell,
  businessSections,
  availability,
  configurePlugin,
  installationWrite,
  isTenantAdmin,
  pinAction,
  shortDigest,
  statusLabel,
  useBusinessCatalog,
  type AreaAvailability,
  type BusinessPlugin,
} from '../../components/business'

const session = useSession()
const { items, error, busy, refresh } = useBusinessCatalog()
const admin = computed(() => isTenantAdmin(session.identity))
const pending = ref('')
const actionError = ref('')
const failedId = ref('')
let alive = true
onBeforeUnmount(() => { alive = false })

const cards = computed(() => {
  const catalog = items.value
  if (!catalog) return []
  return businessSections.map(area => ({
    area,
    item: catalog.find(entry => entry.id === area.pluginId),
    state: availability(area, catalog),
  }))
})

function badge(state: AreaAvailability) {
  if (state.open) return 'open'
  if (state.gate.state === 'digest_mismatch') return 'mismatch'
  if (state.gate.state === 'open') return 'waiting'
  return 'closed'
}

async function pin(item: BusinessPlugin, enabled: boolean) {
  if (!admin.value || pending.value) return
  pending.value = item.id
  actionError.value = ''
  failedId.value = ''
  try {
    await configurePlugin(item.id, installationWrite(item, enabled))
    await refresh()
  } catch (cause) {
    if (!alive) return
    actionError.value = cause instanceof Error ? cause.message : 'Could not update the plugin.'
    failedId.value = item.id
  } finally {
    if (alive) pending.value = ''
  }
}
</script>

<template>
  <BusinessShell
    eyebrow="Commercial work"
    title="Business"
    :catalog="items"
    :busy="busy"
    :error="error"
    @refresh="refresh"
  >
    <p class="business-lead">Rates, organisations, quotes, and hours. Amounts stay in the currency they were recorded in. Opening a section does not grant access; the server checks every action.</p>
    <p v-if="busy && !items" role="status">Loading plugins…</p>
    <div v-else-if="items" class="business-grid">
      <article v-for="card in cards" :key="card.area.id" class="glass-card business-card" :aria-label="card.area.label" :data-business-area="card.area.id">
        <div class="business-card-heading">
          <span class="business-mark"><BusinessIcon :name="card.area.icon" /></span>
          <div class="business-card-title">
            <h2>{{ card.area.label }}</h2>
            <p class="business-badge" :data-state="badge(card.state)">{{ statusLabel(card.state) }}</p>
          </div>
        </div>
        <p>{{ card.area.summary }}</p>
        <p v-if="card.item?.node_kinds.length" class="business-meta">Kinds {{ card.item.node_kinds.map(kind => kind.slug).join(', ') }}</p>
        <p v-if="card.item" class="business-digest">
          Build {{ shortDigest(card.item.digest_sha256) }}
          <template v-if="card.state.gate.state === 'digest_mismatch'"> · Pinned {{ shortDigest(card.item.installation.manifest_digest_sha256) }}</template>
        </p>
        <p v-if="!card.state.open">{{ card.state.reason }}</p>
        <p v-if="admin && card.item && (pinAction(card.state) === 'enable' || pinAction(card.state) === 'grant')" class="business-meta">
          {{ card.item.permissions.length ? `Grants ${card.item.permissions.join(', ')}.` : 'This plugin declares no permissions.' }}
        </p>
        <p v-if="failedId === card.area.pluginId && actionError" class="error" role="alert">{{ actionError }}</p>
        <div class="business-actions">
          <RouterLink v-if="card.state.open" class="button" :to="card.area.to">Open {{ card.area.label }}</RouterLink>
          <button
            v-if="admin && card.item && pinAction(card.state) === 'enable'"
            class="button"
            type="button"
            :disabled="!!pending || busy"
            :aria-label="`Enable ${card.area.label}`"
            @click="pin(card.item, true)"
          >
            <BusinessIcon name="check" />
            <span>{{ pending === card.area.pluginId ? 'Saving…' : 'Enable' }}</span>
          </button>
          <button
            v-if="admin && card.item && pinAction(card.state) === 'grant'"
            class="button"
            type="button"
            :disabled="!!pending || busy"
            :aria-label="`Grant declared permissions for ${card.area.label}`"
            @click="pin(card.item, true)"
          >
            <span>{{ pending === card.area.pluginId ? 'Saving…' : 'Grant permissions' }}</span>
          </button>
          <button
            v-if="admin && card.item && pinAction(card.state) === 'disable'"
            class="button secondary"
            type="button"
            :disabled="!!pending || busy"
            :aria-label="`Disable ${card.area.label}`"
            @click="pin(card.item, false)"
          >
            <span>{{ pending === card.area.pluginId ? 'Saving…' : 'Disable' }}</span>
          </button>
        </div>
      </article>
    </div>
  </BusinessShell>
</template>
