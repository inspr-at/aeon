<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { clearFatal, type Fatal } from '../lib/fatal'
import { absoluteTime } from '../lib/work'
import AppIcon from './AppIcon.vue'
import StatusPage from './StatusPage.vue'

// The page the global error boundary shows. Friendly, with a reference to quote;
// the technical line is folded away and never includes a stack.
const props = defineProps<{ error: Fatal }>()
const router = useRouter()
const update = computed(() => props.error.kind === 'update')
// A failed navigation retries its destination with a fresh load; anything else reloads.
function reload() { if (props.error.path) location.assign(props.error.path); else location.reload() }
async function home() { clearFatal(); await router.replace('/') }
</script>

<template>
  <StatusPage :eyebrow="update ? 'A newer version is ready' : 'Something went wrong'" :title="update ? 'PAIMOS AEON was updated.' : 'This page stumbled.'" tone="problem">
    <p v-if="update">A newer version was released while this page was open. Reload to continue; nothing you saved is lost.</p>
    <p v-else>The page ran into a problem it could not recover from. Your work is saved on the server; reloading usually helps.</p>
    <template #actions>
      <button type="button" class="btn primary" @click="reload"><AppIcon name="refresh" :size="14" />{{ update ? 'Reload' : 'Try again' }}</button>
      <button type="button" class="btn" @click="home"><AppIcon name="folder" :size="14" />Back to projects</button>
    </template>
    <template #details>
      <details class="details">
        <summary><AppIcon name="chevron-right" :size="12" class="chev" />Details for support</summary>
        <dl>
          <div><dt>Reference</dt><dd class="mono">{{ error.reference }}</dd></div>
          <div><dt>When</dt><dd>{{ absoluteTime(error.at) }}</dd></div>
          <div><dt>What</dt><dd class="mono">{{ error.name }}: {{ error.message }}</dd></div>
        </dl>
      </details>
    </template>
  </StatusPage>
</template>

<style scoped>
.details summary { display: inline-flex; align-items: center; gap: 6px; cursor: pointer; list-style: none; font-size: 12.5px; color: var(--ink-2); }
.details summary::-webkit-details-marker { display: none; }
.chev { color: var(--ink-3); transition: transform .15s ease; }
.details[open] .chev { transform: rotate(90deg); }
.details summary:focus-visible { box-shadow: var(--focus-ring); border-radius: 6px; }
dl { display: grid; gap: 4px; margin: 10px 0 0; font-size: 12.5px; }
dl div { display: grid; grid-template-columns: 80px minmax(0, 1fr); gap: 8px; }
dt { font: 500 10px/1.9 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
dd { margin: 0; color: var(--ink); overflow-wrap: anywhere; }
.mono { font-family: var(--mono); font-size: 11.5px; font-variant-ligatures: none; }
</style>
