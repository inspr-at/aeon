// SPDX-License-Identifier: AGPL-3.0-only
// What a quote row offers now, derived from what the server allows in the quote's
// flow state (internal/business/quotes: create, duplicate, finalize, branch,
// visibility, delete and the public link guards). Actions a person cannot take
// are left out, except where leaving them out would puzzle: issuing and revising
// show for everyone who works on quotes, with the reason when they are not theirs.
import type { RowAction } from '../rowActions'
import { statusOf, type QuoteRow } from './list'

export type QuoteActionId = 'open' | 'openPage' | 'copyNumber' | 'link' | 'duplicate' | 'pdf' | 'issue' | 'revise' | 'archive' | 'restore' | 'delete'
// The customer link of the current version, once the menu has asked for it.
export type LinkState = 'loading' | 'none' | 'copy' | 'hidden' | 'ended' | 'error'
// `narrow`: a phone, where a quote opens on its own page (there is no room beside the list).
export interface QuoteActionContext { admin: boolean; staff: boolean; link: LinkState; busy?: QuoteActionId | null; narrow?: boolean }

export function quoteActions(row: QuoteRow, ctx: QuoteActionContext): (RowAction & { id: QuoteActionId })[] {
  const status = statusOf(row)
  const draft = row.state === 'draft'
  const everIssued = row.current_version > 0
  const out: (RowAction & { id: QuoteActionId })[] = []
  const add = (a: RowAction & { id: QuoteActionId }) => out.push(ctx.busy === a.id ? { ...a, busy: true } : a)

  if (ctx.narrow) add({ id: 'openPage', label: 'Open', icon: 'expand', group: 0 })
  else {
    add({ id: 'open', label: 'Open beside the list', icon: 'eye', group: 0, keys: 'Enter' })
    add({ id: 'openPage', label: 'Open on its own page', icon: 'expand', group: 0, keys: 'Shift+Enter' })
  }

  if (row.offer_no) add({ id: 'copyNumber', label: 'Copy quote number', icon: 'tag', group: 1 })
  // The customer link belongs to the issued version; only admins read or create it.
  if (ctx.admin && !draft && status !== 'void') {
    const link = { id: 'link' as const, icon: 'link' as const, group: 1 }
    switch (ctx.link) {
      case 'copy': add({ ...link, label: 'Copy customer link' }); break
      case 'none': if (status !== 'accepted') add({ ...link, label: 'Create customer link' }); break
      case 'ended': add({ ...link, label: 'Copy customer link', reason: 'Its link has ended. Revoke it in Details to create a new one.' }); break
      case 'hidden': add({ ...link, label: 'Copy customer link', reason: 'This older link cannot be copied again. Revoke it in Details to create a new one.' }); break
      case 'error': add({ ...link, label: 'Customer link', reason: 'The link could not be read. Try again in a moment.' }); break
      default: add({ ...link, label: 'Customer link', busy: true })
    }
  }

  if (ctx.staff) add({ id: 'duplicate', label: 'Duplicate as a new draft', icon: 'copy', group: 2 })
  add({ id: 'pdf', label: 'Print or save as PDF', icon: 'print', group: 2 })

  if (draft) {
    add({
      id: 'issue', label: everIssued ? `Issue as version ${row.current_version + 1}…` : 'Issue…', icon: 'seal', group: 3,
      reason: !ctx.admin ? 'Only a workspace admin issues quotes.' : row.archived ? 'Restore it from the archive first.' : !row.offer_no ? 'It has no quote number yet. Set up quote settings first.' : undefined,
    })
  } else if (row.state === 'issued' || row.state === 'accepted') {
    add({
      id: 'revise', label: `Revise as version ${row.current_version + 1}…`, icon: 'edit', group: 3,
      reason: !ctx.admin ? 'Only a workspace admin revises issued quotes.' : row.archived ? 'Restore it from the archive first.' : undefined,
    })
  }

  if (ctx.admin) {
    if (row.archived) add({ id: 'restore', label: 'Restore from the archive', icon: 'rollback', group: 4 })
    else add({ id: 'archive', label: 'Archive', icon: 'archive', group: 4 })
    // Only a draft that was never issued can go; issued versions are kept and archived.
    if (draft && !everIssued) add({ id: 'delete', label: 'Delete draft', icon: 'trash', group: 4, danger: true })
    else if (draft) add({ id: 'delete', label: 'Delete draft', icon: 'trash', group: 4, danger: true, reason: `Version ${row.current_version} was issued and is kept. Archive the quote instead.` })
  }
  return out
}
