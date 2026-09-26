<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import TicketLink from './TicketLink.vue'

// Ticket keys of a release: the ones this workspace has open beside the history.
defineProps<{ tickets: string[] }>()
</script>

<template>
  <ul class="tickets" aria-label="Tickets">
    <li v-for="key in tickets" :key="key"><TicketLink :ticket-key="key" /></li>
  </ul>
</template>

<style scoped>
.tickets { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; padding: 0; list-style: none; }
</style>
