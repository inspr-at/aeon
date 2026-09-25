// SPDX-License-Identifier: AGPL-3.0-only
// A document profile as its editor shows it (U19, AEON-110): the colour tokens and
// what each one paints, WCAG contrast against the paper, the type scale and the
// A4 geometry with the ranges the editor offers, the labels each locale starts
// with, and the server's limits (internal/business/quotes/profiles.go) mirrored so
// a problem shows beside its field before anything is saved. Free of Vue.
import type { QuoteProfileDefinition } from './types'

// ---------- Colours ----------
export type ColorKey = 'ink' | 'muted' | 'soft' | 'accent' | 'rule' | 'paper'
export const COLOR_TOKENS: { key: ColorKey; label: string; use: string; text: boolean }[] = [
  { key: 'ink', label: 'Text', use: 'Body text, amounts and totals', text: true },
  { key: 'muted', label: 'Secondary text', use: 'Subtitles, labels and signature lines', text: true },
  { key: 'soft', label: 'Quiet text', use: 'Page header, footer and page numbers', text: true },
  { key: 'accent', label: 'Accent', use: 'Title, section headings and table heads', text: true },
  { key: 'rule', label: 'Rules', use: 'Lines between blocks and rows', text: false },
  { key: 'paper', label: 'Paper', use: 'The page itself', text: false },
]
export const HEX = /^#[0-9a-fA-F]{6}$/
// "#2A7" or "2a7f78" as people type them, as the six-digit form the server keeps.
export function normalizeHex(raw: string): string | null {
  const text = raw.trim().replace(/^#?/, '#')
  if (/^#[0-9a-fA-F]{3}$/.test(text)) return `#${[...text.slice(1)].map(c => c + c).join('')}`.toLowerCase()
  return HEX.test(text) ? text.toLowerCase() : null
}
function channel(value: number) {
  const c = value / 255
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
}
export function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map(i => channel(parseInt(hex.slice(i, i + 2), 16)))
  return 0.2126 * r! + 0.7152 * g! + 0.0722 * b!
}
// WCAG 2 contrast ratio, 1 to 21.
export function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi! + 0.05) / (lo! + 0.05)
}
export interface ContrastNote { ratio: number; level: 'ok' | 'large' | 'low'; message: string }
// Text tokens need 4.5:1 against the paper (they print small: footers at 7.5 pt,
// table heads at 7.5 pt). Rules are decoration and only get a note when they vanish.
export function contrastNote(key: ColorKey, colors: Record<string, string>): ContrastNote | null {
  const paper = colors.paper, value = colors[key]
  if (key === 'paper' || !paper || !value || !HEX.test(paper) || !HEX.test(value)) return null
  const ratio = contrast(value, paper)
  const text = COLOR_TOKENS.find(t => t.key === key)!.text
  if (!text) return ratio < 1.25 ? { ratio, level: 'low', message: 'Barely visible on the paper.' } : { ratio, level: 'ok', message: '' }
  if (ratio >= 4.5) return { ratio, level: 'ok', message: 'Meets AA on the paper.' }
  if (ratio >= 3) return { ratio, level: 'large', message: 'AA only for large text; small print here needs 4.5:1.' }
  return { ratio, level: 'low', message: 'Below AA on the paper; small print will be hard to read.' }
}
export const ratioText = (ratio: number) => `${(Math.floor(ratio * 10) / 10).toFixed(1)}:1`

