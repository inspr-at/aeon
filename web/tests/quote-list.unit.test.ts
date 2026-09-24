// SPDX-License-Identifier: AGPL-3.0-only
// U18 (AEON-99): the Quotes list's rules (status, search, date and amount
// filters, sorting, columns), the conflict review's words, and the CSP parity
// of the new quote surfaces.
import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { amountBounds, blankFilter, clampWidth, dateRange, dayText, matchesQuote, narrowed, numberOf, sortQuotes, statusOf, visibleColumns, type QuoteRow } from '../src/lib/quotes/list'
import { conflictPlace, conflictValue } from '../src/lib/quotes/conflicts'
import { lifecycleError, shortDigest } from '../src/lib/quotes/lifecycle'
import { APIError } from '../src/lib/api'
import type { QuoteDocumentData } from '../src/lib/quotes/types'
import { COPY, documentLanguage, formatDay, formatMoment, formatMoney } from '../src/lib/quotes/publicCopy'
import { money } from '../src/lib/quotes/layout'

const row = (patch: Partial<QuoteRow>): QuoteRow => ({
  quote_node_id: 'q', project_node_id: '', customer_org_node_id: 'org-a', current_version: 0, state: 'draft', revision: 1, archived: false,
  project_ref: '', classic_status: 'draft', key: 'QUO-1', title: 'Quote', customer_name: 'Alpha', created_at: '2026-09-01T09:00:00Z', updated_at: '2026-09-01T09:00:00Z', ...patch,
})

describe('quote list rules', () => {
  it('shows an issued quote past its validity as expired', () => {
    expect(statusOf({ state: 'issued', classic_status: 'expired' })).toBe('expired')
    expect(statusOf({ state: 'issued', classic_status: 'sent' })).toBe('issued')
    expect(statusOf({ state: 'accepted', classic_status: 'accepted' })).toBe('accepted')
  })
  it('numbers a quote by its commercial number, else its key', () => {
    expect(numberOf({ offer_no: 'A260924-01', key: 'QUO-3' })).toBe('A260924-01')
    expect(numberOf({ key: 'QUO-3' })).toBe('QUO-3')
  })
  it('turns date presets into inclusive ranges ending today', () => {
    expect(dateRange({ preset: '30d', from: '', to: '' }, '2026-09-24')).toEqual({ from: '2026-08-26', to: '2026-09-24' })
    expect(dateRange({ preset: 'quarter', from: '', to: '' }, '2026-09-24')).toEqual({ from: '2026-07-01', to: '2026-09-24' })
    expect(dateRange({ preset: 'custom', from: '2026-01-01', to: '' }, '2026-09-24')).toEqual({ from: '2026-01-01', to: '' })
    expect(dayText('2026-10-24')).toBe('24 Oct 2026')
  })
  it('reads amount bounds as people type them and never guesses a wrong one', () => {
    expect(amountBounds({ min: '1,500', max: '2000.50' })).toEqual({ min: 150000, max: 200050, invalid: false })
    expect(amountBounds({ min: '12.000,50', max: '' })).toEqual({ min: null, max: null, invalid: true })
  })
  it('filters by archive, status, customer, date, amount and search', () => {
    const f = blankFilter()
    const a = row({ quote_node_id: 'a', offer_date: '2026-09-20', net_total_cents: 50000, title: 'Kassen', customer_name: 'Hofer' })
    const b = row({ quote_node_id: 'b', offer_date: '2026-06-01', net_total_cents: undefined, archived: true, state: 'issued', classic_status: 'sent' })
    expect(matchesQuote(b, f, '2026-09-24')).toBe(false)
    expect(matchesQuote(b, { ...f, archived: true }, '2026-09-24')).toBe(true)
    expect(matchesQuote(a, { ...f, statuses: ['issued'] }, '2026-09-24')).toBe(false)
    expect(matchesQuote(a, { ...f, date: { preset: '30d', from: '', to: '' } }, '2026-09-24')).toBe(true)
    expect(matchesQuote(a, { ...f, amount: { min: '600', max: '' } }, '2026-09-24')).toBe(false)
    expect(matchesQuote(b, { ...f, archived: true, amount: { min: '0', max: '' } }, '2026-09-24')).toBe(false)
    expect(matchesQuote(a, { ...f, q: 'hof' }, '2026-09-24')).toBe(true)
    expect(narrowed({ ...f, amount: { min: 'x', max: '' } })).toBe(false)
  })
  it('sorts with blanks last either way and ties newest first', () => {
    const list = [row({ quote_node_id: 'a', net_total_cents: 100 }), row({ quote_node_id: 'b' }), row({ quote_node_id: 'c', net_total_cents: 900 })]
    expect(sortQuotes(list, { key: 'amount', dir: 'desc' }).map(r => r.quote_node_id)).toEqual(['c', 'a', 'b'])
    expect(sortQuotes(list, { key: 'amount', dir: 'asc' }).map(r => r.quote_node_id)).toEqual(['a', 'c', 'b'])
  })
  it('keeps every column within its bounds and drops the least essential ones first', () => {
    expect(clampWidth('customer', 5)).toBe(120)
    expect(clampWidth('customer', undefined)).toBe(220)
    expect(visibleColumns(2000)).toEqual(['number', 'title', 'customer', 'status', 'date', 'valid', 'amount'])
    expect(visibleColumns(760)).toEqual(['number', 'title', 'customer', 'status'])
  })
})

