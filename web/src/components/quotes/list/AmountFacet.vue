<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import { amountBounds, type QuoteFilter } from '../../../lib/quotes/list'
import { minorMoney } from '../../../lib/crm'
import BizIcon from '../../business/BizIcon.vue'
import FloatingPanel from '../../work/FloatingPanel.vue'

// The net-amount filter: a lower bound, an upper bound or both, typed as you
// would write the amount. A bound that is not an amount says so and is ignored.
const props = defineProps<{ value: QuoteFilter['amount'] }>()
const emit = defineEmits<{ change: [value: QuoteFilter['amount']] }>()
const anchor = ref<HTMLElement | null>(null)
const button = ref<HTMLButtonElement>()
const bounds = computed(() => amountBounds(props.value))
const active = computed(() => bounds.value.min !== null || bounds.value.max !== null)
const money = (minor: number) => minorMoney(minor, '')
const label = computed(() => {
  const { min, max } = bounds.value
  if (min !== null && max !== null) return `${money(min)} – ${money(max)}`
  if (min !== null) return `From ${money(min)}`
  if (max !== null) return `Up to ${money(max)}`
  return 'Amount'
})
const minBad = computed(() => amountBounds({ min: props.value.min, max: '' }).invalid)
const maxBad = computed(() => amountBounds({ min: '', max: props.value.max }).invalid)
const swapped = computed(() => bounds.value.min !== null && bounds.value.max !== null && bounds.value.min > bounds.value.max)
function toggleOpen(event: MouseEvent) { anchor.value = anchor.value ? null : event.currentTarget as HTMLElement }
function close(restore: boolean) { anchor.value = null; if (restore) button.value?.focus() }
function set(which: 'min' | 'max', event: Event) { emit('change', { ...props.value, [which]: (event.target as HTMLInputElement).value }) }
</script>

<template>
  <button ref="button" type="button" class="btn sm facet-btn" :class="{ on: active }" aria-haspopup="dialog" :aria-expanded="!!anchor" @click="toggleOpen">
    <span class="facet-label">{{ label }}</span><BizIcon name="chevron" :size="12" class="facet-chevron" />
  </button>
  <FloatingPanel v-if="anchor" :anchor="anchor" :width="280" label="Filter by net amount" @close="close">
    <div class="facet-head">
      <p class="eyebrow">Net amount</p>
      <button v-if="active || value.min || value.max" type="button" class="clear" @click="emit('change', { min: '', max: '' })">Clear</button>
    </div>
    <div class="bounds">
      <label class="bound"><span>At least</span><input class="field" inputmode="decimal" autocomplete="off" placeholder="0.00" :value="value.min" :aria-invalid="minBad" aria-describedby="amount-note" data-autofocus @input="set('min', $event)" /></label>
      <label class="bound"><span>At most</span><input class="field" inputmode="decimal" autocomplete="off" placeholder="No limit" :value="value.max" :aria-invalid="maxBad" aria-describedby="amount-note" @input="set('max', $event)" /></label>
    </div>
    <p v-if="minBad || maxBad" class="bad" role="alert">Type an amount such as 1500 or 1,500.50.</p>
    <p v-else-if="swapped" class="bad" role="alert">The lower bound is above the upper one, so nothing matches.</p>
    <p id="amount-note" class="note">Before tax, in each quote’s own currency. Quotes without positions have no amount and are left out while a bound is set.</p>
  </FloatingPanel>
</template>

<style scoped>
.facet-btn { gap: 6px; max-width: 260px; padding: 0 9px 0 12px; font-weight: 600; color: var(--ink-2); }
.facet-btn:hover, .facet-btn[aria-expanded="true"] { color: var(--ink); }
.facet-btn.on { color: var(--teal-ink); }
.facet-label { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-variant-numeric: tabular-nums; }
.facet-chevron { flex-shrink: 0; color: var(--ink-3); }
.facet-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-height: 26px; padding: 2px 6px 4px 10px; }
.clear { height: 24px; padding: 0 8px; border: 0; border-radius: 999px; background: transparent; color: var(--teal-ink); font-size: 12px; font-weight: 600; }
.clear:hover { background: var(--row-selected); }
.bounds { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; padding: 2px 10px; }
.bound { display: grid; gap: 4px; font-size: 12px; color: var(--ink-2); }
.bound .field { height: 32px; padding: 0 9px; font: 500 13px/1 var(--mono); font-variant-numeric: tabular-nums; }
.bound .field[aria-invalid="true"] { box-shadow: inset 0 0 0 1px var(--danger-line); }
.bad { margin: 8px 10px 0; font-size: 12px; color: var(--danger); }
.note { padding: 8px 10px 4px; font-size: 12px; line-height: 1.45; color: var(--ink-3); }
</style>
