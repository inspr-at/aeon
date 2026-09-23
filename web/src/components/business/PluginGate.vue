<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { availability } from './catalog'
import { businessAreas, type BusinessArea } from './areas'
import type { BusinessPlugin } from './catalog'
import './business.css'

const props = defineProps<{
  pluginId: string
  catalog: BusinessPlugin[] | null
  error?: string
  busy?: boolean
}>()

const state = computed(() => {
  if (!props.catalog) return null
  const area = businessAreas.find((entry): entry is BusinessArea & { pluginId: string } => entry.pluginId === props.pluginId)
  if (!area) return { open: false, reason: 'This section is not part of the business shell.' }
  return availability(area, props.catalog)
})
</script>

<template>
  <p v-if="busy && !catalog" role="status">Loading plugins…</p>
  <p v-else-if="error && !catalog" class="error business-notice" role="alert">{{ error }}</p>
  <div v-else-if="!state?.open" class="glass-card business-closed" role="status">
    <h2>This section is closed.</h2>
    <p>{{ state?.reason || 'Plugin state is unavailable.' }}</p>
    <RouterLink class="button secondary" to="/business">Business overview</RouterLink>
  </div>
  <slot v-else />
</template>
