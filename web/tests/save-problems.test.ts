// SPDX-License-Identifier: AGPL-3.0-only
// A refused draft save names what to fix in the editor's words, with the way to it.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { describe, pathParts, saveProblems } from '../src/lib/quotes/saveProblems.ts'
import type { QuoteDocumentData } from '../src/lib/quotes/types.ts'

const doc = {
  sections: [{ id: 's-1', heading: 'Ausgangslage', body: '', nodes: [] }, { id: 's-2', heading: 'Leistungsgegenstand und Vorgehen im Detail, Schritt für Schritt', body: '', nodes: [] }],
  positions: [{ id: 'p-1', short_text: 'Konzeption' }, { id: 'p-2', short_text: 'Umsetzung' }],
} as unknown as QuoteDocumentData

test('paths read the same in every common spelling', () => {
  for (const path of ['sections[1].heading', 'sections.1.heading', '/sections/1/heading', 'document.sections[1].heading', '$.sections[1].heading']) {
    assert.deepEqual(pathParts(path), ['sections', 1, 'heading'], path)
  }
})

test('a section, a position and a field are named as the editor shows them', () => {
  assert.deepEqual(describe('sections[0].heading', 'must not be empty', doc), { where: 'Section 1 “Ausgangslage”: heading', message: 'Must not be empty.', target: { section: 's-1' } })
  assert.equal(describe('sections[1]', 'too long', doc).where, 'Section 2 “Leistungsgegenstand und Vorgehen im Det…”')
  assert.deepEqual(describe('positions[1].quantity', 'is not a number', doc), { where: 'Position 2 “Umsetzung”: quantity', message: 'Is not a number.', target: { position: 'p-2' } })
  assert.deepEqual(describe('valid_until', 'is before the quote date', doc), { where: 'Valid until', message: 'Is before the quote date.', target: { field: 'dates' } })
  assert.deepEqual(describe('recipient.name', 'is required', doc), { where: 'Customer address', message: 'Is required.', target: null })
  assert.equal(describe('sections[9].heading', 'gone', doc).target, null, 'a section the document no longer has cannot be shown')
})

test('lists and maps of reasons are both read, in order, at most eight', () => {
  assert.deepEqual(saveProblems({ error: 'invalid document', fields: [{ path: 'sections[0].body', message: 'contains an unsupported mark' }, { field: 'currency', reason: 'must be three letters' }] }, doc).map(p => p.where), ['Section 1 “Ausgangslage”: text', 'Currency'])
  assert.deepEqual(saveProblems({ errors: { 'positions.0.unit_price_cents': 'must be whole cents' } }, doc).map(p => `${p.where}: ${p.message}`), ['Position 1 “Konzeption”: price: Must be whole cents.'])
  assert.deepEqual(saveProblems({ details: ['The document is too large.'] }, doc).map(p => p.message), ['The document is too large.'])
  assert.equal(saveProblems({ problems: Array.from({ length: 12 }, (_, i) => ({ path: `sections[0]`, message: `problem ${i}` })) }, doc).length, 8)
})

test('a response without reasons gives none, so the notice falls back to the message', () => {
  assert.deepEqual(saveProblems({ error: 'invalid document' }, doc), [])
  assert.deepEqual(saveProblems(null, doc), [])
})
