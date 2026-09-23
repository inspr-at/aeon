<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import type { RelatedNode } from '../../lib/useTicket'
import { statusMeta } from '../../lib/work'
import StatusIcon from './StatusIcon.vue'

// Blocks, blocked by and relates to: small key chips that open the other ticket.
const props = defineProps<{ related: RelatedNode[] }>()
const emit = defineEmits<{ open: [key: string] }>()
const ORDER = ['Blocked by', 'Blocks', 'Relates to', 'Duplicates', 'Duplicated by', 'Implements', 'Implemented by', 'Cites', 'Cited by']
const groups = computed(() => {
  const map = new Map<string, RelatedNode[]>()
  for (const entry of props.related) { if (!map.has(entry.label)) map.set(entry.label, []); map.get(entry.label)!.push(entry) }
  return [...map.entries()].sort((a, b) => ORDER.indexOf(a[0]) - ORDER.indexOf(b[0]))
})
</script>

<template>
  <section v-if="related.length" class="relations" aria-label="Relations">
    <h3 class="eyebrow">Relations</h3>
    <div v-for="[label, entries] in groups" :key="label" class="relation-group">
      <span class="relation-label">{{ label }}</span>
      <span class="chips">
        <template v-for="entry in entries" :key="entry.relation.id">
          <button v-if="entry.node" type="button" class="rel-chip" :class="{ closed: statusMeta(entry.node.state).closed, blocker: label === 'Blocked by' && !statusMeta(entry.node.state).closed }" :data-tip="`${entry.node.key} · ${statusMeta(entry.node.state).label}\n${entry.node.title}`" @click="emit('open', entry.node.key)">
            <StatusIcon :state="entry.node.state" :size="11" /><span>{{ entry.node.key }}</span>
          </button>
          <span v-else class="rel-chip missing" data-tip="This ticket is not available">Unavailable</span>
        </template>
      </span>
    </div>
  </section>
</template>

<style scoped>
.relations .eyebrow { margin: 0 0 8px; }
.relation-group { display: flex; align-items: baseline; gap: 12px; min-height: 30px; }
.relation-label { flex-shrink: 0; width: 96px; font-size: 12.5px; color: var(--ink-2); }
.chips { display: flex; flex-wrap: wrap; gap: 6px; }
.rel-chip { display: inline-flex; align-items: center; gap: 6px; height: 24px; padding: 0 9px 0 7px; border: 0; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); color: var(--ink); font: 500 11.5px/1 var(--mono); font-variant-ligatures: none; }
@media (hover: hover) { .rel-chip:hover { box-shadow: inset 0 0 0 1px var(--glass-rim); background: var(--row-hover); } }
.rel-chip:focus-visible { box-shadow: var(--focus-ring); }
.rel-chip.closed span { color: var(--ink-3); text-decoration: line-through; text-decoration-color: var(--line-2); }
.rel-chip.blocker { box-shadow: inset 0 0 0 1px var(--danger-line); }
.rel-chip.missing { color: var(--ink-3); font-family: var(--font); }
</style>
