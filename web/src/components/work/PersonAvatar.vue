<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { initials } from '../../lib/work'
const props = withDefaults(defineProps<{ name: string; size?: number }>(), { size: 20 })
const letters = computed(() => initials(props.name))
</script>

<template>
  <span class="avatar" :class="{ small: size < 22 }" :style="{ width: `${size}px`, height: `${size}px` }" aria-hidden="true">{{ letters }}</span>
</template>

<style scoped>
.avatar {
  display: inline-grid; place-items: center; flex-shrink: 0; border-radius: 50%;
  background: var(--avatar-bg); color: var(--teal-ink); box-shadow: 0 0 0 1px var(--glass-rim);
  font: 700 10px/1 var(--mono); letter-spacing: .02em; font-variant-ligatures: none;
}
.avatar.small { font-size: 8.5px; }
</style>
