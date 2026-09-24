<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { useRoute } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import StatusPage from '../components/StatusPage.vue'
import { run } from '../lib/commands'
const route = useRoute()
function search() { run({ name: 'palette' }) }
</script>

<template>
  <StatusPage eyebrow="404 · Page not found" title="A little off the path.">
    <p>Nothing lives at <code class="path">{{ route.fullPath }}</code>. It may have moved, or the link was cut short.</p>
    <template #actions>
      <RouterLink class="btn primary" to="/"><AppIcon name="folder" :size="14" />Back to projects</RouterLink>
      <button type="button" class="btn" @click="search"><AppIcon name="search" :size="14" />Search</button>
    </template>
  </StatusPage>
</template>

<style scoped>
.path { padding: 1px 6px; border-radius: 5px; background: var(--code-bg); font-size: 12.5px; color: var(--ink); overflow-wrap: anywhere; }
</style>
