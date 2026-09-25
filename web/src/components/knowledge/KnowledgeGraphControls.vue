<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import AppIcon from '../AppIcon.vue'
import type { GraphDimension } from '../../lib/knowledgeGraph'
defineProps<{ dimension: GraphDimension; paused: boolean; tickets: boolean; fallback: boolean; reduced: boolean }>()
const emit = defineEmits<{ fit: []; dimension: [value: GraphDimension]; pause: []; tickets: [] }>()
</script>
<template>
  <div class="kg-controls" role="group" aria-label="Graph controls">
    <button type="button" class="btn sm" aria-label="Fit graph to view" data-tip="Fit to view" @click="emit('fit')"><AppIcon name="expand" :size="14" /><span>Fit</span></button>
    <div class="seg" role="group" aria-label="Graph dimensions">
      <button type="button" :aria-pressed="dimension === '2d'" @click="emit('dimension', '2d')">2D</button>
      <button type="button" :aria-pressed="dimension === '3d'" :disabled="fallback" :title="fallback ? '3D is unavailable in this browser' : 'Explore in three dimensions'" @click="emit('dimension', '3d')">3D</button>
    </div>
    <button type="button" class="btn sm kg-motion" :aria-pressed="paused" :disabled="reduced" :aria-label="paused ? 'Resume motion' : 'Pause motion'" :title="reduced ? 'Motion is reduced by your device preference' : undefined" @click="emit('pause')"><AppIcon :name="paused ? 'arrow' : 'pause'" :size="14" /><span>{{ paused ? 'Still' : 'Motion' }}</span></button>
    <button type="button" class="btn sm" :aria-pressed="tickets" aria-label="Show linked tickets" @click="emit('tickets')"><AppIcon name="ticket" :size="14" /><span>Tickets</span></button>
  </div>
</template>
<style scoped>
.kg-controls { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.seg button { min-width: 34px; height: 28px; justify-content: center; padding: 0 9px; }
.seg button[aria-pressed="true"] { background: var(--seg-on); box-shadow: var(--shadow-btn); color: var(--teal-ink); }
.btn[aria-pressed="true"] { background: var(--row-selected); box-shadow: inset 0 0 0 1px var(--chip-teal-line); }
@media (max-width: 600px) { .kg-controls { gap: 6px; } .kg-controls .btn, .seg button { min-height: 40px; } .kg-motion span { display: none; } }
</style>
