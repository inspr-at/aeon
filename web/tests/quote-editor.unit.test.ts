// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from 'vitest'
import { reactive } from 'vue'
import { QuoteEditor } from '../src/lib/quotes/editor.ts'
import { documentTotal, positionTotal } from '../src/lib/quotes/layout.ts'
import type { QuoteDocumentData, QuotePosition } from '../src/lib/quotes/types.ts'

const sectionId = '11111111-1111-4111-8111-111111111111'
const fixture = (): QuoteDocumentData => ({
  schema_version: 1, minimum_writer_version: 1, title: 'Quote', subtitle: '', project_ref: '',
  offer_date: '2026-09-24', valid_until: '2026-10-24', currency: 'EUR',
  sender: {}, recipient: {}, legal: {}, layout: {},
  sections: [{ id: sectionId, heading: 'Scope', body: 'Before', nodes: [{ id: '22222222-2222-4222-8222-222222222222', kind: 'paragraph', text: 'Before' }] }],
  positions: [], net_total_cents: 0,
})

describe('QuoteEditor snapshots', () => {
  it('hydrates nested reactive document data and preserves independent undo history', () => {
    const source = reactive(fixture())
    const editor = new QuoteEditor(source)
    expect(editor.document).toEqual(fixture())
    editor.editSection(sectionId, { heading: 'After' })
    source.sections[0]!.heading = 'Changed outside editor'
    expect(editor.document.sections[0]!.heading).toBe('After')
    editor.undo()
    expect(editor.document.sections[0]!.heading).toBe('Scope')
    editor.redo()
    expect(editor.document.sections[0]!.heading).toBe('After')
    editor.replaceDocument(reactive(fixture()))
    expect(editor.document.sections[0]!.heading).toBe('Scope')
  })

  it('persists section settings and restores them through undo and redo', () => {
    const editor = new QuoteEditor(reactive(fixture()))
    editor.setSectionSettings(sectionId, { numberingStyle: 'upper-roman', pageBreakBefore: true, keepTogether: false, spacingBeforeMm: '3.5', spacingAfterMm: '2' })
    expect(editor.document.sections[0]).toMatchObject({ numbering_style: 'upper-roman', page_break_before: true, keep_together: false, spacing_before_mm: '3.5', spacing_after_mm: '2.0' })
    editor.undo()
    expect(editor.document.sections[0]!.numbering_style).toBeUndefined()
    editor.redo()
    expect(editor.document.sections[0]!.spacing_after_mm).toBe('2.0')
    expect(() => editor.setSectionSettings(sectionId, { spacingBeforeMm: '40.1' })).toThrow(/range/)
  })
})

const position = (quantity: string, unitPriceCents: number): QuotePosition => ({
  id: '33333333-3333-4333-8333-333333333333', pricing_source: 'manual', short_text: 'Work', long_text: '',
  quantity, unit_label: 'item', unit_price_cents: unitPriceCents, total_cents: 0, currency: 'EUR',
})

describe('cent half-up totals', () => {
  it('rounds an exact half cent up and refuses a float quantity or price', () => {
    expect(positionTotal(position('1.00', 100))).toBe(100)
    expect(positionTotal(position('1.50', 9999))).toBe(14999)
    expect(positionTotal(position('0.01', 50))).toBe(1)
    expect(positionTotal(position('0.01', 49))).toBe(0)
    expect(positionTotal(position('2.25', 10))).toBe(23)
    expect(documentTotal([position('0.01', 50), position('1.00', 100)])).toBe(101)
    expect(() => positionTotal(position('1.005', 100))).toThrow(/quantity/)
    expect(() => positionTotal({ ...position('1.00', 100), unit_price_cents: 10.5 })).toThrow(/minor-unit/)
  })
})
