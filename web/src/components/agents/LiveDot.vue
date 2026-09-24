<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import type { LiveTone } from '../../lib/agentState'

// Working pulses green, waiting on Markus pulses gold, idle is a green ring,
// a missing heartbeat a dashed ring, and a stopped session a small grey dot.
withDefaults(defineProps<{ tone: LiveTone; size?: number }>(), { size: 10 })
</script>

<template>
  <span class="live-dot" :class="tone" :style="{ '--d': `${size}px` }" aria-hidden="true" />
</template>

<style scoped>
.live-dot { position: relative; display: inline-block; flex-shrink: 0; width: var(--d); height: var(--d); border-radius: 50%; }
.busy { background: var(--ok); }
.attention { background: var(--gold); }
.idle { box-shadow: inset 0 0 0 2px var(--ok); }
.quiet { border: 1.6px dashed var(--st-backlog); }
.stopped { transform: scale(.7); background: var(--st-closed); }
@media (prefers-reduced-motion: no-preference) {
  .busy::after, .attention::after {
    content: ''; position: absolute; inset: 0; border-radius: 50%; background: inherit; opacity: .45;
    animation: live-pulse 1.8s cubic-bezier(.2, .7, .2, 1) infinite;
  }
  @keyframes live-pulse { from { transform: scale(1); opacity: .45; } to { transform: scale(2.4); opacity: 0; } }
}
</style>
