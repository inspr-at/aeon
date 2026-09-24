// SPDX-License-Identifier: AGPL-3.0-only
import { DocumentHistory, type EditorSnapshot } from './history'
import { documentTotal, exactMM, positionTotal } from './layout'
import { assertProse, changeLevel, markerLabels, nodesFor, project, selectionMarks, setListMode, setMarks, setNumbering, setOffsets } from './prose'
import { newId, type DocumentSettings, type EditorSelection, type ListMode, type MarkName, type NumberingOptions, type OffsetPatch, type QuoteDocumentData, type QuotePosition, type QuoteSection, type TextNode, type TextSelection } from './types'

export class QuoteEditor {
  document: QuoteDocumentData
  selection: EditorSelection = {}
  typingBits: number | null = null
  readonly history = new DocumentHistory()
  onChange?: (document: QuoteDocumentData) => void
  constructor(document: QuoteDocumentData) { this.document = this.hydrate(document) }
  private hydrate(document: QuoteDocumentData): QuoteDocumentData {
    const next = structuredClone(document)
    for (const section of next.sections) if (!section.nodes.length) section.nodes = [{ id: newId(), kind: 'paragraph', text: section.body }]
    return next
  }
  private snapshot(): EditorSnapshot { return { document: this.document, selection: this.selection } }
  private restore(snapshot: EditorSnapshot) {
    this.document = structuredClone(snapshot.document)
    this.selection = structuredClone(snapshot.selection)
    this.onChange?.(this.document)
  }
  private mutate(action: (doc: QuoteDocumentData) => void) {
    const next = structuredClone(this.document)
    action(next)
    if (JSON.stringify(next) === JSON.stringify(this.document)) return
    this.history.record(this.snapshot())
    this.document = next
    this.onChange?.(this.document)
  }
  undo() { const old = this.history.undo(this.snapshot()); if (old) { this.typingBits = null; this.restore(old) } }
  redo() { const next = this.history.redo(this.snapshot()); if (next) { this.typingBits = null; this.restore(next) } }
  select(selection: EditorSelection, preserveTyping = false) {
    const previous = this.selection.text?.focus
    const next = selection.text?.focus
    if (!preserveTyping && previous && next && (previous.nodeId !== next.nodeId || previous.offset !== next.offset)) this.typingBits = null
    this.selection = structuredClone(selection)
  }
  replaceDocument(document: QuoteDocumentData) { this.document = this.hydrate(document); this.selection = {}; this.history.clear(); this.onChange?.(this.document) }
  private section(doc: QuoteDocumentData, id: string): QuoteSection {
    const section = doc.sections.find(s => s.id === id)
    if (!section) throw new Error('Section no longer exists')
    return section
  }
  editField(path: 'title' | 'subtitle' | 'project_ref' | 'offer_date' | 'valid_until', value: string): void
  editField(path: 'sender' | 'recipient' | 'legal', value: { key: string; text: string }): void
  editField(path: string, value: string | { key: string; text: string }) {
    this.mutate(doc => {
      if (typeof value === 'string') (doc as unknown as Record<string, unknown>)[path] = value
      else (doc as unknown as Record<string, Record<string, string>>)[path]![value.key] = value.text
    })
  }
  editSection(id: string, patch: Partial<Pick<QuoteSection, 'heading' | 'body' | 'nodes'>>) {
    this.mutate(doc => {
      const section = this.section(doc, id)
      if (patch.nodes) { assertProse(patch.nodes); section.nodes = structuredClone(patch.nodes); section.body = project(patch.nodes) }
      else if (patch.body !== undefined) { section.body = patch.body; section.nodes = [] }
      if (patch.heading !== undefined) section.heading = patch.heading
    })
  }
  insertSection(afterId?: string): string {
    const id = newId()
    this.mutate(doc => {
      if (doc.sections.length >= 20) throw new Error('At most 20 sections')
      const index = afterId ? doc.sections.findIndex(s => s.id === afterId) : doc.sections.length - 1
      if (afterId && index < 0) throw new Error('Section no longer exists')
      doc.sections.splice(index + 1, 0, { id, heading: '', body: '', nodes: [ { id: newId(), kind: 'paragraph', text: '' } ] })
    })
    this.selection = { sectionId: id }
    return id
  }
  moveSection(id: string, targetIndex: number) {
    this.mutate(doc => {
      const index = doc.sections.findIndex(s => s.id === id)
      if (index < 0) throw new Error('Section no longer exists')
      if (targetIndex < 0 || targetIndex >= doc.sections.length) return
      doc.sections.splice(targetIndex, 0, doc.sections.splice(index, 1)[0]!)
    })
  }
  deleteSection(id: string) {
    this.mutate(doc => { const index = doc.sections.findIndex(s => s.id === id); if (index < 0) return; doc.sections.splice(index, 1) })
    if (this.selection.sectionId === id) this.selection = {}
  }
  addPosition(afterId?: string): string {
    const id = newId()
    this.mutate(doc => {
      if (doc.positions.length >= 100) throw new Error('At most 100 positions')
      const index = afterId ? doc.positions.findIndex(p => p.id === afterId) : doc.positions.length - 1
      if (afterId && index < 0) throw new Error('Position no longer exists')
      doc.positions.splice(index + 1, 0, { id, pricing_source: 'manual', short_text: '', long_text: '', quantity: '1', unit_label: 'item', unit_price_cents: 0, total_cents: 0, currency: doc.currency })
    })
    this.selection = { positionId: id }
    return id
  }
  editPosition(id: string, patch: Partial<QuotePosition>) {
    this.mutate(doc => {
      const row = doc.positions.find(p => p.id === id)
      if (!row) throw new Error('Position no longer exists')
      Object.assign(row, patch)
      row.total_cents = positionTotal(row)
      doc.net_total_cents = documentTotal(doc.positions)
    })
  }
  movePosition(id: string, targetIndex: number) {
    this.mutate(doc => {
      const index = doc.positions.findIndex(p => p.id === id)
      if (index < 0 || targetIndex < 0 || targetIndex >= doc.positions.length) return
      doc.positions.splice(targetIndex, 0, doc.positions.splice(index, 1)[0]!)
    })
  }
  deletePosition(id: string) {
    this.mutate(doc => { const index = doc.positions.findIndex(p => p.id === id); if (index >= 0) { doc.positions.splice(index, 1); doc.net_total_cents = documentTotal(doc.positions) } })
    if (this.selection.positionId === id) this.selection = {}
  }
  private textNodes(doc: QuoteDocumentData, selection: TextSelection) { const section = this.section(doc, selection.sectionId); return { section, nodes: nodesFor(section.body, section.nodes) } }
  private selectedIds(nodes: readonly TextNode[], selection: TextSelection): string[] {
    const a = nodes.findIndex(n => n.id === selection.anchor.nodeId), b = nodes.findIndex(n => n.id === selection.focus.nodeId)
    if (a < 0 || b < 0) throw new Error('Text selection no longer exists')
    return nodes.slice(Math.min(a, b), Math.max(a, b) + 1).map(n => n.id)
  }
  private transformText(fn: (nodes: TextNode[], selected: string[], selection: TextSelection) => TextNode[]) {
    const selection = this.selection.text
    if (!selection) return
    this.mutate(doc => {
      const { section, nodes } = this.textNodes(doc, selection)
      section.nodes = fn(nodes, this.selectedIds(nodes, selection), selection)
      section.body = project(section.nodes)
    })
  }
  setMarks(mark: MarkName | 'normal', active?: boolean) {
    const selection = this.selection.text
    if (!selection) return
    if (selection.anchor.nodeId === selection.focus.nodeId && selection.anchor.offset === selection.focus.offset) {
      const { nodes } = this.textNodes(this.document, selection)
      const state = selectionMarks(nodes, selection.anchor, selection.focus)
      const before = this.typingBits ?? (state.bold === true ? 1 : 0) | (state.italic === true ? 2 : 0)
      const flag = mark === 'bold' ? 1 : 2
      this.typingBits = mark === 'normal' ? 0 : (active ?? !(before & flag)) ? before | flag : before & ~flag
      return
    }
    this.typingBits = null
    this.transformText((nodes, _ids, selected) => setMarks(nodes, selected.anchor, selected.focus, mark, active))
  }
  setListMode(mode: ListMode, bullet?: Exclude<import('./types').QuoteMarker, 'decimal'>) { this.transformText((nodes, ids) => setListMode(nodes, ids, mode, bullet)) }
  indent() { this.transformText((nodes, ids) => changeLevel(nodes, ids, 1)) }
  outdent() { this.transformText((nodes, ids) => changeLevel(nodes, ids, -1)) }
  setNumbering(options: NumberingOptions) { this.transformText((nodes, ids) => setNumbering(nodes, ids, options)) }
  setOffsets(patch: OffsetPatch) { this.transformText((nodes, ids) => setOffsets(nodes, ids, patch)) }
  markerPreview(): string | null {
    const selection = this.selection.text
    if (!selection) return null
    const sectionIndex = this.document.sections.findIndex(s => s.id === selection.sectionId)
    if (sectionIndex < 0) return null
    const nodes = nodesFor(this.document.sections[sectionIndex]!.body, this.document.sections[sectionIndex]!.nodes)
    const index = nodes.findIndex(n => n.id === selection.focus.nodeId)
    return index < 0 ? null : markerLabels(nodes, sectionIndex + 1)[index] || null
  }
  setDocumentSettings(patch: DocumentSettings) {
    this.mutate(doc => {
      if (patch.currency && patch.currency !== doc.currency && doc.positions.length) throw new Error('Remove positions before changing currency')
      for (const key of ['title', 'subtitle', 'project_ref', 'offer_date', 'valid_until', 'currency'] as const) if (patch[key] !== undefined) doc[key] = patch[key]!
      if (patch.legal) Object.assign(doc.legal, patch.legal)
      if (patch.layout) {
        const layout = { ...patch.layout }
        if ('logo_width_mm' in layout) layout.logo_width_mm = exactMM(layout.logo_width_mm, 18, 96)
        if ('logo_offset_mm' in layout) layout.logo_offset_mm = exactMM(layout.logo_offset_mm, -6, 10)
        Object.assign(doc.layout, layout)
      }
    })
  }
  // P1 has no persisted section formatting fields. The inspector must not imply a save succeeded.
  setSectionSettings(_id: string, _patch: { pageBreakBefore?: boolean; keepTogether?: boolean; spacingMm?: string; numberingStyle?: string }): never {
    throw new Error('Section formatting requires a P1 quote contract addition')
  }
}
