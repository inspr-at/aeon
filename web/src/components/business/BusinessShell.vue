<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { useId } from 'vue'
import { businessAreas } from './areas'
import { availability, type BusinessPlugin } from './catalog'
import BusinessIcon from './BusinessIcon.vue'
import './business.css'

const props = defineProps<{
  title: string
  eyebrow?: string
  busy?: boolean
  error?: string
  catalog?: BusinessPlugin[] | null
}>()
defineEmits<{ refresh: [] }>()
const titleId = useId()

function entry(area: typeof businessAreas[number]) {
  if (!area.pluginId || props.catalog === undefined) return { linked: true, reason: '' }
  if (props.catalog === null) return { linked: false, reason: 'Loading plugins.' }
  const state = availability(area, props.catalog)
  return { linked: state.open, reason: state.reason }
}
</script>

<template>
  <section class="business-shell" :aria-labelledby="titleId" :aria-busy="busy ? true : undefined">
    <header class="business-heading">
      <div>
        <p v-if="eyebrow" class="eyebrow">{{ eyebrow }}</p>
        <h1 :id="titleId">{{ title }}</h1>
      </div>
      <div class="business-heading-actions">
        <button class="button secondary" type="button" :disabled="busy" @click="$emit('refresh')">
          <BusinessIcon name="refresh" />
          <span>{{ busy ? 'Refreshing…' : 'Refresh' }}</span>
        </button>
      </div>
    </header>
    <nav class="business-nav" aria-label="Business">
      <RouterLink class="business-link" to="/">
        <BusinessIcon name="work" />
        <span>Work</span>
      </RouterLink>
      <template v-for="area in businessAreas" :key="area.id">
        <RouterLink v-if="entry(area).linked" class="business-link" :to="area.to">
          <BusinessIcon :name="area.icon" />
          <span>{{ area.label }}</span>
        </RouterLink>
        <button v-else class="business-link" type="button" disabled :aria-label="`${area.label}. ${entry(area).reason}`">
          <BusinessIcon :name="area.icon" />
          <span>{{ area.label }}</span>
        </button>
      </template>
    </nav>
    <p v-if="error" class="error business-notice" role="alert">{{ error }}</p>
    <div class="business-body">
      <slot />
    </div>
  </section>
</template>
