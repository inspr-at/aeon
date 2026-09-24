<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import QuoteText from './QuoteText.vue'
import { decimalCents, money, positionTotal } from '../../../lib/quotes/layout'
import type { QuoteEditor } from '../../../lib/quotes/editor'
import type { QuotePosition } from '../../../lib/quotes/types'
const props = defineProps<{ positions: QuotePosition[]; editor: QuoteEditor; indices?: string[]; editable?: boolean }>()
const rows = () => props.indices ? props.positions.filter(p => props.indices!.includes(p.id)) : props.positions
function commitNumber(id: string, kind: 'quantity' | 'unit_price_cents', event: Event) {
  const input = event.target as HTMLInputElement
  const raw = input.value.trim().replace(',', '.')
  if (kind === 'quantity') {
    if (!/^(0|[1-9]\d*)(?:\.\d{1,2})?$/.test(raw)) { input.setCustomValidity('Use a nonnegative quantity with up to two decimals.'); input.reportValidity(); return }
    input.setCustomValidity(''); props.editor.editPosition(id, { quantity: raw }); return
  }
  if (!/^(0|[1-9]\d*)(?:\.\d{1,2})?$/.test(raw)) { input.setCustomValidity('Use a nonnegative price with up to two decimals.'); input.reportValidity(); return }
  const [whole, fraction = ''] = raw.split('.')
  const cents = BigInt(whole!) * 100n + BigInt(fraction.padEnd(2, '0') || '0')
  if (cents > 1_000_000_000n) { input.setCustomValidity('Price is too large.'); input.reportValidity(); return }
  input.setCustomValidity(''); props.editor.editPosition(id, { unit_price_cents: Number(cents) })
}
</script>
<template>
  <section class="quote-positions" aria-label="Positions">
    <h2>Leistungen</h2>
    <table><thead><tr><th>Pos.</th><th>Leistung</th><th>Menge</th><th>Einheit</th><th>Einzelpreis</th><th>Betrag</th></tr></thead>
      <tbody v-for="row in rows()" :key="row.id" :data-position-id="row.id" @focusin="editor.select({ positionId: row.id })">
        <tr><td>{{ String(positions.indexOf(row) + 1).padStart(2, '0') }}</td><td><QuoteText :model-value="row.short_text" :editable="editable" :label="`Leistung Position ${positions.indexOf(row) + 1}`" @update:model-value="editor.editPosition(row.id, { short_text: $event })" /></td>
          <td><input v-if="editable" inputmode="decimal" :aria-label="`Menge Position ${positions.indexOf(row) + 1}`" :value="row.quantity" @change="commitNumber(row.id, 'quantity', $event)" /><template v-else>{{ row.quantity }}</template></td>
          <td><QuoteText :model-value="row.unit_label" :editable="editable" :label="`Einheit Position ${positions.indexOf(row) + 1}`" @update:model-value="editor.editPosition(row.id, { unit_label: $event })" /></td>
          <td><input v-if="editable && row.pricing_source === 'manual'" inputmode="decimal" :aria-label="`Einzelpreis Position ${positions.indexOf(row) + 1}`" :value="decimalCents(row.unit_price_cents)" @change="commitNumber(row.id, 'unit_price_cents', $event)" /><template v-else>{{ money(row.unit_price_cents, row.currency) }}</template></td>
          <td>{{ money(positionTotal(row), row.currency) }}</td></tr>
        <tr class="quote-long"><td></td><td colspan="5"><QuoteText :model-value="row.long_text" :editable="editable" :label="`Beschreibung Position ${positions.indexOf(row) + 1}`" @update:model-value="editor.editPosition(row.id, { long_text: $event })" /></td></tr>
      </tbody></table>
  </section>
</template>
<style scoped>
.quote-positions h2 { font-size: 15pt; margin: 0 0 6mm; }
table { width: 100%; border-collapse: collapse; table-layout: fixed; font-size: 9pt; }
th { text-align: left; font-size: 7pt; text-transform: uppercase; letter-spacing: .06em; border-bottom: 1px solid var(--line-2); padding: 2mm 1mm; }
th:first-child { width: 10mm; }th:nth-child(2) { width: 55mm; }th:nth-child(3) { width: 18mm; }th:nth-child(4) { width: 18mm; }th:nth-child(5),th:nth-child(6) { width: 27mm; }
td { vertical-align: top; padding: 3mm 1mm; overflow-wrap: anywhere; }td:nth-child(n+3) { text-align: right; }tbody { break-inside: avoid; border-bottom: 1px solid var(--line); }.quote-long td { padding-top: 0; color: var(--ink-2); }
input { width: 100%; min-width: 0; font: inherit; text-align: right; background: var(--field-bg); color: var(--ink); border: 1px solid var(--line); }
</style>
