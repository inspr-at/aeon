// SPDX-License-Identifier: AGPL-3.0-only
// Dates on a quote: stored as YYYY-MM-DD, read in the document's language
// (German: 24.10.2026), picked from a month grid. UTC calendar arithmetic only,
// so no time zone or daylight saving can shift a day. Free of Vue.

export const DOC_LOCALE = 'de-AT'
const valid = (y: number, m: number, d: number) => {
  const date = new Date(Date.UTC(y, m - 1, d))
  return date.getUTCFullYear() === y && date.getUTCMonth() === m - 1 && date.getUTCDate() === d
}
const iso = (y: number, m: number, d: number) => `${String(y).padStart(4, '0')}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
export const partsOf = (value: string) => { const [y, m, d] = value.split('-').map(Number); return { y: y!, m: m!, d: d! } }

// "24.10.2026", "24.10.26", "2026-10-24" or "24/10/2026" as YYYY-MM-DD; anything else is null.
export function parseDocDate(raw: string): string | null {
  const text = raw.trim()
  let m = /^(\d{4})-(\d{1,2})-(\d{1,2})$/.exec(text)
  if (m) return valid(+m[1]!, +m[2]!, +m[3]!) ? iso(+m[1]!, +m[2]!, +m[3]!) : null
  m = /^(\d{1,2})[./](\d{1,2})[./](\d{2}|\d{4})$/.exec(text)
  if (!m) return null
  const year = m[3]!.length === 2 ? 2000 + +m[3]! : +m[3]!
  return valid(year, +m[2]!, +m[1]!) ? iso(year, +m[2]!, +m[1]!) : null
}
// As the document prints it: 24.10.2026.
export function formatDocDate(value: string, locale = DOC_LOCALE): string {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return ''
  const { y, m, d } = partsOf(value)
  return new Intl.DateTimeFormat(locale, { day: '2-digit', month: '2-digit', year: 'numeric', timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, d)))
}
// For people: "Saturday, 24 October 2026" in the app's language.
export function longDate(value: string, locale = 'en-GB'): string {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return ''
  const { y, m, d } = partsOf(value)
  return new Intl.DateTimeFormat(locale, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, d)))
}
export function addDays(value: string, days: number): string {
  const { y, m, d } = partsOf(value)
  const date = new Date(Date.UTC(y, m - 1, d + days))
  return iso(date.getUTCFullYear(), date.getUTCMonth() + 1, date.getUTCDate())
}
// The same day in another month, or its last day when that month is shorter.
export function addMonths(value: string, months: number): string {
  const { y, m, d } = partsOf(value)
  const first = new Date(Date.UTC(y, m - 1 + months, 1))
  const last = new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0)).getUTCDate()
  return iso(first.getUTCFullYear(), first.getUTCMonth() + 1, Math.min(d, last))
}
export interface Day { value: string; day: number; inMonth: boolean }
// Six weeks from the Monday on or before the 1st of the month.
export function monthGrid(value: string): Day[] {
  const { y, m } = partsOf(value)
  const first = new Date(Date.UTC(y, m - 1, 1))
  const offset = (first.getUTCDay() + 6) % 7
  const start = iso(y, m, 1)
  return Array.from({ length: 42 }, (_, i) => {
    const day = addDays(start, i - offset)
    const p = partsOf(day)
    return { value: day, day: p.d, inMonth: p.m === m }
  })
}
export function monthTitle(value: string, locale = 'en-GB'): string {
  const { y, m } = partsOf(value)
  return new Intl.DateTimeFormat(locale, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(new Date(Date.UTC(y, m - 1, 1)))
}
export const WEEKDAYS = ['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su']
export function todayIso(now = new Date()): string {
  return iso(now.getFullYear(), now.getMonth() + 1, now.getDate())
}
