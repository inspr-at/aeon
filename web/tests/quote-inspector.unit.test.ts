// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, it } from 'vitest'
import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { QuoteEditor } from '../src/lib/quotes/editor.ts'
import { formatMm, mmText, parseMm, scopeFor, scrubMm, sectionLabel, stepMm, textState } from '../src/lib/quotes/inspector.ts'
import { parseZoom, readZoom, stepZoom, zoomPercent, ZOOM_STEPS } from '../src/lib/quotes/zoom.ts'
import { addMonths, formatDocDate, monthGrid, parseDocDate } from '../src/lib/quotes/dates.ts'
import type { QuoteDocumentData, TextSelection } from '../src/lib/quotes/types.ts'

const S = '11111111-1111-4111-8111-111111111111'
const ids = ['a1', 'a2', 'a3', 'a4'].map(x => `aaaaaaaa-aaaa-4aaa-8aaa-${x.padStart(12, '0')}`)
const doc = (): QuoteDocumentData => ({
  schema_version: 1, minimum_writer_version: 1, title: 'Website relaunch', subtitle: '', project_ref: '', offer_date: '2026-09-24', valid_until: '2026-10-24', currency: 'EUR',
  sender: {}, recipient: {}, legal: {}, layout: {}, positions: [], net_total_cents: 0,
  sections: [{ id: S, heading: 'Leistungsgegenstand', body: '', nodes: [
    { id: ids[0]!, kind: 'paragraph', text: 'Einleitung', marks: [{ start: 0, end: 4, bold: true }] },
    { id: ids[1]!, kind: 'item', text: 'Erstens', marker: 'decimal', numbering: 'outline', depth: 0, section_bound: true },
    { id: ids[2]!, kind: 'item', text: 'Zweitens', marker: 'decimal', numbering: 'outline', depth: 1, section_bound: true, marker_x_mm: '2.5' },
    { id: ids[3]!, kind: 'item', text: 'Punkt', marker: 'disc', depth: 0 },
  ] }, { id: '22222222-2222-4222-8222-222222222222', heading: '', body: 'Zweiter Abschnitt', nodes: [] }],
})
const sel = (a: number, ao: number, b = a, bo = ao): TextSelection => ({ sectionId: S, anchor: { nodeId: ids[a]!, offset: ao }, focus: { nodeId: ids[b]!, offset: bo } })

describe('text state for the inspector', () => {
  it('reads marks, list mode, level, binding and the numbering preview', () => {
    const one = textState(doc(), sel(2, 3))!
    expect(one).toMatchObject({ list: 'numbered', depth: 1, bound: true, preview: '1.1.1', number: 1, markerX: 2.5, markerY: 0, collapsed: true, canOutdent: true })
    const bold = textState(doc(), sel(0, 0, 0, 4))!
    expect(bold).toMatchObject({ bold: true, italic: false, list: 'none', depth: null, canIndent: true, canOutdent: false, preview: null })
  })
  it('says mixed when a selection spans different formatting', () => {
    const span = textState(doc(), sel(0, 0, 3, 5))!
    expect(span).toMatchObject({ count: 4, bold: 'mixed', list: 'mixed', depth: 'mixed', markerX: 'mixed', bullet: 'disc' })
  })
  it('names the scope each tab is about', () => {
    const d = doc()
    const text = textState(d, sel(1, 0))
    expect(scopeFor('text', d, S, text, 'A260924-1')).toEqual({ eyebrow: 'Section 1 of 2', title: 'Leistungsgegenstand', detail: 'List item, level 1 · caret' })
    expect(scopeFor('section', d, S, text, 'A260924-1')).toEqual({ eyebrow: 'Section 1 of 2', title: 'Leistungsgegenstand', detail: '1 paragraph · 3 list items' })
    expect(scopeFor('document', d, S, text, 'A260924-1')).toEqual({ eyebrow: 'Document', title: 'A260924-1', detail: 'Website relaunch' })
    expect(scopeFor('section', d, d.sections[1]!.id, null, '')!.title).toBe('Untitled section')
    expect(scopeFor('text', d, undefined, null, '')).toBeNull()
  })
  it('follows the editor commands it drives', () => {
    const editor = new QuoteEditor(doc())
    editor.select({ sectionId: S, text: sel(3, 0) })
    editor.setListMode('numbered')
    expect(textState(editor.document, editor.selection.text)!.list).toBe('numbered')
    expect(textState(editor.document, editor.selection.text)!.sequence).toBe('follow')
    editor.setNumbering({ mode: 'start', start: 4 })
    expect(textState(editor.document, editor.selection.text)).toMatchObject({ number: 4, sequence: 'start', listStart: 4 })
    editor.setNumbering({ mode: 'continue' })
    expect(textState(editor.document, editor.selection.text)!.sequence).toBe('continue')
    editor.setOffsets({ textStart: mmText(stepMm(0, 1, true, { min: -20, max: 40 })) })
    expect(textState(editor.document, editor.selection.text)!.textStart).toBe(5)
    editor.undo()
    expect(textState(editor.document, editor.selection.text)!.textStart).toBe(0)
  })
})

