<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script lang="ts">
// One inline SVG set: 16×16 grid, stroke 1.8, round caps and joins, currentColor.
export type IconName =
  | 'sun' | 'moon' | 'user' | 'chevron' | 'chevron-right' | 'chevron-left' | 'chevron-up' | 'arrow' | 'arrow-up' | 'arrow-down'
  | 'logout' | 'tree' | 'compass' | 'search' | 'list' | 'plus' | 'close' | 'edit' | 'copy' | 'external' | 'filter' | 'check'
  | 'rows-comfortable' | 'rows-compact' | 'keyboard' | 'epic' | 'ticket' | 'task' | 'alert' | 'refresh' | 'sliders' | 'enter'
  | 'inbox' | 'folder' | 'layers' | 'archive' | 'clock'
</script>
<script setup lang="ts">
withDefaults(defineProps<{ name: IconName; size?: number }>(), { size: 16 })
</script>

<template>
  <svg :width="size" :height="size" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
    <template v-if="name === 'sun'"><circle cx="8" cy="8" r="2.8" /><path d="M8 1.6v1.3M8 13.1v1.3M1.6 8h1.3M13.1 8h1.3M3.5 3.5l.9.9M11.6 11.6l.9.9M3.5 12.5l.9-.9M11.6 4.4l.9-.9" /></template>
    <path v-else-if="name === 'moon'" d="M13.6 9.6A5.9 5.9 0 0 1 6.4 2.4a6 6 0 1 0 7.2 7.2Z" />
    <template v-else-if="name === 'user'"><circle cx="8" cy="5.4" r="2.6" /><path d="M2.8 14c0-2.9 2.3-4.6 5.2-4.6s5.2 1.7 5.2 4.6" /></template>
    <path v-else-if="name === 'chevron'" d="m4.5 6.3 3.5 3.5 3.5-3.5" />
    <path v-else-if="name === 'chevron-right'" d="m6.3 4.5 3.5 3.5-3.5 3.5" />
    <path v-else-if="name === 'chevron-left'" d="m9.7 4.5-3.5 3.5 3.5 3.5" />
    <path v-else-if="name === 'chevron-up'" d="m4.5 9.7 3.5-3.5 3.5 3.5" />
    <path v-else-if="name === 'arrow'" d="M2.8 8h10.4M9.2 4l4 4-4 4" />
    <path v-else-if="name === 'arrow-up'" d="M8 13.2V2.8M4 6.8l4-4 4 4" />
    <path v-else-if="name === 'arrow-down'" d="M8 2.8v10.4M4 9.2l4 4 4-4" />
    <path v-else-if="name === 'logout'" d="M6 2.5H3.3a.8.8 0 0 0-.8.8v9.4c0 .4.4.8.8.8H6M10.2 5l3 3-3 3M5.8 8h7.4" />
    <template v-else-if="name === 'tree'"><rect x="5.8" y="1.6" width="4.4" height="3.8" rx="1.1" /><rect x="1.6" y="10.8" width="4.2" height="3.6" rx="1.1" /><rect x="10.2" y="10.8" width="4.2" height="3.6" rx="1.1" /><path d="M8 5.4v2.4M3.7 10.8V8.6h8.6v2.2" /></template>
    <template v-else-if="name === 'search'"><circle cx="7" cy="7" r="4.4" /><path d="m10.3 10.3 3.5 3.5" /></template>
    <path v-else-if="name === 'list'" d="M5.6 4h8M5.6 8h8M5.6 12h8M2.4 4h.01M2.4 8h.01M2.4 12h.01" />
    <path v-else-if="name === 'plus'" d="M8 3v10M3 8h10" />
    <path v-else-if="name === 'close'" d="m4.2 4.2 7.6 7.6M11.8 4.2l-7.6 7.6" />
    <path v-else-if="name === 'edit'" d="m10.4 2.6 3 3L6 13l-3.6.6L3 10Z" />
    <template v-else-if="name === 'copy'"><rect x="5.6" y="5.6" width="7.8" height="7.8" rx="1.8" /><path d="M10.4 5.6V4a1.4 1.4 0 0 0-1.4-1.4H4A1.4 1.4 0 0 0 2.6 4v5c0 .8.6 1.4 1.4 1.4h1.6" /></template>
    <path v-else-if="name === 'external'" d="M9.4 2.6h4v4M13.4 2.6 7.8 8.2M12 9.6v2.6c0 .7-.6 1.2-1.2 1.2H3.8c-.7 0-1.2-.5-1.2-1.2V5.2c0-.7.5-1.2 1.2-1.2h2.6" />
    <path v-else-if="name === 'filter'" d="M2.4 3.4h11.2L9.3 8.6v4.2l-2.6 1V8.6Z" />
    <path v-else-if="name === 'check'" d="m3.4 8.4 3 3 6.2-6.8" />
    <template v-else-if="name === 'rows-comfortable'"><rect x="2" y="2.6" width="12" height="4.4" rx="1.3" /><rect x="2" y="9" width="12" height="4.4" rx="1.3" /></template>
    <path v-else-if="name === 'rows-compact'" d="M2.4 2.8h11.2M2.4 6.2h11.2M2.4 9.8h11.2M2.4 13.2h11.2" />
    <template v-else-if="name === 'keyboard'"><rect x="1.4" y="3.8" width="13.2" height="8.6" rx="1.8" /><path d="M4.4 6.8h.01M6.8 6.8h.01M9.2 6.8h.01M11.6 6.8h.01M5 9.6h6" /></template>
    <path v-else-if="name === 'epic'" d="M9 1.8 3.6 9.2h4.1l-.9 5 5.6-7.4H8.2Z" />
    <template v-else-if="name === 'ticket'"><rect x="2.6" y="2.4" width="10.8" height="11.2" rx="2" /><path d="M5.4 6h5.2M5.4 8.6h5.2M5.4 11.2h2.8" /></template>
    <template v-else-if="name === 'task'"><rect x="1.8" y="1.8" width="6" height="4.6" rx="1.3" /><path d="M4.8 6.4v4.3c0 .5.4.9.9.9h1.9" /><rect x="7.6" y="9.4" width="6.6" height="4.6" rx="1.3" /></template>
    <template v-else-if="name === 'alert'"><circle cx="8" cy="8" r="6.2" /><path d="M8 4.8v3.6M8 11.1h.01" /></template>
    <path v-else-if="name === 'refresh'" d="M13.2 8a5.2 5.2 0 1 1-1.6-3.8M13.4 2.4v3.2h-3.2" />
    <template v-else-if="name === 'sliders'"><path d="M2.4 4.6h6.6M12.6 4.6h1M2.4 11.4h1M7 11.4h6.6" /><circle cx="10.8" cy="4.6" r="1.7" /><circle cx="5.2" cy="11.4" r="1.7" /></template>
    <path v-else-if="name === 'enter'" d="M13 3.4v4.4c0 .9-.7 1.6-1.6 1.6H3.2M6 6.6 3.2 9.4 6 12.2" />
    <template v-else-if="name === 'inbox'"><path d="M2 9.4 4.2 3.6h7.6L14 9.4v3a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1Z" /><path d="M2 9.4h3.3l1 1.5h3.4l1-1.5H14" /></template>
    <path v-else-if="name === 'folder'" d="M2 4.6c0-.6.4-1 1-1h3.2l1.5 1.5H13c.6 0 1 .4 1 1V12c0 .6-.4 1-1 1H3c-.6 0-1-.4-1-1Z" />
    <path v-else-if="name === 'layers'" d="M8 2 14 5 8 8 2 5ZM2 8.2l6 3 6-3M2 11.2l6 3 6-3" />
    <template v-else-if="name === 'archive'"><rect x="1.8" y="2.8" width="12.4" height="3.4" rx="1" /><path d="M3 6.2v6.2c0 .6.4 1 1 1h8c.6 0 1-.4 1-1V6.2M6.4 9h3.2" /></template>
    <template v-else-if="name === 'clock'"><circle cx="8" cy="8" r="6.2" /><path d="M8 4.6V8l2.2 1.4" /></template>
    <template v-else><circle cx="8" cy="8" r="6.2" /><path d="M10.6 5.4 9.2 9.2l-3.8 1.4 1.4-3.8Z" /></template>
  </svg>
</template>
