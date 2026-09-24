<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { formatAmount } from './money'

// An exact amount and its currency code. The number keeps tabular figures so
// columns of amounts line up; the code stays quiet beside it.
const props = defineProps<{ amount: string; currency: string; signed?: boolean; strong?: boolean }>()
const text = computed(() => { try { return formatAmount(props.amount, props.currency, { signed: props.signed }) } catch { return '' } })
const tone = computed(() => !props.signed ? '' : text.value.startsWith('+') ? 'up' : text.value.startsWith('−') ? 'down' : 'flat')
</script>

<template>
  <span class="money" :class="[tone, { strong }]"><span v-if="text" class="amount">{{ text }}</span><span v-else class="unavailable">—</span> <span class="cur">{{ currency }}</span></span>
</template>

<style scoped>
.money { display: inline-flex; align-items: baseline; gap: 5px; white-space: nowrap; }
.amount { font-family: var(--mono); font-variant-numeric: tabular-nums; font-variant-ligatures: none; font-size: .95em; color: var(--ink); }
.strong .amount { font-weight: 650; }
.cur { font: 500 .72em/1 var(--mono); letter-spacing: .06em; color: var(--ink-3); font-variant-ligatures: none; }
.up .amount { color: var(--ok); }
.down .amount { color: var(--danger); }
.flat .amount { color: var(--ink-3); }
.unavailable { color: var(--ink-3); }
</style>
