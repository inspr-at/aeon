<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script lang="ts">
import type { BizIconName } from '../../business/BizIcon.vue'
// Quote editor glyphs in the app's set: 16×16 grid, stroke 1.8, round caps and
// joins, currentColor, centred. Any other name is drawn by BizIcon/AppIcon.
export type QuoteGlyph = 'bold' | 'italic' | 'clear-format' | 'list-none' | 'list-bullet' | 'list-numbered' | 'indent' | 'outdent'
  | 'undo' | 'redo' | 'sidebar' | 'section-add' | 'page-break' | 'fit-width' | 'fit-page' | 'list-continue' | 'list-restart' | 'list-follow' | 'reset' | 'arrow-up' | 'arrow-down' | 'save' | 'check-circle'
export type QuoteIconName = QuoteGlyph | BizIconName
const GLYPHS = new Set<string>(['bold', 'italic', 'clear-format', 'list-none', 'list-bullet', 'list-numbered', 'indent', 'outdent', 'undo', 'redo', 'sidebar', 'section-add', 'page-break', 'fit-width', 'fit-page', 'list-continue', 'list-restart', 'list-follow', 'reset', 'arrow-up', 'arrow-down', 'save', 'check-circle'])
</script>
<script setup lang="ts">
import BizIcon from '../../business/BizIcon.vue'
withDefaults(defineProps<{ name: QuoteIconName; size?: number }>(), { size: 16 })
</script>

<template>
  <svg v-if="GLYPHS.has(name)" :width="size" :height="size" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">
    <path v-if="name === 'bold'" d="M4.6 2.8h4.1a2.55 2.55 0 0 1 0 5.1H4.6ZM4.6 7.9h4.8a2.65 2.65 0 0 1 0 5.3H4.6Z" />
    <path v-else-if="name === 'italic'" d="M6.8 2.8h4.4M4.8 13.2h4.4M9.6 2.8 6.4 13.2" />
    <path v-else-if="name === 'clear-format'" d="M3 3h7.4M6.9 3 5.4 10.4M9.8 9.8l3.4 3.4M13.2 9.8l-3.4 3.4" />
    <path v-else-if="name === 'list-none'" d="M2.8 4h10.4M2.8 8h10.4M2.8 12h6.4" />
    <template v-else-if="name === 'list-bullet'"><circle cx="3.4" cy="4.2" r=".9" fill="currentColor" stroke="none" /><circle cx="3.4" cy="8" r=".9" fill="currentColor" stroke="none" /><circle cx="3.4" cy="11.8" r=".9" fill="currentColor" stroke="none" /><path d="M6.4 4.2h6.8M6.4 8h6.8M6.4 11.8h6.8" /></template>
    <path v-else-if="name === 'list-numbered'" d="M2.4 2.9h1.1v3.3M2.2 9.3c.3-.5.8-.7 1.3-.7.6 0 1 .4 1 .9 0 .9-2.3 1.5-2.3 2.8h2.4M6.8 4.4h6.6M6.8 11.4h6.6" />
    <path v-else-if="name === 'indent'" d="M7 3.6h6.4M7 8h6.4M7 12.4h6.4M2.6 5.6 5 8l-2.4 2.4" />
    <path v-else-if="name === 'outdent'" d="M7 3.6h6.4M7 8h6.4M7 12.4h6.4M5 5.6 2.6 8 5 10.4" />
    <path v-else-if="name === 'undo'" d="M5.8 3.4 3.2 6l2.6 2.6M3.4 6h6.4a3.2 3.2 0 0 1 0 6.4H7.2" />
    <path v-else-if="name === 'redo'" d="M10.2 3.4 12.8 6l-2.6 2.6M12.6 6H6.2a3.2 3.2 0 0 0 0 6.4h2.6" />
    <template v-else-if="name === 'sidebar'"><rect x="2" y="2.8" width="12" height="10.4" rx="1.8" /><path d="M10 2.8v10.4" /></template>
    <path v-else-if="name === 'section-add'" d="M2.8 3.2h10.4M2.8 6.4h6.4M2.8 9.6h3.6M11.2 8.4v5.2M8.6 11h5.2" />
    <path v-else-if="name === 'page-break'" d="M3.4 2.2v3.4h9.2V2.2M3.4 13.8v-3.4h9.2v3.4M1.8 8h1.6M5.6 8h1.6M8.8 8h1.6M12.6 8h1.6" />
    <path v-else-if="name === 'fit-width'" d="M2.2 8h11.6M4.6 5.6 2.2 8l2.4 2.4M11.4 5.6 13.8 8l-2.4 2.4" />
    <path v-else-if="name === 'fit-page'" d="M4 2.2h8c.4 0 .8.4.8.8v10c0 .4-.4.8-.8.8H4c-.4 0-.8-.4-.8-.8V3c0-.4.4-.8.8-.8ZM6 6 8 4l2 2M6 10l2 2 2-2" />
    <path v-else-if="name === 'list-continue'" d="M2.8 3.4h7M2.8 6.8h4.4M9.6 6.2c1.9 0 3.4 1.3 3.4 3s-1.5 3.2-3.4 3.2H6.4M8.2 10.6l-1.8 1.8 1.8 1.8" />
    <path v-else-if="name === 'list-follow'" d="M2.8 3.4h6M2.8 8h6M2.8 12.6h6M12.2 3v9.4M10.4 10.6l1.8 1.8 1.8-1.8" />
    <path v-else-if="name === 'list-restart'" d="M3.2 8a4.8 4.8 0 1 0 1.5-3.5M3 2.6v2.8h2.8M7.2 6.4h1v3.2" />
    <path v-else-if="name === 'reset'" d="M3.2 7.6a4.8 4.8 0 1 1 1.4 3.6M3 11.8V9h2.8" />
    <path v-else-if="name === 'arrow-up'" d="M8 13V3M4.2 6.8 8 3l3.8 3.8" />
    <path v-else-if="name === 'save'" d="M3.6 2.6h7.6l2.2 2.2v7.8c0 .4-.4.8-.8.8H3.6c-.4 0-.8-.4-.8-.8V3.4c0-.4.4-.8.8-.8ZM5.4 2.6v3h4.4v-3M5 13.4V9.6h6v3.8" />
    <template v-else-if="name === 'check-circle'"><circle cx="8" cy="8" r="5.8" /><path d="m5.6 8.1 1.7 1.7 3.2-3.3" /></template>
    <path v-else d="M8 3v10M4.2 9.2 8 13l3.8-3.8" />
  </svg>
  <BizIcon v-else :name="name as BizIconName" :size="size" />
</template>
