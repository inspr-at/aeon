<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { formatDecimal, formatMinor } from './money'
import './business.css'

const props = defineProps<{
  currency: string
  amount?: string
  minor?: string
  scale?: number
}>()

const text = computed(() => {
  if ((props.amount === undefined) === (props.minor === undefined)) return ''
  try {
    if (props.minor !== undefined) return formatMinor(props.minor, props.currency, props.scale)
    return formatDecimal(props.amount ?? '', props.currency)
  } catch {
    return ''
  }
})
</script>

<template>
  <span class="business-money">
    <span v-if="text">{{ text }}</span>
    <span v-else class="error">Amount unavailable</span>
  </span>
</template>
