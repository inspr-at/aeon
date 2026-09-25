<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import type { GroupDef } from '../../lib/projectGroups'

// A group's marker: a small dot in the group's hue (never an edge). `hollow`
// draws it as a ring, which is how a hidden group shows in the chip row.
withDefaults(defineProps<{ group: Pick<GroupDef, 'kind' | 'hue'>; hollow?: boolean; size?: number }>(), { hollow: false, size: 8 })
</script>

<template>
  <span class="marker" :class="[group.hue ? `h-${group.hue}` : `k-${group.kind}`, { hollow }]" :style="{ '--dot': `${size}px` }" aria-hidden="true" />
</template>

<style scoped>
/* Hues: one lightness and chroma per theme, so every dot reads alike. */
.marker { --l: .6; --c: .11; --h: 190; flex-shrink: 0; width: var(--dot); height: var(--dot); border-radius: 50%; background: oklch(var(--l) var(--c) var(--h)); box-shadow: inset 0 0 0 1.5px oklch(var(--l) var(--c) var(--h)); }
.marker.hollow { background: transparent; }
.h-teal { --h: 190; } .h-gold { --h: 75; --c: .12; } .h-iris { --h: 290; } .h-rose { --h: 12; } .h-sage { --h: 150; --c: .09; }
.h-ocean { --h: 240; } .h-clay { --h: 45; } .h-plum { --h: 335; --c: .1; }
/* No group and Archived stay neutral. */
.k-none, .k-archived { background: var(--ink-3); box-shadow: inset 0 0 0 1.5px var(--ink-3); }
.k-none.hollow, .k-archived.hollow { background: transparent; }
:root[data-theme="dark"] .marker { --l: .78; --c: .1; }
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) .marker { --l: .78; --c: .1; } }
</style>
