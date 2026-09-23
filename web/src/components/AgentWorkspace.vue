<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import '../styles/agents.css'
defineProps<{ title: string; live: boolean; busy: boolean; error: string }>()
defineEmits<{ refresh: [] }>()
</script>
<template>
  <section class="agent-workspace" aria-labelledby="agent-title">
    <header class="agent-heading">
      <div><p class="eyebrow">Agent operations</p><h1 id="agent-title">{{ title }}</h1></div>
      <div class="agent-actions"><span role="status">{{ live ? 'Live updates connected' : 'Reconnecting · refresh every 30s' }}</span><button class="button secondary" :disabled="busy" @click="$emit('refresh')">Refresh</button></div>
    </header>
    <nav class="agent-nav" aria-label="Workspace sections">
      <RouterLink to="/">Work</RouterLink><RouterLink to="/agents">Agents</RouterLink><RouterLink to="/runs">Sessions &amp; runs</RouterLink><RouterLink to="/approvals">Approvals</RouterLink><RouterLink to="/pacing">Pacing</RouterLink>
    </nav>
    <p v-if="error" role="alert" class="error agent-notice">{{ error }}. Displayed data may be out of date. Use Refresh to retry.</p>
    <p v-if="busy" role="status">Updating…</p>
    <slot />
  </section>
</template>