describe('conflict review words', () => {
  const doc = {
    title: 'T', sections: [{ id: 's1', heading: 'Scope', body: '', nodes: [{ id: 'n1', kind: 'paragraph', text: 'a' }, { id: 'n2', kind: 'item', text: 'b' }] }],
    positions: [{ id: 'p1', short_text: 'Design' }],
  } as unknown as QuoteDocumentData
  it('names the place, not the path', () => {
    expect(conflictPlace('$.title', doc)).toBe('Title')
    expect(conflictPlace('$.sections[s1].nodes[n2].text', doc)).toBe('Section 1 “Scope”, list item 2 · text')
    expect(conflictPlace('$.sections[s1].nodes.order', doc)).toBe('Section 1 “Scope” · paragraph order')
    expect(conflictPlace('$.positions[p1].quantity', doc)).toBe('Position 1 “Design” · quantity')
    expect(conflictPlace('$.legal.vat_note', doc)).toBe('VAT note')
    expect(conflictPlace('$.sender.company', doc)).toBe('Sender · company')
  })
  it('shows each side briefly', () => {
    expect(conflictValue('  Hello ')).toBe('“Hello”')
    expect(conflictValue(undefined)).toBe('Removed')
    expect(conflictValue(12345, '$.positions[p1].unit_price_cents')).toBe('123.45')
    expect(conflictValue([{ start: 0, end: 2, bold: true }], '$.sections[s1].nodes[n1].marks')).toBe('1 styled passage')
  })
})

describe('lifecycle words', () => {
  it('explains failures in the app’s words', () => {
    expect(lifecycleError(new APIError(409, 'quote or draft revision is stale'), 'x')).toMatch(/Someone changed this quote/)
    expect(lifecycleError(new APIError(403, 'nope'), 'x')).toMatch(/An admin can/)
    expect(lifecycleError(new TypeError('Failed to fetch'), 'x')).toMatch(/could not be reached/)
    expect(shortDigest('0123456789abcdef'.repeat(4))).toBe('01234567…abcdef')
  })
})

describe('the customer page speaks the document’s language', () => {
  const doc = (texts: Partial<QuoteDocumentData>) => ({ title: '', subtitle: '', legal: {}, layout: {}, sections: [], positions: [], ...texts }) as unknown as QuoteDocumentData
  it('reads the language from the document, German when in doubt, a declared one first', () => {
    expect(documentLanguage(doc({ title: 'Relaunch des Kundenportals', legal: { intro: 'Vielen Dank für Ihre Anfrage.' } }))).toBe('de')
    expect(documentLanguage(doc({ title: 'Synthetic service proposal', legal: { intro: 'A synthetic offer for layout checks.', accept_text: 'I accept this offer.' } }))).toBe('en')
    expect(documentLanguage(doc({ title: 'Website' }))).toBe('de')
    expect(documentLanguage(doc({ title: 'Relaunch des Portals', layout: { language: 'en' } as never }))).toBe('en')
  })
  it('formats dates and amounts in one locale', () => {
    expect(formatDay('2026-09-21', 'de')).toBe('21.09.2026')
    expect(formatDay('2026-09-21', 'en')).toBe('21/09/2026')
    expect(formatMoney(556000, 'EUR', 'de')).toBe('5.560,00 EUR')
    expect(formatMoney(123456789, 'EUR', 'en')).toBe('1,234,567.89 EUR')
    expect(formatMoment('2026-09-21T08:12:00Z', 'de')).toMatch(/^21\.09\.2026, \d{2}:12 Uhr$/)
    // The paper groups German amounts the same way.
    expect(money(556000, 'EUR')).toBe(formatMoney(556000, 'EUR', 'de'))
  })
  it('has every string in both catalogs', () => {
    expect(Object.keys(COPY.de).sort()).toEqual(Object.keys(COPY.en).sort())
    expect(COPY.de.acceptLead(1, 'A260922-12', '5.560,00 EUR')).toBe('Sie nehmen Version 1 des Angebots A260922-12 über netto 5.560,00 EUR an, genau so wie oben dargestellt.')
  })
})

// CSP parity: default-src 'self'; img-src 'self' blob: data: on the app, stricter on
// the public page. The new surfaces need no eval, inline handlers or other origins.
describe('CSP parity of the quote surfaces', () => {
  it('uses no eval, string handlers, innerHTML or other origins', () => {
    const dirs = ['../src/components/quotes/details/', '../src/components/quotes/list/', '../src/components/quotes/collaboration/'].map(d => new URL(d, import.meta.url).pathname)
    const files = [
      ...dirs.flatMap(d => readdirSync(d).map(f => join(d, f))),
      ...['../src/public/PublicQuoteView.vue', '../src/views/business/QuotesView.vue', '../src/components/business/QuoteWorkspace.vue', '../src/components/business/QuoteCreateDialog.vue', '../src/lib/quoteWorkspace.ts', '../src/lib/quotes/list.ts', '../src/lib/quotes/lifecycle.ts', '../src/lib/quotes/conflicts.ts', '../src/lib/quotes/publicCopy.ts'].map(f => new URL(f, import.meta.url).pathname),
    ]
    for (const file of files) {
      const text = readFileSync(file, 'utf8')
      expect(text, file).not.toMatch(/\beval\(|new Function\(|<script(?! setup| lang)|https?:\/\/(?!www\.w3\.org)|innerHTML|javascript:/)
    }
  })
})
