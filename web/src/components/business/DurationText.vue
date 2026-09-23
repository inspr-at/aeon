<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { formatDuration } from './duration'
import './business.css'

const props = defineProps<{ seconds: number }>()
const text = computed(() => {
  try { return formatDuration(props.seconds) } catch { return '' }
})
</script>

<template>
  <span class="business-duration">
    <span v-if="text">{{ text }}</span>
    <span v-else class="error">Duration unavailable</span>
  </span>
</template>
