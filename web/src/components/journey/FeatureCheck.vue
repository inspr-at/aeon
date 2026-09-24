<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import type { Selection } from '../../lib/journey'

// A feature's tri-state box: all, some or none of its open tickets are in the
// release. Clicking cycles all → none → the last partial pick → all.
const props = defineProps<{ state: Selection; label: string; included: number; total: number; next: string; disabled?: boolean }>()
const emit = defineEmits<{ toggle: [] }>()
const checked = computed(() => props.state === 'all' ? 'true' : props.state === 'some' ? 'mixed' : 'false')
</script>

<template>
  <button
    type="button" class="fcb" :class="state" role="checkbox" :aria-checked="checked" :disabled="disabled"
    :aria-label="`${label}: ${included} of ${total} in the release. Click to ${next}.`" :data-tip="disabled ? `${included} of ${total} in the release` : `${included} of ${total} in the release · click to ${next}`"
    @click.stop="emit('toggle')"
  >
    <svg v-if="state === 'all'" viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m3.5 8.5 3 3 6-7" /></svg>
    <svg v-else-if="state === 'some'" viewBox="0 0 16 16" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" aria-hidden="true"><path d="M4 8h8" /></svg>
  </button>
</template>

<style scoped>
.fcb {
  display: inline-grid; place-items: center; flex-shrink: 0; width: 16px; height: 16px; padding: 0; border: 0; border-radius: 4px;
  background: var(--surface); color: var(--button-ink); box-shadow: inset 0 0 0 1.5px var(--line-2); cursor: pointer;
}
.fcb.all, .fcb.some { background: var(--teal); box-shadow: none; }
.fcb:hover:not(:disabled), .fcb:focus-visible { box-shadow: inset 0 0 0 1.5px var(--line-2), 0 0 0 3px color-mix(in oklab, var(--aqua) 60%, transparent); }
.fcb.all:hover:not(:disabled), .fcb.some:hover:not(:disabled), .fcb.all:focus-visible, .fcb.some:focus-visible { box-shadow: 0 0 0 3px color-mix(in oklab, var(--aqua) 60%, transparent); }
.fcb:disabled { cursor: default; opacity: .7; }
.fcb.empty { visibility: hidden; }
</style>
