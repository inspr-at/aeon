// SPDX-License-Identifier: AGPL-3.0-only
// QL1 (AEON-109): what a quote row and a customer row offer in each flow state,
// mirroring the server's guards (issue and revise are an admin's, only a draft
// that was never issued can be deleted, issued versions are archived instead).
import { describe, expect, it } from 'vitest'
import { quoteActions, type LinkState } from '../src/lib/quotes/actions'
import { linkState } from '../src/lib/quotes/lifecycle'
import { customerActions, type Customer } from '../src/lib/crm'
import { opensRowMenu } from '../src/lib/rowActions'
import type { QuoteRow } from '../src/lib/quotes/list'

const row = (patch: Partial<QuoteRow>): QuoteRow => ({
  quote_node_id: 'q', project_node_id: '', customer_org_node_id: 'org-a', current_version: 0, state: 'draft', revision: 1, archived: false, offer_no: 'A260924-01',
  project_ref: '', classic_status: 'draft', key: 'QUO-1', title: 'Quote', customer_name: 'Alpha', created_at: '2026-09-01T09:00:00Z', updated_at: '2026-09-01T09:00:00Z', ...patch,
})
const admin = { admin: true, staff: true, link: 'copy' as LinkState }
const member = { admin: false, staff: true, link: 'loading' as LinkState }
const ids = (list: { id: string }[]) => list.map(a => a.id)
const find = <T extends { id: string }>(list: T[], id: string) => list.find(a => a.id === id)

describe('quote row actions', () => {
  it('offers a never-issued draft everything, delete included', () => {
    const list = quoteActions(row({}), admin)
    expect(ids(list)).toEqual(['open', 'openPage', 'copyNumber', 'duplicate', 'pdf', 'issue', 'archive', 'delete'])
    expect(find(list, 'delete')).toMatchObject({ danger: true })
    expect(find(list, 'delete')?.reason).toBeUndefined()
    expect(find(list, 'issue')?.label).toBe('Issue…')
    expect(find(list, 'issue')?.reason).toBeUndefined()
  })
  it('keeps an issued version: archive instead of delete, revise, the customer link', () => {
    const list = quoteActions(row({ state: 'issued', classic_status: 'sent', current_version: 1 }), admin)
    expect(ids(list)).toEqual(['open', 'openPage', 'copyNumber', 'link', 'duplicate', 'pdf', 'revise', 'archive'])
    expect(find(list, 'revise')?.label).toBe('Revise as version 2…')
    expect(find(list, 'link')?.label).toBe('Copy customer link')
  })
  it('shows delete on a revising draft as unavailable, with the reason', () => {
    const list = quoteActions(row({ current_version: 2 }), admin)
    expect(find(list, 'delete')?.reason).toMatch(/Version 2 was issued and is kept\. Archive/)
    expect(find(list, 'issue')?.label).toBe('Issue as version 3…')
  })
  it('explains to a member why issuing and revising are not theirs, and hides admin actions', () => {
    const draft = quoteActions(row({}), member)
    expect(find(draft, 'issue')?.reason).toBe('Only a workspace admin issues quotes.')
    expect(ids(draft)).not.toContain('archive')
    expect(ids(draft)).not.toContain('delete')
    const issued = quoteActions(row({ state: 'issued', current_version: 1, classic_status: 'sent' }), member)
    expect(find(issued, 'revise')?.reason).toMatch(/Only a workspace admin/)
    expect(ids(issued)).not.toContain('link')
  })
  it('restores an archived quote before issuing or revising it', () => {
    const list = quoteActions(row({ archived: true }), admin)
    expect(ids(list)).toContain('restore')
    expect(ids(list)).not.toContain('archive')
    expect(find(list, 'issue')?.reason).toBe('Restore it from the archive first.')
  })
  it('follows the customer link through its states', () => {
    const accepted = row({ state: 'accepted', classic_status: 'accepted', current_version: 1 })
    expect(find(quoteActions(accepted, { ...admin, link: 'none' }), 'link')).toBeUndefined()
    const issued = row({ state: 'issued', classic_status: 'sent', current_version: 1 })
    expect(find(quoteActions(issued, { ...admin, link: 'none' }), 'link')?.label).toBe('Create customer link')
    expect(find(quoteActions(issued, { ...admin, link: 'hidden' }), 'link')?.reason).toMatch(/cannot be copied again/)
    expect(find(quoteActions(issued, { ...admin, link: 'ended' }), 'link')?.reason).toMatch(/has ended/)
    expect(find(quoteActions(issued, { ...admin, link: 'loading' }), 'link')?.busy).toBe(true)
    expect(find(quoteActions(issued, { ...admin, link: 'copy', busy: 'link' }), 'link')?.busy).toBe(true)
  })
  it('reads a link as copyable only while it is open and its address is known', () => {
    const base = { id: 'l', public_tenant: 't', quote_node_id: 'q', version: 1, target_content_sha256: 'a', expires_at: '2026-10-01T00:00:00Z' }
    const now = Date.parse('2026-09-25T00:00:00Z')
    expect(linkState(null, now)).toBe('none')
    expect(linkState({ ...base, revoked_at: '2026-09-20T00:00:00Z' }, now)).toBe('none')
    expect(linkState({ ...base, path: '/offers/t/x' }, now)).toBe('copy')
    expect(linkState(base, now)).toBe('hidden')
    expect(linkState({ ...base, path: '/offers/t/x' }, Date.parse('2026-10-02T00:00:00Z'))).toBe('ended')
  })
  it('opens on its own page on phones', () => {
    const list = quoteActions(row({}), { ...admin, narrow: true })
    expect(ids(list).slice(0, 2)).toEqual(['openPage', 'copyNumber'])
    expect(find(list, 'openPage')?.label).toBe('Open')
  })
  it('never offers a copy of a number the quote does not have', () => {
    expect(ids(quoteActions(row({ offer_no: undefined }), admin))).not.toContain('copyNumber')
    expect(find(quoteActions(row({ offer_no: undefined }), admin), 'issue')?.reason).toMatch(/no quote number/)
  })
})

describe('customer row actions', () => {
  const customer = (patch: Partial<Customer> = {}) => ({ id: 'org-a', key: 'ORG-1', name: 'Alpha', revision: 1, customer_no: 'K-1001', primary_contact_node_id: null, archived: false, ...patch }) as Customer
  it('offers open, a new quote, the number and archive', () => {
    expect(ids(customerActions(customer(), { admin: true, canQuote: true }))).toEqual(['open', 'quote', 'copyNumber', 'archive'])
    expect(ids(customerActions(customer({ customer_no: null }), { admin: false, canQuote: false }))).toEqual(['open'])
  })
  it('restores an archived customer before a new quote', () => {
    const list = customerActions(customer({ archived: true }), { admin: true, canQuote: true })
    expect(ids(list)).toContain('restore')
    expect(find(list, 'quote')?.reason).toBe('Restore it from the archive first.')
  })
})

describe('row menu keys', () => {
  it('opens on the context-menu key and on Shift+F10 only', () => {
    expect(opensRowMenu({ key: 'ContextMenu', shiftKey: false } as KeyboardEvent)).toBe(true)
    expect(opensRowMenu({ key: 'F10', shiftKey: true } as KeyboardEvent)).toBe(true)
    expect(opensRowMenu({ key: 'F10', shiftKey: false } as KeyboardEvent)).toBe(false)
  })
})
