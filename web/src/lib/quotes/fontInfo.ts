// SPDX-License-Identifier: AGPL-3.0-only
// What a dropped font file is (U19, AEON-110): its format from the first bytes
// (the server checks the same way), and its family, weight and style from the
// font's own name and OS/2 tables for TTF and OTF. WOFF2 is compressed, so its
// weight and style come from the file name ("Beispiel-SemiBoldItalic.woff2").
// Free of Vue and the DOM, for unit tests.
export type FontFormat = 'ttf' | 'otf' | 'woff2'
export interface FontInfo { format: FontFormat; family: string; weight: number; style: 'normal' | 'italic'; from: 'font' | 'name' }

export function fontFormat(bytes: Uint8Array): FontFormat | null {
  const tag = String.fromCharCode(...bytes.slice(0, 4))
  if (tag === 'wOF2') return 'woff2'
  if (tag === 'OTTO') return 'otf'
  if (bytes[0] === 0 && bytes[1] === 1 && bytes[2] === 0 && bytes[3] === 0) return 'ttf'
  return null
}

const WEIGHTS: [RegExp, number][] = [
  [/(hairline|thin)/, 100], [/(extra|ultra)[\s_-]?light/, 200], [/light/, 300], [/(semi|demi)[\s_-]?bold/, 600],
  [/(extra|ultra)[\s_-]?bold/, 800], [/(black|heavy)/, 900], [/bold/, 700], [/medium/, 500], [/(regular|normal|book|roman)/, 400],
]
const STYLE_WORDS = /(hairline|thin|extra[\s_-]?light|ultra[\s_-]?light|light|semi[\s_-]?bold|demi[\s_-]?bold|extra[\s_-]?bold|ultra[\s_-]?bold|black|heavy|bold|medium|regular|normal|book|roman|italic|oblique|variable|vf)/gi
export const weightOf = (text: string) => WEIGHTS.find(([re]) => re.test(text.toLowerCase()))?.[1] ?? 400
export const italicOf = (text: string) => /(italic|oblique)/i.test(text)
// A family name the server takes: letters, digits, spaces, dots, dashes; 80 at most.
export function cleanFamily(raw: string): string {
  const text = raw.normalize('NFC').replace(/[^\p{L}\p{N} ._-]+/gu, ' ').replace(/\s+/g, ' ').trim().replace(/^[^\p{L}\p{N}]+/u, '')
  return text.slice(0, 80).trim()
}
// "Beispiel_Sans-SemiBoldItalic.woff2" → Beispiel Sans, 600, italic.
export function fromFileName(name: string): Omit<FontInfo, 'format'> {
  const base = name.replace(/\.[^.]+$/, '')
  const family = cleanFamily(base.replace(STYLE_WORDS, ' ').replace(/[-_]+/g, ' ').replace(/([a-z])([A-Z])/g, '$1 $2')) || 'Custom font'
  return { family, weight: weightOf(base), style: italicOf(base) ? 'italic' : 'normal', from: 'name' }
}

function tables(view: DataView): Map<string, { offset: number; length: number }> {
  const count = view.getUint16(4)
  const out = new Map<string, { offset: number; length: number }>()
  for (let i = 0; i < count && 12 + i * 16 + 16 <= view.byteLength; i++) {
    const at = 12 + i * 16
    const tag = String.fromCharCode(view.getUint8(at), view.getUint8(at + 1), view.getUint8(at + 2), view.getUint8(at + 3))
    out.set(tag, { offset: view.getUint32(at + 8), length: view.getUint32(at + 12) })
  }
  return out
}
// The name table's record for one id, preferring Windows (UTF-16) English, then Mac Roman.
function nameRecord(view: DataView, table: { offset: number; length: number }, id: number): string {
  const base = table.offset
  if (base + 6 > view.byteLength) return ''
  const count = view.getUint16(base + 2), strings = base + view.getUint16(base + 4)
  let best: { score: number; text: string } | null = null
  for (let i = 0; i < count; i++) {
    const at = base + 6 + i * 12
    if (at + 12 > view.byteLength) break
    const platform = view.getUint16(at), encoding = view.getUint16(at + 2), language = view.getUint16(at + 4), nameId = view.getUint16(at + 6)
    const length = view.getUint16(at + 8), offset = view.getUint16(at + 10)
    if (nameId !== id || strings + offset + length > view.byteLength) continue
    let text = ''
    if (platform === 3 || platform === 0) {
      for (let j = 0; j + 1 < length; j += 2) text += String.fromCharCode(view.getUint16(strings + offset + j))
    } else if (platform === 1 && encoding === 0) {
      for (let j = 0; j < length; j++) text += String.fromCharCode(view.getUint8(strings + offset + j))
    } else continue
    const score = platform === 3 && language === 0x409 ? 3 : platform === 3 ? 2 : 1
    if (!best || score > best.score) best = { score, text }
  }
  return best?.text.trim() ?? ''
}

// The face a file holds, or null for something that is not TTF, OTF or WOFF2.
export function fontInfo(buffer: ArrayBuffer, fileName: string): FontInfo | null {
  const bytes = new Uint8Array(buffer)
  const format = fontFormat(bytes)
  if (!format) return null
  const guess = fromFileName(fileName)
  if (format === 'woff2' || bytes.length < 12) return { format, ...guess }
  try {
    const view = new DataView(buffer)
    const dir = tables(view)
    const name = dir.get('name'), os2 = dir.get('OS/2')
    const family = name ? cleanFamily(nameRecord(view, name, 16) || nameRecord(view, name, 1)) : ''
    const sub = name ? nameRecord(view, name, 17) || nameRecord(view, name, 2) : ''
    let weight = 0, italic: boolean | null = null
    if (os2 && os2.offset + 64 <= view.byteLength) {
      weight = view.getUint16(os2.offset + 4)
      const selection = view.getUint16(os2.offset + 62)
      italic = (selection & 0x1) !== 0 || (selection & 0x200) !== 0
    }
    if (!family && !weight) return { format, ...guess }
    const rounded = weight >= 100 && weight <= 1000 ? Math.min(900, Math.max(100, Math.round(weight / 100) * 100)) : sub ? weightOf(sub) : guess.weight
    return { format, family: family || guess.family, weight: rounded, style: (italic ?? italicOf(sub)) ? 'italic' : 'normal', from: 'font' }
  } catch {
    return { format, ...guess }
  }
}
export const WEIGHT_NAMES: Record<number, string> = { 100: 'Thin', 200: 'Extra light', 300: 'Light', 400: 'Regular', 500: 'Medium', 600: 'Semibold', 700: 'Bold', 800: 'Extra bold', 900: 'Black' }
