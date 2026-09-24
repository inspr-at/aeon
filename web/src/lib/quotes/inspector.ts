// SPDX-License-Identifier: AGPL-3.0-only
// What the format inspector shows for the current selection: the text's marks,
// list mode, level, numbering and optical offsets (with mixed states across a
// multi-paragraph selection), the scope header per tab, the section numbering
// labels, and the millimetre stepping rules. Read-only over P3's model: every
// change still goes through QuoteEditor's commands and history.
//
// CSP parity: the app runs under "default-src 'self'; img-src 'self' blob: data:".
// Nothing here or in the inspector may need inline scripts, eval or another origin.
import { MAX_DEPTH, markerLabels, nodesFor, selectionMarks } from './prose'
import type { QuoteDocumentData, QuoteMarker, SectionNumberingStyle, TextNode, TextSelection } from './types'

export type Mixed<T> = T | 'mixed'
export type ListKind = 'none' | 'bullet' | 'numbered'
export interface TextState {
  sectionId: string; sectionIndex: number; count: number; collapsed: boolean
  bold: boolean | 'mixed'; italic: boolean | 'mixed'
  list: Mixed<ListKind>; bullet: Mixed<Exclude<QuoteMarker, 'decimal'>> | null
  depth: Mixed<number> | null; canIndent: boolean; canOutdent: boolean
  bound: Mixed<boolean> | null; continued: Mixed<boolean> | null; listStart: number | null
  sequence: 'follow' | 'continue' | 'start' | 'mixed' | null
  number: number | null; preview: string | null
  markerX: Mixed<number> | null; markerY: Mixed<number> | null; textStart: Mixed<number> | null
  kind: Mixed<'paragraph' | 'item'>
}
const same = <T>(values: T[]): Mixed<T> => values.every(v => v === values[0]) ? values[0]! : 'mixed'
const mm = (value: string | undefined) => value ? Number(value) : 0
const listKind = (node: TextNode): ListKind => node.kind === 'paragraph' ? 'none' : node.marker === 'decimal' ? 'numbered' : 'bullet'

export function selectedNodes(doc: QuoteDocumentData, selection: TextSelection): { nodes: TextNode[]; all: TextNode[]; index: number } | null {
  const index = doc.sections.findIndex(s => s.id === selection.sectionId)
  if (index < 0) return null
  const section = doc.sections[index]!
  const all = nodesFor(section.body, section.nodes)
  const a = all.findIndex(n => n.id === selection.anchor.nodeId), b = all.findIndex(n => n.id === selection.focus.nodeId)
  if (a < 0 || b < 0) return null
  return { nodes: all.slice(Math.min(a, b), Math.max(a, b) + 1), all, index }
}

// `typingBits` is the style a collapsed caret will type with (Cmd+B before typing).
export function textState(doc: QuoteDocumentData, selection: TextSelection | undefined, typingBits: number | null = null): TextState | null {
  if (!selection) return null
  const picked = selectedNodes(doc, selection)
  if (!picked) return null
  const { nodes, all, index } = picked
  const collapsed = selection.anchor.nodeId === selection.focus.nodeId && selection.anchor.offset === selection.focus.offset
  const marks = collapsed && typingBits !== null ? { bold: !!(typingBits & 1), italic: !!(typingBits & 2) } : selectionMarks(all, selection.anchor, selection.focus)
  const kinds = nodes.map(listKind)
  const items = nodes.filter(n => n.kind === 'item')
  const numbered = items.filter(n => n.marker === 'decimal')
  const bullets = items.filter(n => n.marker !== 'decimal')
  const labels = markerLabels(all, index + 1)
  const focusIndex = all.findIndex(n => n.id === selection.focus.nodeId)
  const focusNode = all[focusIndex]
  const preview = focusNode && focusNode.kind === 'item' && focusNode.marker === 'decimal' ? labels[focusIndex] || null : null
  const lastPart = preview ? Number(preview.split('.').filter(Boolean).at(-1)) : NaN
  return {
    sectionId: selection.sectionId, sectionIndex: index, count: nodes.length,
    collapsed,
    bold: marks.bold, italic: marks.italic,
    list: same(kinds), kind: same(nodes.map(n => n.kind)),
    bullet: bullets.length ? same(bullets.map(n => (n.marker ?? 'disc') as Exclude<QuoteMarker, 'decimal'>)) : null,
    depth: items.length ? same(items.map(n => n.depth ?? 0)) : null,
    // Tab deepens a paragraph into a list; items stop at the deepest level.
    canIndent: nodes.some(n => n.kind === 'paragraph' || (n.depth ?? 0) < MAX_DEPTH),
    canOutdent: items.length > 0,
    bound: numbered.length ? same(numbered.map(n => !!n.section_bound)) : null,
    continued: numbered.length ? same(numbered.slice(0, 1).map(n => !!n.list_continue)) : null,
    listStart: numbered[0]?.list_start ?? null,
    // How the first numbered item counts: on from the list, on across text, or from a set number.
    sequence: !numbered.length ? null : numbered[0]!.list_continue ? 'continue' : numbered[0]!.list_start ? 'start' : 'follow',
    number: Number.isFinite(lastPart) ? lastPart : null,
    preview,
    markerX: items.length ? same(items.map(n => mm(n.marker_x_mm))) : null,
    markerY: items.length ? same(items.map(n => mm(n.marker_y_mm))) : null,
    textStart: items.length ? same(items.map(n => mm(n.text_start_mm))) : null,
  }
}

