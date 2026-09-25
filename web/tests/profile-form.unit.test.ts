// SPDX-License-Identifier: AGPL-3.0-only
// U19 (AEON-110): the document profile editor's rules. Font files read for their
// family, weight and style; WCAG contrast against the paper; the server's limits
// beside each field; labels that follow the locale; exact decimals; and the
// sample the preview lays out, which invents everything but the sender.
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { cleanFamily, fontFormat, fontInfo, fromFileName } from '../src/lib/quotes/fontInfo'
import { contrast, contrastNote, decimal, normalizeHex, problems, ratioText, stable, switchLocale, tableRoom, tableWidth, localeDate } from '../src/lib/quotes/profileForm'
import { defaultProfile, previewSnapshot, profileDate } from '../src/lib/quotes/profile'
import { sampleDocument } from '../src/lib/quotes/sampleDocument'
import { syntheticTtf } from './synthetic-font'

const buffer = (bytes: Uint8Array) => bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer

describe('font files', () => {
  it('reads family, weight and style from a TrueType file’s own tables', () => {
    const info = fontInfo(buffer(syntheticTtf('Beispiel Grotesk', 'SemiBold Italic', 600, true)), 'whatever.ttf')
    expect(info).toEqual({ format: 'ttf', family: 'Beispiel Grotesk', weight: 600, style: 'italic', from: 'font' })
    expect(fontInfo(buffer(syntheticTtf('Beispiel Grotesk', 'Regular', 400, false)), 'x.ttf')).toMatchObject({ weight: 400, style: 'normal' })
  })
  it('falls back to the file name for WOFF2, whose tables are compressed', () => {
    const woff2 = readFileSync(new URL('../src/assets/fonts/jetbrains.woff2', import.meta.url))
    expect(fontFormat(new Uint8Array(woff2))).toBe('woff2')
    expect(fontInfo(buffer(new Uint8Array(woff2)), 'Beispiel_Sans-SemiBoldItalic.woff2')).toEqual({ format: 'woff2', family: 'Beispiel Sans', weight: 600, style: 'italic', from: 'name' })
    expect(fromFileName('BeispielSerif-Black.woff2')).toMatchObject({ family: 'Beispiel Serif', weight: 900, style: 'normal' })
    expect(fromFileName('Beispiel-ExtraLight.otf').weight).toBe(200)
  })
  it('refuses what is not a font, and cleans names to what the server takes', () => {
    expect(fontInfo(buffer(new TextEncoder().encode('<svg></svg>')), 'mark.svg')).toBeNull()
    expect(cleanFamily('  «Beispiel» Sans / 2  ')).toBe('Beispiel Sans 2')
    expect(cleanFamily('___')).toBe('')
  })
})

describe('colours', () => {
  it('measures WCAG contrast', () => {
    expect(contrast('#000000', '#ffffff')).toBeCloseTo(21, 5)
    expect(ratioText(contrast('#767676', '#ffffff'))).toBe('4.5:1')
    expect(normalizeHex('2A7')).toBe('#22aa77')
    expect(normalizeHex('#2a7f78')).toBe('#2a7f78')
    expect(normalizeHex('teal')).toBeNull()
  })
  it('warns when small print loses AA on the paper, and never for the paper itself', () => {
    const colors = defaultProfile().colors
    expect(contrastNote('ink', colors)?.level).toBe('ok')
    expect(contrastNote('soft', { ...colors, soft: '#91a1a3' })?.level).toBe('low')
    expect(contrastNote('soft', { ...colors, soft: '#7b8b8d' })?.level).toBe('large')
    expect(contrastNote('rule', { ...colors, rule: '#fdfdfd' })?.level).toBe('low')
    expect(contrastNote('paper', colors)).toBeNull()
  })
})

