// SPDX-License-Identifier: AGPL-3.0-only
// Time zones and languages for the profile: the IANA list the browser knows, each
// zone's current offset, the languages offered, and how dates will look.

export const browserZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone
export function timeZones(): string[] {
  const all = (Intl as unknown as { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf?.('timeZone') ?? []
  return all.length ? all : ['UTC', 'Europe/Vienna', 'Europe/Berlin', 'Europe/London', 'America/New_York', 'Asia/Tokyo']
}
// "GMT+2", "GMT-4:30", "GMT" at `at`.
export function zoneOffset(zone: string, at = new Date()) {
  try {
    const part = new Intl.DateTimeFormat('en-GB', { timeZone: zone, timeZoneName: 'shortOffset' }).formatToParts(at).find(p => p.type === 'timeZoneName')
    return (part?.value ?? '').replace(/^GMT[+-]0$/, 'GMT')
  } catch { return '' }
}
export function zoneTime(zone: string, at = new Date()) {
  try { return new Intl.DateTimeFormat('en-GB', { timeZone: zone, hour: '2-digit', minute: '2-digit' }).format(at) } catch { return '' }
}
// "Europe/Vienna" reads "Vienna", with "Europe" as the region; "America/Argentina/Salta" reads "Salta".
export function zoneParts(zone: string) {
  const parts = zone.split('/')
  return { city: (parts[parts.length - 1] ?? zone).replace(/_/g, ' '), region: parts.length > 1 ? parts.slice(0, -1).join(' / ').replace(/_/g, ' ') : '' }
}
export function matchZone(zone: string, needle: string) {
  const q = needle.trim().toLowerCase().replace(/\s+/g, ' ')
  if (!q) return true
  return zone.toLowerCase().replace(/_/g, ' ').includes(q)
}

// Languages offered for dates, numbers and the week's first day.
export const LOCALES = ['en-GB', 'en-US', 'de-AT', 'de-DE', 'de-CH', 'fr-FR', 'fr-CH', 'it-IT', 'es-ES', 'nl-NL', 'pt-PT', 'pt-BR', 'sv-SE', 'da-DK', 'nb-NO', 'fi-FI', 'pl-PL', 'cs-CZ', 'ja-JP'] as const
export function localeName(tag: string) {
  try {
    const english = new Intl.DisplayNames(['en'], { type: 'language' }).of(tag) ?? tag
    const native = new Intl.DisplayNames([tag], { type: 'language' }).of(tag) ?? ''
    return { english, native: native && native.toLowerCase() !== english.toLowerCase() ? native : '' }
  } catch { return { english: tag, native: '' } }
}
// How a date and a time read in `tag`, in `zone`.
export function datePreview(tag: string, zone: string, at = new Date()) {
  try {
    const long = new Intl.DateTimeFormat(tag, { dateStyle: 'full', timeZone: zone }).format(at)
    const short = new Intl.DateTimeFormat(tag, { dateStyle: 'short', timeStyle: 'short', timeZone: zone }).format(at)
    return `${long} · ${short}`
  } catch { return '' }
}
export const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
