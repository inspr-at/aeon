<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BusinessPage from '../../components/business/BusinessPage.vue'
import BizIcon from '../../components/business/BizIcon.vue'
import AppIcon from '../../components/AppIcon.vue'
import { settingsLink } from '../../lib/settings'
import { useBusiness } from '../../stores/business'
import { api } from '../../lib/api'

// Customers remain parked; Quotes shows the live list or an empty state.
const props = defineProps<{ part: 'customers' | 'quotes' }>()
const business = useBusiness()
interface QuoteRow { quote_node_id: string; offer_no?: string; state: string }
const quotes = ref<QuoteRow[]>([])
const quotesLoaded = ref(false)
const quotesLoading = ref(false)
const quotesError = ref('')
const nextCursor = ref('')
async function loadQuotes(reset = false) {
  if (quotesLoading.value) return
  if (reset) { quotes.value = []; nextCursor.value = ''; quotesLoaded.value = false }
  quotesLoading.value = true
  try {
    const response = await api(`/quotes?limit=200${nextCursor.value ? `&cursor=${encodeURIComponent(nextCursor.value)}` : ''}`)
    if (!response.ok) throw new Error('Quotes could not be loaded.')
    const rows: unknown = await response.json()
    quotes.value = [...quotes.value, ...(Array.isArray(rows) ? rows as QuoteRow[] : [])]
    nextCursor.value = response.headers.get('X-Next-Cursor') ?? ''
    quotesError.value = ''
  } catch (cause) { quotesError.value = cause instanceof Error ? cause.message : 'Quotes could not be loaded.' }
  finally { quotesLoaded.value = true; quotesLoading.value = false }
}
watch(() => [props.part, business.open.quotes], () => {
  if (props.part === 'quotes' && business.open.quotes) void loadQuotes(true)
}, { immediate: true })
const COPY = {
  customers: {
    title: 'Customers', icon: 'building' as const, heading: 'Customers arrive with the CRM port',
    body: 'Organisations and the people you work with there, in one place with the projects and quotes that belong to them.',
    items: ['Organisations with their address and website', 'Contacts with email, phone and role', 'Each customer’s projects, quotes and hours at a glance'],
    link: { label: 'Business settings', to: settingsLink('business', 'customers') },
  },
  quotes: {
    title: 'Quotes', icon: 'document' as const, heading: 'No quotes yet',
    body: 'Draft and issued quotes will appear here.',
    items: [] as string[],
    link: { label: 'Quote settings', to: settingsLink('business', 'quotes') },
  },
}
const copy = computed(() => COPY[props.part])
</script>

<template>
  <BusinessPage :title="copy.title">
    <template #summary>{{ part === 'quotes' ? 'Drafts and issued offers.' : 'Not available yet.' }}</template>
    <div v-if="part === 'quotes' && quotes.length" class="quote-list glass-card" role="region" aria-label="Quotes">
      <h2>Quotes</h2>
      <ul>
        <li v-for="quote in quotes" :key="quote.quote_node_id">
          <RouterLink :to="`/business/quotes/${encodeURIComponent(quote.quote_node_id)}`">{{ quote.offer_no || 'Draft quote' }}</RouterLink>
          <span>{{ quote.state }}</span>
        </li>
      </ul>
      <button v-if="nextCursor" type="button" class="quote-more" :disabled="quotesLoading" @click="loadQuotes()">{{ quotesLoading ? 'Loading…' : 'Load more' }}</button>
    </div>
    <div v-else-if="part === 'quotes' && quotesError" class="arriving glass-card" role="alert">{{ quotesError }}</div>
    <div v-else class="arriving glass-card" :aria-labelledby="`${part}-arriving`" role="region">
      <span class="arriving-icon" aria-hidden="true"><BizIcon :name="copy.icon" :size="22" /></span>
      <h2 :id="`${part}-arriving`">{{ copy.heading }}</h2>
      <p class="body">{{ copy.body }}</p>
      <ul v-if="copy.items.length" class="items" :aria-label="`What ${copy.title} will hold`">
        <li v-for="item in copy.items" :key="item"><AppIcon name="check" :size="14" />{{ item }}</li>
      </ul>
      <RouterLink v-if="business.admin" class="settings-link" :to="copy.link.to">{{ copy.link.label }}<AppIcon name="arrow" :size="14" /></RouterLink>
    </div>
  </BusinessPage>
</template>

<style scoped>
.arriving { display: grid; justify-items: center; gap: 10px; max-width: 620px; margin: 8px auto 0; padding: 40px 32px 32px; text-align: center; }
.quote-list { max-width: 800px; margin: 8px auto; padding: 20px; }
.quote-list ul { margin: 12px 0 0; padding: 0; list-style: none; }
.quote-list li { display: flex; justify-content: space-between; gap: 16px; padding: 12px 0; border-bottom: 1px solid var(--line-2); }
.quote-more { margin-top: 16px; border: 1px solid var(--line-2); border-radius: 8px; background: var(--surface-raised); color: var(--ink); padding: 8px 12px; cursor: pointer; }
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
