<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import QuoteText from './QuoteText.vue'
import { documentTotal, money } from '../../../lib/quotes/layout'
import type { QuoteEditor } from '../../../lib/quotes/editor'
import type { QuoteDocumentData } from '../../../lib/quotes/types'
defineProps<{ document: QuoteDocumentData; editor: QuoteEditor; editable?: boolean; accepted?: { name: string; company?: string; at: string; digest: string } | null }>()
</script>
<template>
  <section class="quote-acceptance" aria-label="Acceptance">
    <div class="quote-total"><span>Nettosumme</span><strong>{{ money(documentTotal(document.positions), document.currency) }}</strong></div>
    <QuoteText :model-value="document.legal.vat_note ?? ''" label="Umsatzsteuerhinweis" :editable="editable" @update:model-value="editor.editField('legal', { key: 'vat_note', text: $event })" />
    <QuoteText tag="p" class="quote-accept-text" :model-value="document.legal.accept_text ?? ''" label="Annahmetext" :editable="editable" @update:model-value="editor.editField('legal', { key: 'accept_text', text: $event })" />
    <div class="quote-signatures"><div v-if="accepted" class="quote-stamp"><strong>Digital angenommen</strong><span>{{ accepted.name }} · {{ accepted.company }}</span><time>{{ accepted.at }}</time><small>{{ accepted.digest }}</small></div><div v-else>Ort, Datum, Unterschrift Auftraggeber</div><div>Ort, Datum, Unterschrift Auftragnehmer</div></div>
  </section>
</template>
<style scoped>
.quote-acceptance { display: grid; gap: 6mm; break-inside: avoid; font-size: 9pt; }.quote-total { display: flex; justify-content: space-between; gap: 10mm; border-top: 1px solid var(--line-2); padding-top: 4mm; font-size: 13pt; }.quote-accept-text { line-height: 1.5; }.quote-signatures { display: grid; grid-template-columns: 1fr 1fr; gap: 12mm; margin-top: 8mm; }.quote-signatures > div { border-top: 1px solid var(--ink-2); padding-top: 3mm; }.quote-stamp { display: grid; gap: 1mm; border: 1px solid var(--line-2) !important; border-radius: 4px; padding: 4mm !important; overflow-wrap: anywhere; }.quote-stamp small { font-size: 7pt; }
</style>
