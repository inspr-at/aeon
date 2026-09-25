<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { TYPES } from '../../lib/knowledge'
import { graphTypeTokens, type GraphNode } from '../../lib/knowledgeGraph'
const props = defineProps<{ nodes: GraphNode[] }>()
const items = computed(() => [...TYPES.map(t => ({ type: t.type, label: t.plural })), { type: 'ticket' as const, label: 'Tickets' }]
  .map(t => ({ ...t, count: props.nodes.filter(n => n.type === t.type).length })).filter(t => t.count))
</script>
<template>
  <div class="kg-legend" aria-label="Graph legend">
    <span v-for="item in items" :key="item.type"><i :style="{ background: `var(${graphTypeTokens[item.type]})` }" aria-hidden="true" />{{ item.label }}<small class="mono">{{ item.count }}</small></span>
    <span class="kg-size">Larger bubbles have more links</span>
  </div>
</template>
<style scoped>
.kg-legend { display: flex; flex-wrap: wrap; gap: 8px 18px; align-items: center; font-size: 11.5px; color: var(--ink-2); }
.kg-legend > span { display: inline-flex; gap: 6px; align-items: center; }
.kg-legend i { width: 7px; height: 7px; border-radius: 50%; }
.kg-legend small { margin-left: 1px; color: var(--ink-3); font-size: 10px; }
.kg-legend .kg-size { margin-left: auto; color: var(--ink-3); font-size: 11px; }
</style>
