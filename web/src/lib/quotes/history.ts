// SPDX-License-Identifier: AGPL-3.0-only
import type { EditorSelection, QuoteDocumentData } from './types'
export interface EditorSnapshot { document: QuoteDocumentData; selection: EditorSelection }
export class DocumentHistory {
  private past: EditorSnapshot[] = []
  private future: EditorSnapshot[] = []
  private limit: number
  constructor(limit = 100) { this.limit = limit }
  get canUndo() { return this.past.length > 0 }
  get canRedo() { return this.future.length > 0 }
  record(before: EditorSnapshot): void {
    this.past.push(structuredClone(before))
    if (this.past.length > this.limit) this.past.shift()
    this.future = []
  }
  undo(current: EditorSnapshot): EditorSnapshot | null {
    const prior = this.past.pop()
    if (!prior) return null
    this.future.push(structuredClone(current))
    return structuredClone(prior)
  }
  redo(current: EditorSnapshot): EditorSnapshot | null {
    const next = this.future.pop()
    if (!next) return null
    this.past.push(structuredClone(current))
    return structuredClone(next)
  }
  clear() { this.past = []; this.future = [] }
}
