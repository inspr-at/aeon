// SPDX-License-Identifier: AGPL-3.0-only
// Calendar weeks for hours: Monday to Sunday in the viewer's time zone. Pure
// functions, no Vue, so tests can run them under plain Node.

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
export const WEEKDAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

export function startOfDay(date: Date): Date { return new Date(date.getFullYear(), date.getMonth(), date.getDate()) }
export function addDays(date: Date, days: number): Date { return new Date(date.getFullYear(), date.getMonth(), date.getDate() + days, date.getHours(), date.getMinutes()) }
export function startOfWeek(date: Date): Date {
  const day = startOfDay(date)
  return addDays(day, -((day.getDay() + 6) % 7))
}
export function weekDays(start: Date): Date[] { return Array.from({ length: 7 }, (_, i) => addDays(start, i)) }
// Local calendar day as YYYY-MM-DD.
export function dayKey(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}
export function parseDayKey(key: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(key)
  if (!match) return null
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]))
  return dayKey(date) === key ? date : null
}
export function isoWeek(date: Date): number {
  const target = startOfDay(date)
  target.setDate(target.getDate() + 3 - ((target.getDay() + 6) % 7))
  const firstThursday = new Date(target.getFullYear(), 0, 4)
  return 1 + Math.round(((target.getTime() - firstThursday.getTime()) / 86_400_000 - 3 + ((firstThursday.getDay() + 6) % 7)) / 7)
}
export function dayLabel(date: Date): string { return `${date.getDate()} ${MONTHS[date.getMonth()]}` }
export function rangeLabel(start: Date, end: Date): string {
  const last = addDays(end, 0)
  if (start.getFullYear() !== last.getFullYear()) return `${dayLabel(start)} ${start.getFullYear()} – ${dayLabel(last)} ${last.getFullYear()}`
  if (start.getMonth() !== last.getMonth()) return `${dayLabel(start)} – ${dayLabel(last)} ${last.getFullYear()}`
  return `${start.getDate()}–${last.getDate()} ${MONTHS[last.getMonth()]} ${last.getFullYear()}`
}
export function weekLabel(start: Date): string { return rangeLabel(start, addDays(start, 6)) }
// A period's inclusive last day: its end is exclusive, so a Monday 00:00 end ends on Sunday.
export function periodLabel(startsAt: string, endsAt: string): string {
  const start = new Date(startsAt)
  const end = new Date(Date.parse(endsAt) - 1)
  return rangeLabel(start, end)
}
export function overlaps(aStart: number, aEnd: number, bStart: number, bEnd: number) { return aStart < bEnd && bStart < aEnd }
export function timeOfDay(iso: string): string {
  const date = new Date(iso)
  return `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}
// "9", "9:30", "0930", "14.15" as minutes after midnight, or null.
export function parseTimeInput(text: string): number | null {
  const value = text.trim().replace('.', ':')
  const match = /^(\d{1,2})(?::?(\d{2}))?$/.exec(value)
  if (!match) return null
  const hours = Number(match[1]), minutes = Number(match[2] ?? 0)
  if (hours > 23 || minutes > 59) return null
  return hours * 60 + minutes
}
