<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
// The hidden groups, folded into one quiet line at the end:
// "Hidden: Paused 3 · Archived 1 · show". A name shows that group; show shows all.
// While a filter is on, it counts the matches the hidden groups hold.
defineProps<{ items: { id: string; name: string; count: number }[]; filtering: boolean }>()
const emit = defineEmits<{ show: [ids: string[]] }>()
</script>

<template>
  <p v-if="items.length" class="hidden-line">
    <span class="lead">{{ filtering ? 'Also matching in hidden groups:' : 'Hidden:' }}</span>
    <template v-for="(item, index) in items" :key="item.id">
      <span v-if="index" class="sep" aria-hidden="true">·</span>
      <button type="button" class="hidden-group" :aria-label="`Show ${item.name}, ${item.count} ${item.count === 1 ? 'project' : 'projects'}`" @click="emit('show', [item.id])">{{ item.name }} <span class="mono">{{ item.count }}</span></button>
    </template>
    <span class="sep" aria-hidden="true">·</span>
    <button type="button" class="show-all" :aria-label="items.length === 1 ? `Show ${items[0].name}` : 'Show all hidden groups'" @click="emit('show', items.map(item => item.id))">show</button>
  </p>
</template>

<style scoped>
.hidden-line { display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 2px 6px; margin: 0; padding: 14px 16px 16px; font-size: 12.5px; color: var(--ink-3); }
.lead { margin-right: 2px; }
.sep { color: var(--ink-3); }
.hidden-group, .show-all { display: inline-flex; align-items: center; gap: 5px; height: 26px; padding: 0 6px; border: 0; border-radius: 6px; background: transparent; color: var(--ink-2); font-size: 12.5px; }
.hidden-group .mono { font-size: 11.5px; color: var(--ink-3); }
.show-all { color: var(--teal-ink); font-weight: 600; }
@media (hover: hover) { .hidden-group:hover, .show-all:hover { background: var(--row-hover); color: var(--ink); } }
.hidden-group:focus-visible, .show-all:focus-visible { box-shadow: var(--focus-ring); }
@media (max-width: 600px) { .hidden-group, .show-all { height: 36px; } }
</style>