describe('section numbering labels', () => {
  it('writes every style', () => {
    expect([sectionLabel(4, 'decimal'), sectionLabel(4, 'upper-roman'), sectionLabel(4, 'lower-roman'), sectionLabel(4, 'upper-alpha'), sectionLabel(28, 'lower-alpha'), sectionLabel(4, 'none'), sectionLabel(9, undefined)])
      .toEqual(['4.', 'IV.', 'iv.', 'D.', 'ab.', '', '9.'])
  })
})

describe('millimetre fields', () => {
  const range = { min: -30, max: 30 }
  it('steps by 0.5, Shift by 5, within range', () => {
    expect([stepMm(0, 1, false, range), stepMm(0.3, 1, false, range), stepMm(0.3, -1, false, range), stepMm(0.5, 1, true, range), stepMm(-4, -1, true, range), stepMm(29.5, 1, true, range)]).toEqual([0.5, 0.8, -0.2, 5.5, -9, 30])
  })
  it('parses typed values and scrubs by pointer travel', () => {
    expect([parseMm('2,5'), parseMm(' 3 mm'), parseMm('−1.5'), parseMm('abc'), parseMm('')]).toEqual([2.5, 3, -1.5, null, null])
    expect([scrubMm(1, 13, false, range), scrubMm(1, -9, true, range), scrubMm(0, 400, false, range)]).toEqual([2.5, -9, 30])
    expect([formatMm(-2.5), formatMm(0), mmText(0), mmText(2.5), mmText(-3)]).toEqual(['−2.5', '0', null, '2.5', '-3.0'])
  })
})

describe('zoom', () => {
  it('keeps 25–800 %, drops 500 and 700 from the presets and steps between them', () => {
    expect(ZOOM_STEPS).not.toContain(500); expect(ZOOM_STEPS).not.toContain(700)
    expect([parseZoom('500'), parseZoom('125 %'), parseZoom('24'), parseZoom('801'), parseZoom('1.5')]).toEqual([500, 125, null, null, null])
    expect([stepZoom(100, 1), stepZoom(100, -1), stepZoom(500, 1), stepZoom(25, -1), stepZoom(800, 1)]).toEqual([125, 75, 600, null, null])
  })
  it('fits width or the whole page to the desk', () => {
    expect(zoomPercent('width', { width: 842, height: 900 })).toBe(100)
    expect(zoomPercent('page', { width: 1600, height: 700 })).toBe(58)
    expect(zoomPercent(1000, { width: 1, height: 1 })).toBe(800)
    expect([readZoom('page', 100), readZoom(150, 'width'), readZoom('huge', 'width')]).toEqual(['page', 150, 'width'])
  })
})

describe('dates on the paper', () => {
  it('read in the document language and parse what people type', () => {
    expect(formatDocDate('2026-10-24')).toBe('24.10.2026')
    expect([parseDocDate('24.10.2026'), parseDocDate('1.2.26'), parseDocDate('2026-02-29'), parseDocDate('2028-02-29'), parseDocDate('31.11.2026'), parseDocDate('soon')]).toEqual(['2026-10-24', '2026-02-01', null, '2028-02-29', null, null])
  })
  it('lays a month out from Monday and keeps month ends when stepping', () => {
    const grid = monthGrid('2026-10-24')
    expect(grid).toHaveLength(42)
    expect(grid[0]).toEqual({ value: '2026-09-28', day: 28, inMonth: false })
    expect(grid.find(d => d.value === '2026-10-01')).toEqual({ value: '2026-10-01', day: 1, inMonth: true })
    expect([addMonths('2026-01-31', 1), addMonths('2026-03-31', -1), addMonths('2026-12-15', 1)]).toEqual(['2026-02-28', '2026-02-28', '2027-01-15'])
  })
})

// CSP parity: default-src 'self'; img-src 'self' blob: data:. The inspector needs no
// inline scripts, no eval and nothing from another origin.
describe('CSP parity of the inspector', () => {
  it('uses no eval, inline handlers as strings, or other origins', () => {
    const root = new URL('../src/components/quotes/inspector/', import.meta.url).pathname
    const files = [...readdirSync(root).map(f => join(root, f)), new URL('../src/components/quotes/QuoteTitleBar.vue', import.meta.url).pathname, new URL('../src/components/quotes/DatePicker.vue', import.meta.url).pathname, new URL('../src/lib/quotes/inspector.ts', import.meta.url).pathname]
    for (const file of files) {
      const text = readFileSync(file, 'utf8')
      expect(text, file).not.toMatch(/\beval\(|new Function\(|<script(?! setup| lang)|https?:\/\/(?!www\.w3\.org)|innerHTML|javascript:/)
    }
  })
})
