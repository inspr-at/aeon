// SPDX-License-Identifier: AGPL-3.0-only
// Section actions shared by the handle and context menu on the page and by the
// outline in the inspector: add below, move, delete. They call P3's editor
// commands (one history step each). Deleting asks nothing; its toast offers
// Undo, which undoes the deletion only while nothing else has changed since.
import { toast } from '../toast'
import type { QuoteEditor } from './editor'

export interface SectionActions {
  addBelow(id: string): string | null
  move(id: string, direction: -1 | 1): boolean
  moveTo(id: string, index: number): void
  remove(id: string): void
}
export function sectionActions(editor: QuoteEditor, changed: () => void): SectionActions {
  const indexOf = (id: string) => editor.document.sections.findIndex(s => s.id === id)
  const attempt = <T>(run: () => T, fallback: T): T => {
    try { return run() } catch (e) { toast(e instanceof Error ? plain(e.message) : 'That did not work.', { tone: 'error' }); return fallback }
  }
  return {
    addBelow(id) {
      const created = attempt(() => editor.insertSection(id), null)
      changed()
      return created
    },
    move(id, direction) {
      const index = indexOf(id)
      const target = index + direction
      if (index < 0 || target < 0 || target >= editor.document.sections.length) return false
      attempt(() => editor.moveSection(id, target), undefined)
      changed()
      return true
    },
    moveTo(id, index) {
      if (indexOf(id) === index) return
      attempt(() => editor.moveSection(id, index), undefined)
      changed()
    },
    remove(id) {
      const index = indexOf(id)
      if (index < 0) return
      const heading = editor.document.sections[index]!.heading.trim()
      attempt(() => editor.deleteSection(id), undefined)
      changed()
      const after = JSON.stringify(editor.document)
      toast(`Deleted section ${index + 1}${heading ? `, “${heading}”` : ''}.`, {
        timeout: 8000,
        action: {
          label: 'Undo', run: () => {
            if (JSON.stringify(editor.document) !== after) { toast('Later changes came after it. Use Undo in the title bar to step back.', { tone: 'error' }); return }
            editor.undo(); changed()
          },
        },
      })
    },
  }
}
// The editor's limits, as people say them.
function plain(message: string) {
  if (/20 sections/.test(message)) return 'A quote holds at most 20 sections.'
  if (/no longer exists/.test(message)) return 'That section is gone. It may have been removed meanwhile.'
  return message
}