// ---------- Numbers ----------
export interface Range { min: number; max: number }
// Point sizes the editor offers (the server takes 0 to 100).
export const TYPE_SCALE: { key: string; label: string; range: Range; fallback: number }[] = [
  { key: 'body_pt', label: 'Body text', range: { min: 6, max: 16 }, fallback: 10 },
  { key: 'title_pt', label: 'Title', range: { min: 8, max: 48 }, fallback: 17 },
  { key: 'section_pt', label: 'Headings', range: { min: 7, max: 36 }, fallback: 18 },
  { key: 'table_pt', label: 'Table', range: { min: 6, max: 14 }, fallback: 9.6 },
  { key: 'footer_pt', label: 'Footer', range: { min: 5, max: 12 }, fallback: 7.5 },
]
export const MARGINS: { key: 'top_mm' | 'right_mm' | 'bottom_mm' | 'left_mm'; label: string }[] = [
  { key: 'top_mm', label: 'Top' }, { key: 'right_mm', label: 'Right' }, { key: 'bottom_mm', label: 'Bottom' }, { key: 'left_mm', label: 'Left' },
]
export const MARGIN_RANGE: Range = { min: 5, max: 50 }
export const COVER: { key: string; label: string; range: Range; fallback: number }[] = [
  { key: 'top_mm', label: 'Space above the title', range: { min: 0, max: 60 }, fallback: 11 },
  { key: 'title_gap_mm', label: 'Title to subtitle', range: { min: 0, max: 40 }, fallback: 7 },
  { key: 'columns_gap_mm', label: 'Above the address block', range: { min: 0, max: 40 }, fallback: 8 },
  { key: 'columns_padding_mm', label: 'Inside the address block', range: { min: 0, max: 30 }, fallback: 6 },
]
export const COLUMN_LABELS: Record<string, string> = { position: 'Position', description: 'Description', quantity: 'Quantity', unit: 'Unit', unit_price: 'Unit price', total: 'Amount' }
export const COLUMN_SHORT: Record<string, string> = { position: 'Pos.', description: 'Description', quantity: 'Qty', unit: 'Unit', unit_price: 'Price', total: 'Amount' }
export const COLUMN_RANGE: Range = { min: 5, max: 180 }
export const SIGNATURE_RANGE: Range = { min: 0, max: 60 }
export const MARK_WIDTH_RANGE: Range = { min: 10, max: 96 }
export const MARK_OFFSET_RANGE: Range = { min: -6, max: 10 }
// The server's exact decimal: at most two places, no trailing zeros ("9.6", "12").
export function decimal(value: number): string {
  const rounded = Math.round(Math.abs(value) * 100) / 100
  const text = String(rounded)
  return value < 0 && rounded !== 0 ? `-${text}` : text
}
export const num = (text: string | undefined, fallback = 0) => { const n = Number(text); return Number.isFinite(n) && text !== '' && text !== undefined ? n : fallback }
// Width left for the positions table between the side margins.
export const tableRoom = (d: QuoteProfileDefinition) => 210 - num(d.page.left_mm) - num(d.page.right_mm)
export const tableWidth = (d: QuoteProfileDefinition) => d.positions_table.columns.reduce((sum, c) => sum + num(c.width_mm), 0)

