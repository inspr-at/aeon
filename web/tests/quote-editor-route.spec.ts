// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test } from '@playwright/test'
import { fixtures, mockWork } from './work-fixtures'
import { businessData, mockBusiness } from './business-fixtures'

const quoteId = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const document = {
  schema_version: 1, minimum_writer_version: 1, title: 'A real draft', subtitle: 'For the customer',
  project_ref: '', offer_date: '2026-09-24', valid_until: '2026-10-24', currency: 'EUR',
  sender: { company: 'Example Studio' }, recipient: { name: 'Example Customer' }, legal: {}, layout: {},
  sections: [{ id: '11111111-1111-4111-8111-111111111111', heading: 'Scope', body: '', nodes: [] }],
  positions: [], net_total_cents: 0,
}

test('Quotes list opens a live draft in the session and presence editor', async ({ page }) => {
  await mockWork(page, fixtures())
  await mockBusiness(page, businessData({ enabled: ['business_costs', 'business_crm', 'business_quotes', 'business_hours'] }))
  let presenceJoined = false
  await page.route('**/api/quotes**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/quotes') return route.fulfill({ json: [{ quote_node_id: quoteId, offer_no: 'A260924-1', state: 'draft' }] })
    if (path === `/api/quotes/${quoteId}/draft`) return route.fulfill({ json: {
      document, document_sha256: 'a'.repeat(64), draft_revision: 1, quote_revision: 1,
      schema_version: 1, minimum_writer_version: 1, base_version: 0,
      updated_at: '2026-09-24T09:00:00Z', updated_by_principal_id: '22222222-2222-4222-8222-222222222222',
    } })
    if (path === `/api/quotes/${quoteId}/presence`) {
      presenceJoined = true
      return route.fulfill({ json: { session_id: '33333333-3333-4333-8333-333333333333', snapshot: {
        sessions: [], draft_revision: 1, quote_revision: 1, state: 'draft',
      } } })
    }
    if (path.endsWith('/collaboration/stream')) return route.fulfill({ contentType: 'text/event-stream', body: '' })
    if (path === `/api/quotes/${quoteId}`) return route.fulfill({ json: { quote_node_id: quoteId, offer_no: 'A260924-1', state: 'draft' } })
    return route.fallback()
  })
  await page.goto('/business/quotes')
  // A row opens the quote beside the list (U18); from there it opens on its own page.
  await page.getByRole('row', { name: /A260924-1/ }).click()
  await expect(page).toHaveURL(new RegExp(`/business/quotes\\?quote=${quoteId}$`))
  await page.locator('.quote-dock').getByRole('button', { name: 'Open on its own page' }).click()
  await expect(page).toHaveURL(new RegExp(`/business/quotes/${quoteId}$`))
  await expect(page.getByRole('region', { name: 'Quote editor' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: 'Angebotstitel' })).toHaveText('A real draft')
  await expect(page.getByRole('textbox', { name: 'Untertitel' })).toHaveText('For the customer')
  await expect(page.getByRole('button', { name: 'Save draft' })).toBeDisabled()
  await expect.poll(() => presenceJoined).toBe(true)
})

test('draft save shows field reasons from a 400 response', async ({ page }) => {
  await mockWork(page, fixtures())
  await mockBusiness(page, businessData({ enabled: ['business_costs', 'business_crm', 'business_quotes', 'business_hours'] }))
  await page.route('**/api/quotes**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === `/api/quotes/${quoteId}/draft` && route.request().method() === 'PATCH') {
      return route.fulfill({ status: 400, json: { error: 'invalid quote document', errors: { 'document.sections.nodes.marker_x_mm': 'must be string' } } })
    }
    if (path === `/api/quotes/${quoteId}/draft`) return route.fulfill({ json: {
      document, document_sha256: 'a'.repeat(64), draft_revision: 1, quote_revision: 1,
      schema_version: 1, minimum_writer_version: 1, base_version: 0,
      updated_at: '2026-09-24T09:00:00Z', updated_by_principal_id: '22222222-2222-4222-8222-222222222222',
    } })
    if (path === `/api/quotes/${quoteId}/presence`) return route.fulfill({ json: { session_id: '33333333-3333-4333-8333-333333333333', snapshot: {
      sessions: [], draft_revision: 1, quote_revision: 1, state: 'draft',
    } } })
    if (path.endsWith('/collaboration/stream')) return route.fulfill({ contentType: 'text/event-stream', body: '' })
    if (path === `/api/quotes/${quoteId}`) return route.fulfill({ json: { quote_node_id: quoteId, offer_no: 'A260924-1', state: 'draft' } })
    return route.fallback()
  })
  await page.goto(`/business/quotes/${quoteId}`)
  const title = page.getByRole('textbox', { name: 'Angebotstitel' })
  await title.fill('Changed synthetic draft')
  await page.getByRole('button', { name: 'Save draft' }).click()
  await expect(page.getByRole('alert')).toContainText('document.sections.nodes.marker_x_mm: must be string')
})
