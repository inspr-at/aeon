<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import BusinessPage from '../../components/business/BusinessPage.vue'
import BizIcon from '../../components/business/BizIcon.vue'
import AppIcon from '../../components/AppIcon.vue'
import { settingsLink } from '../../lib/settings'
import { useBusiness } from '../../stores/business'

// Quotes, honestly: what the page will hold, and that it arrives with the quote
// editor. No sample rows, no fake data.
const props = defineProps<{ part: 'quotes' }>()
const business = useBusiness()
const COPY = {
  quotes: {
    title: 'Quotes', icon: 'document' as const, heading: 'Quotes arrive with the quote editor',
    body: 'Offers with exact totals from your rates. Each version is kept as it was sent, and the customer’s acceptance is recorded on its own.',
    items: ['Positions priced from cost unit rates', 'Frozen versions you can compare and export', 'Issuing and acceptance, each on the record'],
    link: { label: 'Quote settings', to: settingsLink('business', 'quotes') },
  },
}
const copy = computed(() => COPY[props.part])
</script>

<template>
  <BusinessPage :title="copy.title">
    <template #summary>Not available yet.</template>
    <div class="arriving glass-card" :aria-labelledby="`${part}-arriving`" role="region">
      <span class="arriving-icon" aria-hidden="true"><BizIcon :name="copy.icon" :size="22" /></span>
      <h2 :id="`${part}-arriving`">{{ copy.heading }}</h2>
      <p class="body">{{ copy.body }}</p>
      <ul class="items" :aria-label="`What ${copy.title} will hold`">
        <li v-for="item in copy.items" :key="item"><AppIcon name="check" :size="14" />{{ item }}</li>
      </ul>
      <RouterLink v-if="business.admin" class="settings-link" :to="copy.link.to">{{ copy.link.label }}<AppIcon name="arrow" :size="14" /></RouterLink>
    </div>
  </BusinessPage>
</template>

<style scoped>
.arriving { display: grid; justify-items: center; gap: 10px; max-width: 620px; margin: 8px auto 0; padding: 40px 32px 32px; text-align: center; }
.arriving-icon { display: grid; place-items: center; width: 52px; height: 52px; margin-bottom: 4px; border-radius: 16px; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
h2 { font-size: 19px; text-wrap: balance; }
.body { max-width: 50ch; font-size: 14px; line-height: 1.6; color: var(--ink-2); }
.items { display: grid; gap: 8px; margin: 10px 0 4px; padding: 14px 18px; list-style: none; text-align: left; border-radius: 12px; background: var(--surface-2); }
.items li { display: flex; align-items: flex-start; gap: 10px; font-size: 13.5px; color: var(--ink); }
.items svg { margin-top: 3px; color: var(--teal-ink); }
.settings-link { display: inline-flex; align-items: center; gap: 6px; min-height: 44px; margin-top: 4px; padding: 0 10px; border-radius: 10px; color: var(--teal-ink); font-size: 13.5px; font-weight: 600; }
@media (hover: hover) { .settings-link:hover { background: var(--row-hover); } }
.settings-link:focus-visible { box-shadow: var(--focus-ring); }
@media (max-width: 600px) { .arriving { padding: 28px 18px 22px; } }
</style>