// ---------- Labels ----------
export type Locale = 'de-AT' | 'en'
export const LOCALES: { value: Locale; label: string }[] = [{ value: 'de-AT', label: 'Deutsch (Österreich)' }, { value: 'en', label: 'English' }]
// A date as the document prints it in this locale (QuoteCover's format).
export const localeDate = (locale: Locale, iso: string) => new Intl.DateTimeFormat(locale === 'en' ? 'en-GB' : 'de-AT', { day: '2-digit', month: '2-digit', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${iso}T00:00:00Z`))
export const LABELS: { key: string; label: string; de: string; en: string }[] = [
  { key: 'quote', label: 'Document title', de: 'ANGEBOT', en: 'QUOTE' },
  { key: 'recipient', label: 'Recipient', de: 'Auftraggeber', en: 'Client' },
  { key: 'number', label: 'Quote number', de: 'Angebotsnummer', en: 'Quote number' },
  { key: 'date', label: 'Quote date', de: 'Angebotsdatum', en: 'Quote date' },
  { key: 'customer', label: 'Customer number', de: 'Kundennummer', en: 'Customer number' },
  { key: 'valid', label: 'Valid until', de: 'Gültig bis', en: 'Valid until' },
  { key: 'contact', label: 'Contact', de: 'Ansprechpartner', en: 'Contact' },
  { key: 'project', label: 'Project reference', de: 'Projektreferenz', en: 'Project reference' },
  { key: 'positions', label: 'Positions heading', de: 'Leistungsaufstellung', en: 'Services' },
  { key: 'net', label: 'Net total', de: 'Nettosumme', en: 'Net total' },
  { key: 'discount', label: 'Discount', de: 'Rabatt', en: 'Discount' },
  { key: 'vat', label: 'VAT', de: 'Umsatzsteuer', en: 'VAT' },
  { key: 'signature_customer', label: 'Client signature', de: 'Ort, Datum, Unterschrift Auftraggeber', en: 'Place, date, signature of the client' },
  { key: 'signature_sender', label: 'Your signature', de: 'Ort, Datum, Unterschrift Auftragnehmer', en: 'Place, date, signature of the contractor' },
]
export const LOCALE_TEXT: Record<Locale, { net_label: string; payment: string; page: string }> = {
  'de-AT': { net_label: 'Nettosumme', payment: 'Zahlungsbedingungen', page: 'SEITE {page} VON {total}' },
  en: { net_label: 'Net total', payment: 'Payment terms', page: 'PAGE {page} OF {total}' },
}
const localeKey = (locale: Locale) => locale === 'en' ? 'en' : 'de'
// Switching the locale moves every text still at the other locale's default to the
// new one; anything written by hand stays. Returns how many changed.
export function switchLocale(d: QuoteProfileDefinition, to: Locale): number {
  const from = d.locale as Locale
  if (from === to) return 0
  let changed = 0
  for (const entry of LABELS) {
    const current = d.labels[entry.key]
    if (current === undefined || current === '' || current === entry[localeKey(from)]) {
      if (d.labels[entry.key] !== entry[localeKey(to)]) changed++
      d.labels[entry.key] = entry[localeKey(to)]
    }
  }
  const was = LOCALE_TEXT[from], next = LOCALE_TEXT[to]
  if (d.totals.net_label === was.net_label || !d.totals.net_label) { d.totals.net_label = next.net_label; changed++ }
  if (d.payment_terms.heading === was.payment || !d.payment_terms.heading) { d.payment_terms.heading = next.payment; changed++ }
  if (d.footer.page_number_format === was.page) { d.footer.page_number_format = next.page; changed++ }
  d.locale = to
  return changed
}

// ---------- The server's limits, beside each field ----------
export const FAMILY = /^[\p{L}\p{N}][\p{L}\p{N} ._-]{0,79}$/u
export interface Problem { field: string; message: string }
export function problems(name: string, d: QuoteProfileDefinition): Problem[] {
  const out: Problem[] = []
  const trimmed = name.trim()
  if (!trimmed) out.push({ field: 'name', message: 'Give the profile a name.' })
  else if (trimmed.length > 100) out.push({ field: 'name', message: 'At most 100 characters.' })
  for (const token of COLOR_TOKENS) if (!HEX.test(d.colors[token.key] ?? '')) out.push({ field: `color.${token.key}`, message: 'Use a colour like #2a7f78.' })
  if (tableWidth(d) > tableRoom(d) + 1e-9) out.push({ field: 'columns', message: `The columns are ${decimal(tableWidth(d))} mm wide; the page has ${decimal(tableRoom(d))} mm between the margins.` })
  const format = d.footer.page_number_format
  if (!format.includes('{page}') || !format.includes('{total}')) out.push({ field: 'page_number_format', message: 'Keep {page} and {total} in the page number.' })
  else if (format.length > 100) out.push({ field: 'page_number_format', message: 'At most 100 characters.' })
  for (const [key, value] of Object.entries(d.labels)) {
    if (value.length > 100) out.push({ field: `label.${key}`, message: 'At most 100 characters.' })
    else if (/[<>]/.test(value)) out.push({ field: `label.${key}`, message: 'Angle brackets are not allowed.' })
  }
  if (d.totals.net_label.length > 100) out.push({ field: 'net_label', message: 'At most 100 characters.' })
  if (d.payment_terms.heading.length > 100) out.push({ field: 'payment_heading', message: 'At most 100 characters.' })
  if (d.fonts.length > 12) out.push({ field: 'fonts', message: 'At most 12 font files per profile.' })
  d.fonts.forEach((f, i) => { if (!FAMILY.test(f.family)) out.push({ field: `font.${i}`, message: 'A family name starts with a letter or digit; letters, digits, spaces, dots, dashes.' }) })
  return out
}

// Key order does not matter (the server returns maps sorted): the same profile
// gives the same text, so "unsaved changes" means a real change.
export function stable(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stable).join(',')}]`
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>
    return `{${Object.keys(record).filter(k => record[k] !== undefined).sort().map(k => `${JSON.stringify(k)}:${stable(record[k])}`).join(',')}}`
  }
  return JSON.stringify(value) ?? 'null'
}
// A plain copy of JSON data, reactive proxies included (structuredClone refuses those).
export const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T
