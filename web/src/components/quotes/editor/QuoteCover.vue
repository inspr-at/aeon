<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import QuoteText from './QuoteText.vue'
import type { QuoteEditor } from '../../../lib/quotes/editor'
import type { QuoteDocumentData } from '../../../lib/quotes/types'
const props = defineProps<{ document: QuoteDocumentData; editor: QuoteEditor; offerNo?: string; editable?: boolean }>()
const set = (part: 'sender' | 'recipient' | 'legal', key: string, text: string) => props.editor.editField(part, { key, text })
const date = (value: string) => value ? new Intl.DateTimeFormat('de-AT', { day: '2-digit', month: '2-digit', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${value}T00:00:00Z`)) : ''
</script>
<template>
  <section class="quote-cover" aria-label="Quote cover">
    <p class="quote-overline">Angebot <span>{{ offerNo }}</span></p>
    <QuoteText tag="h1" class="quote-title" :model-value="document.title" label="Angebotstitel" :editable="editable" @update:model-value="editor.editField('title', $event)" />
    <QuoteText tag="p" class="quote-subtitle" :model-value="document.subtitle" label="Untertitel" :editable="editable" @update:model-value="editor.editField('subtitle', $event)" />
    <div class="quote-cover-grid">
      <div>
        <p class="quote-label">Auftraggeber</p>
        <QuoteText tag="p" class="quote-recipient" :model-value="document.recipient.name ?? ''" label="Firma des Kunden" :editable="editable" @update:model-value="set('recipient', 'name', $event)" />
        <QuoteText tag="p" :model-value="document.recipient.address ?? ''" label="Kundenanschrift" :editable="editable" @update:model-value="set('recipient', 'address', $event)" />
        <QuoteText tag="p" :model-value="document.recipient.contact ?? ''" label="Kundenkontakt" :editable="editable" @update:model-value="set('recipient', 'contact', $event)" />
        <QuoteText tag="p" :model-value="document.recipient.country ?? ''" label="Land des Kunden" :editable="editable" @update:model-value="set('recipient', 'country', $event)" />
      </div>
      <dl class="quote-meta">
        <dt>Angebotsnummer</dt><dd>{{ offerNo }}</dd>
        <dt>Angebotsdatum</dt><dd><input v-if="editable" type="date" aria-label="Angebotsdatum" :value="document.offer_date" @change="editor.editField('offer_date', ($event.target as HTMLInputElement).value)" /><template v-else>{{ date(document.offer_date) }}</template></dd>
        <dt>Kundennummer</dt><dd>{{ document.recipient.customer_no }}</dd>
        <dt>Gültig bis</dt><dd><input v-if="editable" type="date" aria-label="Gültig bis" :value="document.valid_until" @change="editor.editField('valid_until', ($event.target as HTMLInputElement).value)" /><template v-else>{{ date(document.valid_until) }}</template></dd>
        <dt>Ansprechpartner</dt><dd><QuoteText :model-value="document.sender.contact_person ?? ''" label="Ansprechpartner" :editable="editable" @update:model-value="set('sender', 'contact_person', $event)" /></dd>
        <dt>Projektreferenz</dt><dd><QuoteText :model-value="document.project_ref" label="Projektreferenz" :editable="editable" @update:model-value="editor.editField('project_ref', $event)" /></dd>
      </dl>
    </div>
    <p class="quote-sender"><strong>{{ document.sender.company }}</strong><span>{{ document.sender.street }}, {{ document.sender.postal_code }} {{ document.sender.city }} {{ document.sender.country }}</span><span v-if="document.sender.register_no">{{ document.sender.register_no }} · {{ document.sender.register_court }}</span><span>{{ document.sender.email }}</span></p>
    <QuoteText tag="p" class="quote-intro" :model-value="document.legal.intro ?? ''" label="Einleitung" :editable="editable" @update:model-value="set('legal', 'intro', $event)" />
  </section>
</template>
<style scoped>
.quote-cover { display: grid; gap: 8mm; }
.quote-overline { font-size: 10pt; font-weight: 700; letter-spacing: .18em; text-transform: uppercase; display: flex; justify-content: space-between; }
.quote-title { font-size: 27pt; line-height: 1.13; font-weight: 700; margin: 7mm 0 0; }
.quote-subtitle { font-size: 12pt; color: var(--ink-2); margin: 0; }
.quote-cover-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12mm; margin-top: 12mm; }
.quote-label { font-size: 8pt; font-weight: 700; text-transform: uppercase; letter-spacing: .1em; color: var(--ink-2); }
.quote-recipient { font-size: 12pt; font-weight: 700; }
.quote-meta { display: grid; grid-template-columns: 32mm 1fr; gap: 3mm 5mm; margin: 0; font-size: 9pt; }
.quote-meta dt { color: var(--ink-2); }.quote-meta dd { margin: 0; overflow-wrap: anywhere; }
.quote-meta input { width: 100%; font: inherit; }
.quote-sender { display: flex; flex-wrap: wrap; gap: 1mm 5mm; border-top: 1px solid var(--line-2); padding-top: 4mm; font-size: 8pt; }
.quote-intro { font-size: 10pt; line-height: 1.5; margin-top: 8mm; }
</style>
