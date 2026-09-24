// SPDX-License-Identifier: AGPL-3.0-only
import { exactMM } from './layout'
import { newId, type InlineMark, type ListMode, type MarkName, type NumberingOptions, type OffsetPatch, type QuoteMarker, type TextNode, type TextPoint } from './types'

export const MAX_DEPTH = 5
export const MAX_NODES = 100
export const MAX_CODEPOINTS = 2000
export const BULLETS: Record<Exclude<QuoteMarker, 'decimal'>, string> = { disc: '•', circle: '◦', square: '▪', dash: '–' }
export const paragraph = (text = ''): TextNode => ({ id: newId(), kind: 'paragraph', text })
export const nodesFor = (body: string, nodes: TextNode[]): TextNode[] => nodes.length ? nodes : [paragraph(body)]
export const project = (nodes: readonly TextNode[]): string => nodes.map(node => node.text).join('\n')
export const boundary = (text: string, offset: number): boolean => offset <= 0 || offset >= text.length || !(text.charCodeAt(offset - 1) >= 0xd800 && text.charCodeAt(offset - 1) <= 0xdbff && text.charCodeAt(offset) >= 0xdc00 && text.charCodeAt(offset) <= 0xdfff)
export function assertProse(nodes: readonly TextNode[]) {
  if (!nodes.length || nodes.length > MAX_NODES) throw new Error('Invalid node count')
  const ids = new Set<string>()
  for (const node of nodes) {
    if (ids.has(node.id)) throw new Error('Duplicate text ID')
    ids.add(node.id)
    if ([...node.text].length > MAX_CODEPOINTS) throw new Error('Paragraph is too long')
    if (node.kind === 'item' && ((node.depth ?? 0) > MAX_DEPTH || (node.depth ?? 0) < 0)) throw new Error('Invalid list level')
    let previous = 0
    for (const mark of node.marks ?? []) {
      if (mark.start < previous || mark.end <= mark.start || mark.end > node.text.length || !boundary(node.text, mark.start) || !boundary(node.text, mark.end) || (!mark.bold && !mark.italic)) throw new Error('Invalid inline mark')
      previous = mark.end
    }
  }
}
function bits(node: TextNode): number[] {
  const values = Array<number>(node.text.length).fill(0)
  for (const mark of node.marks ?? []) {
    for (let i = mark.start; i < mark.end; i++) values[i] = (mark.bold ? 1 : 0) | (mark.italic ? 2 : 0)
  }
  return values
}
function fromBits(values: readonly number[]): InlineMark[] | undefined {
  const result: InlineMark[] = []
  for (let i = 0; i < values.length;) {
    const value = values[i] ?? 0
    let end = i + 1
    while (end < values.length && values[end] === value) end++
    if (value) result.push({ start: i, end, ...(value & 1 ? { bold: true } : {}), ...(value & 2 ? { italic: true } : {}) })
    i = end
  }
  return result.length ? result : undefined
}
export function markRuns(node: TextNode): { text: string; bold: boolean; italic: boolean }[] {
  const values = bits(node)
  if (!node.text) return [{ text: '', bold: false, italic: false }]
  const runs: { text: string; bold: boolean; italic: boolean }[] = []
  for (let i = 0; i < node.text.length;) {
    const value = values[i] ?? 0
    let end = i + 1
    while (end < node.text.length && values[end] === value) end++
    runs.push({ text: node.text.slice(i, end), bold: !!(value & 1), italic: !!(value & 2) })
    i = end
  }
  return runs
}
const clamp = (n: number, low: number, high: number) => Math.min(high, Math.max(low, n))
function point(nodes: readonly TextNode[], p: TextPoint): { index: number; offset: number } {
  const index = Math.max(0, nodes.findIndex(n => n.id === p.nodeId))
  const node = nodes[index]!
  let offset = clamp(p.offset, 0, node.text.length)
  if (!boundary(node.text, offset)) offset--
  return { index, offset }
}
function ordered(nodes: readonly TextNode[], a: TextPoint, b: TextPoint) {
  const pa = point(nodes, a), pb = point(nodes, b)
  return pa.index < pb.index || pa.index === pb.index && pa.offset <= pb.offset ? [pa, pb] : [pb, pa]
}
const asPoint = (nodes: readonly TextNode[], index: number, offset: number): TextPoint => ({ nodeId: nodes[index]!.id, offset })
function commitNodes(nodes: TextNode[]) { assertProse(nodes); return nodes }
export function selectionMarks(nodes: readonly TextNode[], a: TextPoint, b: TextPoint): { bold: boolean | 'mixed'; italic: boolean | 'mixed' } {
  const [start, end] = ordered(nodes, a, b)
  const covered: number[] = []
  for (let i = start.index; i <= end.index; i++) {
    const v = bits(nodes[i]!)
    const from = i === start.index ? start.offset : 0
    const to = i === end.index ? end.offset : v.length
    covered.push(...v.slice(from, to))
  }
  if (!covered.length) {
    const v = bits(nodes[start.index]!)
    covered.push(v[start.offset - 1] ?? v[start.offset] ?? 0)
  }
  const state = (flag: number): boolean | 'mixed' => covered.every(v => !!(v & flag)) ? true : covered.every(v => !(v & flag)) ? false : 'mixed'
  return { bold: state(1), italic: state(2) }
}
export function setMarks(nodes: readonly TextNode[], a: TextPoint, b: TextPoint, mark: MarkName | 'normal', active?: boolean): TextNode[] {
  const [start, end] = ordered(nodes, a, b)
  const next = structuredClone(nodes) as TextNode[]
  const current = selectionMarks(nodes, a, b)
  const turnOn = active ?? (mark === 'normal' ? false : current[mark] !== true)
  for (let i = start.index; i <= end.index; i++) {
    const node = next[i]!, value = bits(node)
    const from = i === start.index ? start.offset : 0
    const to = i === end.index ? end.offset : node.text.length
    for (let j = from; j < to; j++) value[j] = mark === 'normal' ? 0 : turnOn ? (value[j] ?? 0) | (mark === 'bold' ? 1 : 2) : (value[j] ?? 0) & ~(mark === 'bold' ? 1 : 2)
    node.marks = fromBits(value)
  }
  return commitNodes(next)
}
export function replaceText(nodes: readonly TextNode[], a: TextPoint, b: TextPoint, inserted: string, typingBits?: number): { nodes: TextNode[]; caret: TextPoint } {
  const [start, end] = ordered(nodes, a, b)
  const left = nodes[start.index]!, right = nodes[end.index]!
  const clean = inserted.replace(/\r\n?/g, '\n')
  const chunks = clean.split('\n')
  const prefix = left.text.slice(0, start.offset), suffix = right.text.slice(end.offset)
  const before = bits(left).slice(0, start.offset), after = bits(right).slice(end.offset)
  const inherited = typingBits ?? before.at(-1) ?? after[0] ?? 0
  const make = (source: TextNode, text: string, value: number[]): TextNode => ({ ...source, text, marks: fromBits(value) })
  const replacements: TextNode[] = []
  if (chunks.length === 1) {
    const middle = chunks[0]!
    replacements.push(make(left, prefix + middle + suffix, before.concat(Array(middle.length).fill(inherited), after)))
  } else {
    replacements.push(make(left, prefix + chunks[0]!, before.concat(Array(chunks[0]!.length).fill(inherited))))
    for (let i = 1; i < chunks.length - 1; i++) replacements.push(make({ ...left, id: newId(), list_start: undefined, list_continue: undefined }, chunks[i]!, Array(chunks[i]!.length).fill(inherited)))
    const last = chunks.at(-1)!
    replacements.push(make({ ...left, id: newId(), list_start: undefined, list_continue: undefined }, last + suffix, Array(last.length).fill(inherited).concat(after)))
  }
  const next = commitNodes([...nodes.slice(0, start.index), ...replacements, ...nodes.slice(end.index + 1)] as TextNode[])
  const at = start.index + replacements.length - 1
  const offset = replacements.length === 1 ? prefix.length + chunks[0]!.length : chunks.at(-1)!.length
  return { nodes: next, caret: asPoint(next, at, offset) }
}
export function deleteBackward(nodes: readonly TextNode[], caret: TextPoint): { nodes: TextNode[]; caret: TextPoint } {
  const p = point(nodes, caret)
  if (p.offset > 0) {
    const segment = new Intl.Segmenter(undefined, { granularity: 'grapheme' })
    let previous = 0
    for (const part of segment.segment(nodes[p.index]!.text.slice(0, p.offset))) previous = part.index
    return replaceText(nodes, asPoint(nodes, p.index, previous), caret, '')
  }
  if (p.index === 0) return { nodes: [...nodes], caret }
  const prior = nodes[p.index - 1]!
  return replaceText(nodes, { nodeId: prior.id, offset: prior.text.length }, caret, '')
}
export function deleteForward(nodes: readonly TextNode[], caret: TextPoint): { nodes: TextNode[]; caret: TextPoint } {
  const p = point(nodes, caret), node = nodes[p.index]!
  if (p.offset < node.text.length) {
    const next = [...new Intl.Segmenter(undefined, { granularity: 'grapheme' }).segment(node.text.slice(p.offset))][0]
    return replaceText(nodes, caret, asPoint(nodes, p.index, p.offset + (next?.segment.length ?? 1)), '')
  }
  if (p.index === nodes.length - 1) return { nodes: [...nodes], caret }
  return replaceText(nodes, caret, asPoint(nodes, p.index + 1, 0), '')
}
export function setListMode(nodes: readonly TextNode[], ids: readonly string[], mode: ListMode, bullet: Exclude<QuoteMarker, 'decimal'> = 'disc'): TextNode[] {
  const selected = new Set(ids)
  return commitNodes(nodes.map(node => {
    if (!selected.has(node.id)) return structuredClone(node)
    if (mode === 'none') return { id: node.id, kind: 'paragraph' as const, text: node.text, ...(node.marks ? { marks: node.marks } : {}) }
    return { ...node, kind: 'item' as const, marker: mode === 'numbered' ? 'decimal' as const : bullet,
      depth: clamp(node.depth ?? 0, 0, MAX_DEPTH), numbering: mode === 'numbered' ? 'outline' as const : undefined,
      list_start: mode === 'numbered' ? node.list_start : undefined, list_continue: mode === 'numbered' ? node.list_continue : undefined,
      section_bound: mode === 'numbered' ? node.section_bound : undefined, glyph: mode === 'numbered' ? undefined : node.glyph }
  }))
}
export function changeLevel(nodes: readonly TextNode[], ids: readonly string[], direction: 1 | -1): TextNode[] {
  const selected = new Set(ids)
  return commitNodes(nodes.map(node => {
    if (!selected.has(node.id)) return structuredClone(node)
    if (node.kind === 'paragraph') return direction === 1 ? { ...node, kind: 'item' as const, marker: 'disc' as const, depth: 0 } : structuredClone(node)
    const depth = (node.depth ?? 0) + direction
    if (depth < 0) return { id: node.id, kind: 'paragraph' as const, text: node.text, ...(node.marks ? { marks: node.marks } : {}) }
    if (depth > MAX_DEPTH) return structuredClone(node)
    return { ...node, depth, list_start: undefined }
  }))
}
export function setNumbering(nodes: readonly TextNode[], ids: readonly string[], options: NumberingOptions): TextNode[] {
  if (options.mode === 'start' && (!Number.isInteger(options.start) || options.start! < 1 || options.start! > 9999)) throw new Error('Invalid list start')
  const selected = new Set(ids)
  let first = true
  return commitNodes(nodes.map(node => {
    if (!selected.has(node.id)) return structuredClone(node)
    const mode = first ? options.mode : options.mode === 'section' ? 'bound' : options.mode === 'independent' ? 'unbound' : 'follow'
    first = false
    const bound = mode === 'section' || mode === 'bound' ? true : mode === 'independent' || mode === 'unbound' ? false : options.sectionBound ?? node.section_bound
    const base: TextNode = { ...node, kind: 'item', marker: 'decimal', numbering: 'outline', depth: node.depth ?? 0, section_bound: bound, glyph: undefined, list_start: undefined, list_continue: undefined }
    if (mode === 'restart') base.list_start = 1
    if (mode === 'start') base.list_start = options.start
    if (mode === 'continue') base.list_continue = true
    if (['section', 'bound', 'independent', 'unbound'].includes(mode)) { base.list_start = node.list_start; base.list_continue = node.list_continue }
    return base
  }))
}
export function setOffsets(nodes: readonly TextNode[], ids: readonly string[], patch: OffsetPatch): TextNode[] {
  const selected = new Set(ids)
  return commitNodes(nodes.map(node => {
    if (!selected.has(node.id) || node.kind !== 'item') return structuredClone(node)
    const next = { ...node }
    if ('markerX' in patch) next.marker_x_mm = exactMM(patch.markerX, -30, 30)
    if ('markerY' in patch) next.marker_y_mm = exactMM(patch.markerY, -20, 20)
    if ('textStart' in patch) next.text_start_mm = exactMM(patch.textStart, -20, 40)
    return next
  }))
}
export function markerLabels(nodes: readonly TextNode[], sectionNumber = 0): string[] {
  const plain = Array<number>(6).fill(0), outline = Array<number>(6).fill(0)
  const snapshots: number[][] = []
  return nodes.map((node, index) => {
    if (node.kind === 'paragraph') { plain.fill(0); outline.fill(0); return '' }
    const depth = clamp(node.depth ?? 0, 0, 5)
    if (node.marker !== 'decimal') {
      for (let d = depth; d < 6; d++) { plain[d] = 0; outline[d] = 0 }
      return node.glyph || BULLETS[node.marker ?? 'disc']
    }
    const values = node.numbering === 'outline' ? outline : plain
    for (let d = depth + 1; d < 6; d++) values[d] = 0
    if (node.list_start && node.list_start > 0) values[depth] = node.list_start
    else if (node.list_continue) {
      let previous = -1
      for (let j = index - 1; j >= 0; j--) {
        const candidate = nodes[j]!
        if (candidate.kind === 'item' && candidate.marker === 'decimal' && (candidate.depth ?? 0) === depth && candidate.numbering === node.numbering && !!candidate.section_bound === !!node.section_bound) { previous = j; break }
      }
      const remembered = snapshots[previous]
      if (remembered) for (let d = 0; d <= depth; d++) values[d] = remembered[d] ?? 0
      values[depth] = (values[depth] ?? 0) + 1
    } else values[depth] = (values[depth] ?? 0) + 1 || 1
    snapshots[index] = values.slice()
    if (node.numbering !== 'outline') return `${values[depth]}.`
    const parts = values.slice(0, depth + 1).filter(value => value > 0)
    if (!parts.length) parts.push(1)
    return [...(node.section_bound && sectionNumber ? [sectionNumber] : []), ...parts].join('.')
  })
}
// DOM input reconciliation keeps unchanged marks and derives the inserted style from nearby text.
export function reconcileInput(node: TextNode, nextText: string, typingBits?: number): TextNode {
  if (node.text === nextText) return node
  let prefix = 0
  while (prefix < node.text.length && prefix < nextText.length && node.text[prefix] === nextText[prefix]) prefix++
  while (!boundary(node.text, prefix)) prefix--
  let suffix = 0
  while (suffix < node.text.length - prefix && suffix < nextText.length - prefix && node.text.at(-1 - suffix) === nextText.at(-1 - suffix)) suffix++
  while (suffix && (!boundary(node.text, node.text.length - suffix) || !boundary(nextText, nextText.length - suffix))) suffix--
  const old = bits(node)
  const first = old.slice(0, prefix), last = suffix ? old.slice(-suffix) : []
  const inherited = typingBits ?? first.at(-1) ?? last[0] ?? 0
  const result = { ...node, text: nextText, marks: fromBits(first.concat(Array(nextText.length - prefix - suffix).fill(inherited), last)) }
  assertProse([result])
  return result
}
