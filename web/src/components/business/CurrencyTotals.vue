<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import MoneyAmount from './MoneyAmount.vue'
import './business.css'

defineProps<{ amounts: { currency: string; amount: string }[] }>()
</script>

<template>
  <div class="business-totals" role="list" aria-label="Amounts by currency">
    <p v-if="!amounts.length">No amounts in any currency.</p>
    <div v-for="(row, index) in amounts" :key="index" role="listitem">
      <MoneyAmount :amount="row.amount" :currency="row.currency" />
    </div>
  </div>
</template>
