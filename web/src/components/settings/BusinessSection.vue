<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getQuoteSettings, senderLine, statusOf, type QuoteSettings } from '../../lib/settings'
import { useBusiness } from '../../stores/business'
import SetupCard from '../business/SetupCard.vue'
import AppIcon from '../AppIcon.vue'
import SettingsCard from './SettingsCard.vue'

// Business for admins: which parts are on (the same switches as the overview),
// and the quote settings as they are stored. Editing quote texts and layout
// arrives with the quote editor; customers arrive with the CRM port.
const business = useBusiness()
const quotes = ref<QuoteSettings | null>(null)
const quotesState = ref<'loading' | 'ready' | 'closed' | 'error'>('loading')
async function loadQuotes() {
  quotesState.value = 'loading'
  try { quotes.value = await getQuoteSettings(); quotesState.value = 'ready' }
  catch (e) { quotesState.value = [403, 404, 409].includes(statusOf(e)) ? 'closed' : 'error' }
}
onMounted(() => { void business.loadPlugins(); void loadQuotes() })
</script>

<template>
  <div class="section">
    <div id="parts" class="parts-anchor">
      <SetupCard variant="manage" />
    </div>

    <SettingsCard title="Quote settings" icon="document" anchor="quotes">
      <template #lead>How quotes are numbered, priced and signed. They apply to every new quote.</template>
      <div v-if="quotesState === 'loading'" class="set-skeleton" role="status" aria-label="Loading quote settings"><span class="skeleton" /><span class="skeleton" /></div>
      <p v-else-if="quotesState === 'closed'" class="set-note"><AppIcon name="info" :size="14" />Quote settings show here once Quotes is enabled for this workspace.</p>
      <p v-else-if="quotesState === 'error'" class="set-note error" role="alert"><AppIcon name="alert" :size="14" />The quote settings could not be loaded.<button type="button" class="btn sm" @click="loadQuotes">Try again</button></p>
      <template v-else-if="quotes">
        <dl class="set-facts">
          <div><dt>Currency</dt><dd class="mono">{{ quotes.default_currency || 'Not set' }}</dd></div>
          <div><dt>Numbering</dt><dd>{{ quotes.numbering_time_zone ? `Dates in ${quotes.numbering_time_zone}` : 'Not set' }}</dd></div>
          <div><dt>Sender</dt><dd>{{ senderLine(quotes.sender) || 'Not set' }}</dd></div>
          <div><dt>Confirmations</dt><dd>{{ !quotes.smtp_configured ? 'No mail server configured' : quotes.smtp_confirmation_enabled ? 'Sent by email' : 'Off' }}</dd></div>
        </dl>
        <p class="set-note"><AppIcon name="info" :size="14" />Editing texts, layout and the sender arrives with the quote editor.</p>
      </template>
    </SettingsCard>

    <SettingsCard title="Customers" icon="users" anchor="customers">
      <template #lead>Organisations and their contacts, linked to projects and quotes.</template>
      <p class="set-note"><AppIcon name="info" :size="14" />Customer settings arrive with the CRM port.</p>
    </SettingsCard>
  </div>
</template>

<style scoped>
.section { display: grid; gap: 14px; }
.parts-anchor { scroll-margin-top: 20px; }
/* The parts card is the overview's own; here it takes the settings cards' measure. */
.parts-anchor :deep(.setup) { margin: 0; padding: 18px 20px; }
.parts-anchor :deep(.setup-head) { gap: 12px; }
.parts-anchor :deep(.setup-icon) { width: 32px; height: 32px; border-radius: 10px; }
.parts-anchor :deep(.setup-head h2) { font: 600 15px/1.35 var(--font); letter-spacing: 0; }
.parts-anchor :deep(.setup-head p) { margin-top: 2px; font-size: 13px; }
@media (max-width: 600px) { .parts-anchor :deep(.setup) { padding: 16px 14px; } }
.mono { font-family: var(--mono); }
</style>
