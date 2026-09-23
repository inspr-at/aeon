<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import type { BusinessIconName } from './areas'
defineProps<{ name: BusinessIconName }>()
</script>

<template>
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
    <template v-if="name === 'work'"><rect x="3" y="4" width="7" height="6" rx="1.5" /><rect x="14" y="14" width="7" height="6" rx="1.5" /><path d="M10 7h4a3 3 0 0 1 3 3v4" /></template>
    <template v-else-if="name === 'overview'"><rect x="3" y="3" width="7" height="7" rx="1.5" /><rect x="14" y="3" width="7" height="7" rx="1.5" /><rect x="3" y="14" width="7" height="7" rx="1.5" /><rect x="14" y="14" width="7" height="7" rx="1.5" /></template>
    <template v-else-if="name === 'costs'"><path d="M12 3 21 12l-9 9-9-9V3h9Z" /><path d="M9 9h.01" /></template>
    <template v-else-if="name === 'crm'"><rect x="4" y="3" width="16" height="18" rx="1.5" /><path d="M8 7h.01M12 7h.01M16 7h.01M8 11h.01M12 11h.01M16 11h.01M10 21v-4h4v4" /></template>
    <template v-else-if="name === 'quotes'"><path d="M7 3h7l5 5v13H7a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1Z" /><path d="M14 3v5h5M8 13h8M8 17h5" /></template>
    <template v-else-if="name === 'hours'"><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></template>
    <path v-else-if="name === 'check'" d="m5 12 4 4L19 6" />
    <template v-else><path d="M21 12a9 9 0 1 1-2.2-5.8" /><path d="M21 3v6h-6" /></template>
  </svg>
</template>