describe('limits and labels', () => {
  it('mirrors the server: name, colours, table width, page number, labels, fonts', () => {
    const d = defaultProfile()
    expect(problems('Beispiel Stahl GmbH', d)).toEqual([])
    expect(problems('  ', d).map(p => p.field)).toEqual(['name'])
    d.positions_table.columns[1]!.width_mm = '120'
    expect(tableWidth(d)).toBe(217); expect(tableRoom(d)).toBe(168)
    d.footer.page_number_format = 'Seite {page}'
    d.labels.quote = '<b>'
    d.colors.accent = 'teal'
    d.fonts = [{ role: 'body', family: '-dash', weight: 400, style: 'normal', asset_id: 'x' }]
    expect(problems('Beispiel', d).map(p => p.field).sort()).toEqual(['color.accent', 'columns', 'font.0', 'label.quote', 'page_number_format'])
  })
  it('switches the labels still at the old locale’s defaults, keeps the ones written by hand', () => {
    const d = defaultProfile()
    d.labels.recipient = 'Kunde'
    expect(switchLocale(d, 'en')).toBeGreaterThan(5)
    expect(d.locale).toBe('en')
    expect(d.labels.quote).toBe('QUOTE')
    expect(d.labels.recipient).toBe('Kunde')
    expect(d.footer.page_number_format).toBe('PAGE {page} OF {total}')
    expect(d.totals.net_label).toBe('Net total')
    switchLocale(d, 'de-AT')
    expect(d.labels.quote).toBe('ANGEBOT')
  })
  it('writes exact decimals and compares profiles whatever the key order', () => {
    expect([decimal(9.6), decimal(12), decimal(7.499), decimal(-2.5), decimal(-0.001)]).toEqual(['9.6', '12', '7.5', '-2.5', '0'])
    const canonical = '{"a":[{"d":2}],"b":1}'
    expect(stable({ b: 1, a: [{ d: 2, c: undefined }] })).toBe(canonical)
    expect(stable({ a: [{ d: 2 }], b: 1 })).toBe(canonical)
    expect(stable({ a: [{ d: 3 }], b: 1 })).not.toBe(canonical)
  })
  it('prints dates in the document’s locale, English in the European order', () => {
    expect(localeDate('de-AT', '2026-09-21')).toBe('21.09.2026')
    expect(localeDate('en', '2026-09-21')).toBe('21/09/2026')
    const en = defaultProfile(); en.locale = 'en'
    expect(profileDate({ id: 'p', revision: 1, definition: en }, '2026-09-21')).toBe('21/09/2026')
    expect(profileDate(null, '2026-09-21')).toBe('21.09.2026')
  })
})

describe('the preview', () => {
  it('lays out an invented quote in the profile’s language, with the workspace’s own sender', () => {
    const de = sampleDocument('de-AT', { company: 'Beispiel Studio GmbH', city: 'Graz' })
    expect(de.recipient.name).toBe('Beispiel Stahl GmbH')
    expect(de.sender.company).toBe('Beispiel Studio GmbH')
    expect(de.sections.length).toBe(3)
    expect(de.net_total_cents).toBe(de.positions.reduce((sum, p) => sum + p.total_cents, 0))
    const en = sampleDocument('en', null)
    expect(en.title).toBe('Production hall control upgrade')
    expect(en.sender.company).toBe('Your company')
    const ids = [...de.sections.flatMap(s => [s.id, ...s.nodes.map(n => n.id)]), ...de.positions.map(p => p.id)]
    expect(new Set(ids).size).toBe(ids.length)
  })
  it('loads the preview’s fonts under a new name whenever the faces change', () => {
    const d = defaultProfile()
    const a = previewSnapshot(d, 'p1')
    d.colors.accent = '#000000'
    expect(previewSnapshot(d, 'p1').id).toBe(a.id)
    d.fonts = [{ role: 'body', family: 'Beispiel', weight: 400, style: 'normal', asset_id: 'f1' }]
    expect(previewSnapshot(d, 'p1').id).not.toBe(a.id)
    expect(a.id).toMatch(/^[a-z0-9-]+$/)
  })
})
