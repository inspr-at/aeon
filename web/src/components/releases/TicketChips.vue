<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { brand } from '../../lib/brand'
import { useProjects } from '../../stores/projects'

// Ticket keys of a release. A key whose project lives here opens the ticket;
// the others (earlier trackers, other products) stay plain.
defineProps<{ tickets: string[] }>()
const projects = useProjects()
void projects.load()
function path(key: string) {
  const project = projects.byRouteKey(key.split('-')[0])
  return project ? `/p/${encodeURIComponent(project.routeKey)}/${encodeURIComponent(key)}` : ''
}
</script>

<template>
  <ul class="tickets" aria-label="Tickets">
    <li v-for="key in tickets" :key="key">
      <RouterLink v-if="path(key)" class="key-badge ticket-link" :to="path(key)" :aria-label="`Open ticket ${key}`">{{ key }}</RouterLink>
      <span v-else class="key-badge plain" :data-tip="`${key.split('-')[0]} is not a project in this ${brand.short_name} workspace`">{{ key }}</span>
    </li>
  </ul>
</template>

<style scoped>
.tickets { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; }
.ticket-link { height: 24px; transition: box-shadow .15s ease, transform .15s ease; }
@media (max-width: 600px) { .ticket-link { height: 44px; } }
@media (hover: hover) { .ticket-link:hover { box-shadow: inset 0 0 0 1px var(--teal), 0 4px 10px -6px rgba(14, 111, 108, .5); } }
.ticket-link:focus-visible { box-shadow: var(--focus-ring); }
.plain { height: 24px; background: var(--surface-2); box-shadow: inset 0 0 0 1px var(--line); color: var(--ink-2); }
</style>
