<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
defineProps<{ name: 'left' | 'right' | 'check' | 'minus' | 'close' | 'search' | 'expand' | 'external' | 'info' | 'compare' | 'zoom' | 'keys' | 'refresh' }>()
</script>
<template>
  <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
    <path v-if="name === 'left'" d="m14 6-6 6 6 6" /><path v-else-if="name === 'right'" d="m10 6 6 6-6 6" />
    <path v-else-if="name === 'check'" d="m5 12 4 4L19 6" /><path v-else-if="name === 'minus'" d="M5 12h14" />
    <path v-else-if="name === 'close'" d="m6 6 12 12M6 18 18 6" />
    <template v-else-if="name === 'search'"><circle cx="10" cy="10" r="6" /><path d="m15 15 5 5" /></template>
    <path v-else-if="name === 'expand'" d="M4 9V4h5m6 0h5v5M4 15v5h5m6 0h5v-5" />
    <path v-else-if="name === 'external'" d="M14 3h7v7m0-7L10 14m0-9H4v15h15v-6" />
    <template v-else-if="name === 'info'"><circle cx="12" cy="12" r="9" /><path d="M12 11v6m0-10v.01" /></template>
    <template v-else-if="name === 'compare'"><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M12 2v20" /></template>
    <template v-else-if="name === 'zoom'"><circle cx="10" cy="10" r="6" /><path d="m15 15 5 5M7 10h6m-3-3v6" /></template>
    <template v-else-if="name === 'keys'"><rect x="2" y="5" width="20" height="14" rx="3" /><path d="M6 9h.01M10 9h.01M14 9h.01M18 9h.01M6 13h.01M10 13h.01M14 13h.01M18 13h.01M8 16h8" /></template>
    <path v-else d="M20 8a8 8 0 1 0 0 8M20 3v5h-5" />
  </svg>
</template>
