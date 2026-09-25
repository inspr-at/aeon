// SPDX-License-Identifier: AGPL-3.0-only
// A minimal TrueType file for tests (U19): an sfnt header with just the 'OS/2' and
// 'name' tables, enough for the profile editor to read family, weight and style.
// It holds no glyphs and is never rendered; the invented names are synthetic.
export function syntheticTtf(family: string, subfamily: string, weight: number, italic: boolean): Uint8Array {
  const utf16 = (text: string) => { const out = new Uint8Array(text.length * 2); for (let i = 0; i < text.length; i++) { out[i * 2] = text.charCodeAt(i) >> 8; out[i * 2 + 1] = text.charCodeAt(i) & 0xff }; return out }
  const strings = [utf16(family), utf16(subfamily)]
  const nameLength = 6 + 12 * strings.length + strings.reduce((sum, s) => sum + s.length, 0)
  const name = new DataView(new ArrayBuffer(nameLength))
  name.setUint16(0, 0); name.setUint16(2, strings.length); name.setUint16(4, 6 + 12 * strings.length)
  let offset = 0
  strings.forEach((s, i) => {
    const at = 6 + i * 12
    name.setUint16(at, 3); name.setUint16(at + 2, 1); name.setUint16(at + 4, 0x409); name.setUint16(at + 6, i + 1); name.setUint16(at + 8, s.length); name.setUint16(at + 10, offset)
    new Uint8Array(name.buffer).set(s, 6 + 12 * strings.length + offset)
    offset += s.length
  })
  const os2 = new DataView(new ArrayBuffer(78))
  os2.setUint16(0, 0); os2.setUint16(4, weight); os2.setUint16(62, italic ? 0x1 : 0x40)
  const tables: [string, Uint8Array][] = [['OS/2', new Uint8Array(os2.buffer)], ['name', new Uint8Array(name.buffer)]]
  const pad = (n: number) => (n + 3) & ~3
  const headerLength = 12 + 16 * tables.length
  const total = headerLength + tables.reduce((sum, [, data]) => sum + pad(data.length), 0)
  const out = new Uint8Array(total)
  const view = new DataView(out.buffer)
  view.setUint32(0, 0x00010000); view.setUint16(4, tables.length); view.setUint16(6, 32); view.setUint16(8, 1); view.setUint16(10, 0)
  let at = headerLength
  tables.forEach(([tag, data], i) => {
    const record = 12 + i * 16
    for (let j = 0; j < 4; j++) view.setUint8(record + j, tag.charCodeAt(j))
    view.setUint32(record + 4, 0); view.setUint32(record + 8, at); view.setUint32(record + 12, data.length)
    out.set(data, at)
    at += pad(data.length)
  })
  return out
}
