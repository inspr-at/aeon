// SPDX-License-Identifier: AGPL-3.0-only
// Merge conflicts in words: "$.sections[id].nodes[id].text" becomes "Section 2,
// paragraph 3 · text", and each side's value a short readable preview, so the
// review dialog can ask a person to choose without showing document paths.
import type { QuoteDocumentData } from './types'

const FIELD: Record<string, string> = {
  title: 'Title', subtitle: 'Subtitle', project_ref: 'Project reference', offer_date: 'Quote date', valid_until: 'Valid until', currency: 'Currency',
  heading: 'heading', body: 'text', text: 'text', marks: 'bold and italic', order: 'order', kind: 'list format', depth: 'list level', marker: 'list format',
  numbering: 'numbering', list_start: 'numbering start', list_continue: 'numbering', section_bound: 'numbering', glyph: 'bullet', marker_x_mm: 'marker position',
  marker_y_mm: 'marker position', text_start_mm: 'text indent', numbering_style: 'heading numbers', page_break_before: 'page break', keep_together: 'keep together',
  spacing_before_mm: 'space before', spacing_after_mm: 'space after', quantity: 'quantity', unit_price_cents: 'price', short_text: 'short text', long_text: 'description',
  unit_label: 'unit', pricing_source: 'pricing', cost_unit_node_id: 'rate', rate_unit: 'rate unit', intro: 'Introduction', accept_text: 'Acceptance text', vat_note: 'VAT note',
}
const GROUP: Record<string, string> = { sender: 'Sender', recipient: 'Recipient', legal: '', layout: 'Layout', sections: 'Sections', positions: 'Positions' }
const clip = (text: string, max = 90) => text.length > max ? `${text.slice(0, max - 1).trimEnd()}…` : text
const human = (key: string) => FIELD[key] ?? key.replace(/_/g, ' ')

export function conflictPlace(path: string, doc: QuoteDocumentData | null): string {
  type Token = { key: string } | { id: string }
  const tokens: Token[] = [...path.matchAll(/\.([A-Za-z0-9_]+)|\[([^\]]+)\]/g)].map(m => m[1] ? { key: m[1] } : { id: m[2]! })
  const parts: string[] = []
  let section: QuoteDocumentData['sections'][number] | undefined
  let field = ''
  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i]!
    if ('id' in t) continue
    const next = tokens[i + 1]
    if (t.key === 'sections' && next && 'id' in next) {
      const index = doc?.sections.findIndex(s => s.id === next.id) ?? -1
      section = index >= 0 ? doc!.sections[index] : undefined
      parts.push(index >= 0 ? `Section ${index + 1}${section?.heading.trim() ? ` “${clip(section.heading.trim(), 32)}”` : ''}` : 'A removed section')
    } else if (t.key === 'nodes' && next && 'id' in next) {
      const nodes = section?.nodes ?? []
      const index = nodes.findIndex(n => n.id === next.id)
      parts.push(index >= 0 ? `${nodes[index]!.kind === 'item' ? 'list item' : 'paragraph'} ${index + 1}` : 'a removed paragraph')
    } else if (t.key === 'positions' && next && 'id' in next) {
      const index = doc?.positions.findIndex(p => p.id === next.id) ?? -1
      const short = index >= 0 ? doc!.positions[index]!.short_text.trim() : ''
      parts.push(index >= 0 ? `Position ${index + 1}${short ? ` “${clip(short, 32)}”` : ''}` : 'A removed position')
    } else if (t.key === 'order') {
      const owner = tokens[i - 1]
      field = owner && 'key' in owner ? ({ nodes: 'paragraph order', sections: 'section order', positions: 'position order' } as Record<string, string>)[owner.key] ?? 'order' : 'order'
    } else if (next && 'key' in next && next.key === 'order') {
      continue
    } else if (t.key in GROUP && (!next || 'key' in next)) {
      if (GROUP[t.key]) parts.push(GROUP[t.key]!)
    } else field = human(t.key)
  }
  const cap = (text: string) => text.charAt(0).toUpperCase() + text.slice(1)
  const where = parts.map((part, i) => i === 0 ? cap(part) : part).join(', ')
  if (!field) return where || 'The whole document'
  return where ? `${where} · ${field}` : cap(field)
}

export function conflictValue(value: unknown, path = ''): string {
  if (value === undefined || value === null) return 'Removed'
  if (typeof value === 'string') return value.trim() ? `“${clip(value.trim())}”` : 'Empty'
  if (typeof value === 'number') {
    if (/unit_price_cents$/.test(path)) return `${Math.floor(value / 100)}.${String(value % 100).padStart(2, '0')}`
    return String(value)
  }
  if (typeof value === 'boolean') return value ? 'On' : 'Off'
  if (Array.isArray(value)) {
    if (/\.order$/.test(path)) return 'Their own order'
    if (/marks$/.test(path)) return value.length ? `${value.length} styled ${value.length === 1 ? 'passage' : 'passages'}` : 'No styling'
    return `${value.length} ${value.length === 1 ? 'entry' : 'entries'}`
  }
  if (typeof value === 'object') {
    const text = (value as { text?: unknown; heading?: unknown; short_text?: unknown }).text ?? (value as { heading?: unknown }).heading ?? (value as { short_text?: unknown }).short_text
    return typeof text === 'string' && text.trim() ? `“${clip(text.trim())}”` : 'Changed'
  }
  return 'Changed'
}