// ---------- Scope headers: what each tab is about right now ----------
export type InspectorTab = 'text' | 'section' | 'document'
export interface Scope { eyebrow: string; title: string; detail: string }
const headingOf = (doc: QuoteDocumentData, index: number) => doc.sections[index]?.heading.trim() || 'Untitled section'
export function scopeFor(tab: InspectorTab, doc: QuoteDocumentData, sectionId: string | undefined, text: TextState | null, offerNo: string): Scope | null {
  if (tab === 'document') return { eyebrow: 'Document', title: offerNo || 'Draft quote', detail: doc.title.trim() || 'Untitled quote' }
  const index = sectionId ? doc.sections.findIndex(s => s.id === sectionId) : -1
  if (index < 0) return null
  const where = `Section ${index + 1} of ${doc.sections.length}`
  if (tab === 'section') return { eyebrow: where, title: headingOf(doc, index), detail: sectionSummary(doc, index) }
  if (!text) return null
  const what = text.count > 1 ? `${text.count} paragraphs` : text.kind === 'item' ? `List item, level ${(typeof text.depth === 'number' ? text.depth : 0) + 1}` : 'Paragraph'
  return { eyebrow: where, title: headingOf(doc, index), detail: `${what} · ${text.collapsed ? 'caret' : 'selected text'}` }
}
function sectionSummary(doc: QuoteDocumentData, index: number): string {
  const section = doc.sections[index]!
  const nodes = nodesFor(section.body, section.nodes)
  const items = nodes.filter(n => n.kind === 'item').length
  const paragraphs = nodes.length - items
  const parts: string[] = []
  if (paragraphs) parts.push(`${paragraphs} ${paragraphs === 1 ? 'paragraph' : 'paragraphs'}`)
  if (items) parts.push(`${items} list ${items === 1 ? 'item' : 'items'}`)
  return parts.join(' · ') || 'No text yet'
}

// ---------- Section numbering ----------
export const SECTION_STYLES: { id: SectionNumberingStyle; glyph: string; name: string }[] = [
  { id: 'decimal', glyph: '1', name: 'Numbers' },
  { id: 'upper-roman', glyph: 'I', name: 'Roman capitals' },
  { id: 'lower-roman', glyph: 'i', name: 'Roman small' },
  { id: 'upper-alpha', glyph: 'A', name: 'Letter capitals' },
  { id: 'lower-alpha', glyph: 'a', name: 'Letters small' },
  { id: 'none', glyph: '–', name: 'No number' },
]
function roman(value: number): string {
  const table: [number, string][] = [[1000, 'M'], [900, 'CM'], [500, 'D'], [400, 'CD'], [100, 'C'], [90, 'XC'], [50, 'L'], [40, 'XL'], [10, 'X'], [9, 'IX'], [5, 'V'], [4, 'IV'], [1, 'I']]
  let rest = value, out = ''
  for (const [n, s] of table) while (rest >= n) { out += s; rest -= n }
  return out
}
function alpha(value: number): string {
  let rest = value, out = ''
  while (rest > 0) { const r = (rest - 1) % 26; out = String.fromCharCode(65 + r) + out; rest = Math.floor((rest - 1) / 26) }
  return out
}
// The heading's number: "2.", "II.", "ii.", "B.", "b." or nothing.
export function sectionLabel(position: number, style: SectionNumberingStyle | undefined): string {
  switch (style ?? 'decimal') {
    case 'upper-roman': return `${roman(position)}.`
    case 'lower-roman': return `${roman(position).toLowerCase()}.`
    case 'upper-alpha': return `${alpha(position)}.`
    case 'lower-alpha': return `${alpha(position).toLowerCase()}.`
    case 'none': return ''
    default: return `${position}.`
  }
}

// ---------- Millimetres: one decimal, 0.5 steps by keyboard or scrub ----------
export interface MmRange { min: number; max: number }
export const OFFSET_RANGES = { markerX: { min: -30, max: 30 }, markerY: { min: -20, max: 20 }, textStart: { min: -20, max: 40 } } as const
export const SPACING_RANGE: MmRange = { min: 0, max: 40 }
export const round1 = (value: number) => Math.round(value * 10) / 10
export const clampMm = (value: number, range: MmRange) => round1(Math.min(range.max, Math.max(range.min, value)))
// Text typed in a millimetre field ("2,5", "2.5 mm", "-3") as a number, or null.
export function parseMm(raw: string): number | null {
  const text = raw.trim().replace(/\s*mm$/i, '').replace(',', '.').replace('−', '-').trim()
  if (!/^-?\d{1,3}(\.\d+)?$/.test(text)) return null
  return round1(Number(text))
}
// Arrow keys add or take 0.5 mm (Shift: 5 mm), within range.
export function stepMm(value: number, direction: 1 | -1, big: boolean, range: MmRange): number {
  return clampMm(value + direction * (big ? 5 : 0.5), range)
}
// Scrubbing: every 4px of pointer travel is one 0.5 mm step (Shift: 5 mm).
export function scrubMm(start: number, dx: number, big: boolean, range: MmRange): number {
  const steps = Math.trunc(dx / 4)
  return clampMm(start + steps * (big ? 5 : 0.5), range)
}
export const formatMm = (value: number) => (Object.is(value, -0) ? 0 : round1(value)).toString().replace('-', '−')
// The model's text: one decimal, or null for the default 0.
export const mmText = (value: number): string | null => round1(value) === 0 ? null : round1(value).toFixed(1)
