<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
defineProps<{ name: 'sun' | 'moon' | 'user' | 'chevron' | 'arrow' | 'logout' | 'tree' | 'compass' | 'search' | 'list' | 'plus' | 'close' | 'edit' }>()
</script>

<template>
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
    <template v-if="name === 'sun'"><circle cx="12" cy="12" r="4" /><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5" /></template>
    <path v-else-if="name === 'moon'" d="M20.5 13.5A8.7 8.7 0 0 1 10.5 3a8.8 8.8 0 1 0 10 10.5Z" />
    <template v-else-if="name === 'user'"><circle cx="12" cy="8" r="3.3" /><path d="M5 21v-2a7 7 0 0 1 14 0v2" /></template>
    <path v-else-if="name === 'chevron'" d="m8 10 4 4 4-4" />
    <path v-else-if="name === 'arrow'" d="M4 12h15m-6-6 6 6-6 6" />
    <path v-else-if="name === 'logout'" d="M9 4H4v16h5m6-13 5 5-5 5m-6-5h11" />
    <template v-else-if="name === 'tree'"><rect x="9" y="3" width="6" height="6" rx="2" /><rect x="2" y="16" width="6" height="5" rx="1.5" /><rect x="16" y="16" width="6" height="5" rx="1.5" /><path d="M12 9v4m-7 3v-3h14v3" /></template>
    <template v-else-if="name === 'search'"><circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 5 5" /></template>
    <path v-else-if="name === 'list'" d="M8 5h13M8 12h13M8 19h13M3 5h.01M3 12h.01M3 19h.01" />
    <path v-else-if="name === 'plus'" d="M12 5v14M5 12h14" />
    <path v-else-if="name === 'close'" d="m6 6 12 12M6 18 18 6" />
    <path v-else-if="name === 'edit'" d="m15 4 5 5M4 20l5-1L21 7l-5-5L4 14v6Z" />
    <template v-else><circle cx="12" cy="12" r="9" /><path d="m16 8-2.5 5.5L8 16l2.5-5.5L16 8Z" /></template>
  </svg>
</template>
