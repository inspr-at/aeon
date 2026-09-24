// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test, type Page, type Route } from '@playwright/test'
import { fixtures, mockWork } from '../work-fixtures'
import { crmData, mockCRM } from '../crm-fixtures'
import documentFixture from './fixtures/synthetic-document.json' with { type: 'json' }

const quoteId = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const digest = 'a'.repeat(64)
const sessionId = '44444444-4444-4444-8444-444444444444'

test('customer → quote → inspector save → issue → public acceptance race → receipt', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await mockWork(page, fixtures())
  const crm = crmData()
  const crmCalls = await mockCRM(page, crm)
  let document = structuredClone(documentFixture)
  let draftRevision = 1
  let quoteRevision = 1
  let state = 'draft'
  const calls: { method: string; path: string; body: Record<string, unknown> }[] = []
  await page.route('**/api/quotes**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname, method = request.method()
    let body: Record<string, unknown> = {}
    try { body = request.postDataJSON() as Record<string, unknown> } catch { /* GET */ }
    calls.push({ method, path, body })
    if (path === '/api/quotes' && method === 'POST') {
      if (body.customer_org_node_id !== 'org-new-1') return route.fulfill({ status: 400, json: { error: 'wrong customer' } })
      return route.fulfill({ status: 201, json: { quote_node_id: quoteId, offer_no: 'A260924-01', state, revision: quoteRevision } })
    }
    if (path === '/api/quotes' && method === 'GET') return route.fulfill({ json: [{ quote_node_id: quoteId, offer_no: 'A260924-01', state }] })
    if (path === `/api/quotes/${quoteId}`) return route.fulfill({ json: { quote_node_id: quoteId, offer_no: 'A260924-01', state, revision: quoteRevision } })
    if (path === `/api/quotes/${quoteId}/draft` && method === 'GET') return route.fulfill({ json: {
      document, document_sha256: digest, draft_revision: draftRevision, quote_revision: quoteRevision,
      schema_version: 1, minimum_writer_version: 1, base_version: 0, updated_at: '2026-09-24T09:00:00Z', updated_by_principal_id: sessionId,
    } })
    if (path === `/api/quotes/${quoteId}/draft` && method === 'PATCH') {
      if (request.headers()['if-match'] !== `"qd-${draftRevision}"`) return route.fulfill({ status: 412, json: { error: 'stale' } })
      document = structuredClone(body.document as typeof document)
      draftRevision++
      return route.fulfill({ json: { mutation_id: body.mutation_id, acknowledged_revision: draftRevision,
        acknowledged_quote_revision: quoteRevision, current_revision: draftRevision, current_quote_revision: quoteRevision,
        replayed: false, document, document_sha256: digest, updated_at: '2026-09-24T09:01:00Z', updated_by_principal_id: sessionId } })
    }
    if (path === `/api/quotes/${quoteId}/finalize` && method === 'POST') {
      if (body.expected_quote_revision !== quoteRevision || body.expected_draft_revision !== draftRevision || body.expected_document_sha256 !== digest) return route.fulfill({ status: 409, json: { error: 'stale' } })
      state = 'issued'; quoteRevision++
      return route.fulfill({ json: { quote_node_id: quoteId, state, current_version: 1, revision: quoteRevision } })
    }
    if (path === `/api/quotes/${quoteId}/versions/1/public-link` && method === 'POST') return route.fulfill({ status: 201, json: { url: 'http://127.0.0.1:5175/offers/synthetic/token' } })
    if (path === `/api/quotes/${quoteId}/versions/1/confirmation` && method === 'GET') return route.fulfill({ json: { state: 'ready', receipt_sha256: 'b'.repeat(64), renderer_version: 'aeon-quote-document-1' } })
    if (path === `/api/quotes/${quoteId}/presence` && method === 'POST') return route.fulfill({ json: { session_id: sessionId, snapshot: { sessions: [], draft_revision: draftRevision, quote_revision: quoteRevision, state } } })
    if (path.startsWith(`/api/quotes/${quoteId}/presence`)) return route.fulfill({ json: { sessions: [], draft_revision: draftRevision, quote_revision: quoteRevision, state } })
    if (path.endsWith('/collaboration/stream')) return route.fulfill({ contentType: 'text/event-stream', body: '' })
    return route.fallback()
  })

  await page.goto('/business/customers')
  await page.getByRole('button', { name: 'New customer' }).click()
  const dialog = page.getByRole('dialog', { name: 'New customer' })
  await dialog.getByLabel('Name', { exact: true }).fill('Sample Customer')
  await dialog.getByRole('button', { name: 'Add customer' }).click()
  await expect(page.getByRole('heading', { name: 'Sample Customer', level: 1 })).toBeVisible()
  expect(crmCalls.some(call => call.method === 'POST' && call.path === '/api/crm/organisations')).toBe(true)

  // P7 has not exposed quote creation in the shell yet. Exercise its API
  // boundary and then the real routed editor, keeping that UI gap explicit.
  const created = await page.evaluate(async id => {
    const response = await fetch('/api/quotes', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ title: 'Synthetic service proposal', customer_org_node_id: id }) })
    return { status: response.status, body: await response.json() }
  }, 'org-new-1')
  expect(created.status).toBe(201)
  expect(created.body.offer_no).toBe('A260924-01')
  await page.goto(`/business/quotes/${quoteId}`)
  await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
  await expect(page.getByRole('complementary', { name: 'Format' }).getByRole('tab')).toHaveText(['Text', 'Section', 'Document'])
  await page.getByRole('textbox', { name: 'Angebotstitel' }).fill('Synthetic service proposal revised')
  await page.getByRole('button', { name: 'Save draft' }).click()
  await expect.poll(() => draftRevision).toBe(2)
  expect(document.title).toBe('Synthetic service proposal revised')
  expect(calls.find(call => call.method === 'PATCH' && call.path.endsWith('/draft'))?.body).toMatchObject({ writer_version: 1 })

  const issued = await page.evaluate(async ({ quoteId, quoteRevision, draftRevision, digest }) => {
    const response = await fetch(`/api/quotes/${quoteId}/finalize`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ expected_quote_revision: quoteRevision, expected_draft_revision: draftRevision, expected_document_sha256: digest }) })
    return { status: response.status, body: await response.json() }
  }, { quoteId, quoteRevision, draftRevision, digest })
  expect(issued).toMatchObject({ status: 200, body: { state: 'issued', current_version: 1 } })
  const link = await page.evaluate(async quoteId => {
    const response = await fetch(`/api/quotes/${quoteId}/versions/1/public-link`, { method: 'POST' })
    return response.json()
  }, quoteId)
  expect(link.url).toContain('/offers/synthetic/token')
  expect(calls.some(call => call.path.includes('/email'))).toBe(false)

  let accepted = false
  let attempts = 0
  const publicRequests: { credentials: string; body: Record<string, unknown> }[] = []
  await page.context().route('**/api/public/quotes/synthetic/token**', async (route: Route) => {
    const request = route.request(), path = new URL(request.url()).pathname
    if (path.endsWith('/accept')) {
      attempts++
      publicRequests.push({ credentials: request.headers().cookie ?? '', body: request.postDataJSON() as Record<string, unknown> })
      if (accepted) return route.fulfill({ status: 409, json: { error: 'already accepted' } })
      accepted = true
      return route.fulfill({ status: 201, json: { accepted_at: '2026-09-24T12:00:00Z', confirmation_state: 'pending' } })
    }
    return route.fulfill({ json: { document, offer_no: 'A260924-01', version: 1, content_sha256: digest,
      state: accepted ? 'accepted' : 'issued', expires_at: '2026-10-24T00:00:00Z', acceptable: !accepted, receipt_ready: accepted } })
  })
  const a = await page.context().newPage(), b = await page.context().newPage()
  try {
    await Promise.all([a.goto('/offers/synthetic/token'), b.goto('/offers/synthetic/token')])
    for (const tab of [a, b]) {
      await expect(tab.getByRole('heading', { name: 'Synthetic service proposal revised' }).first()).toBeVisible()
      await tab.getByLabel('Your name').fill('Example Signer')
      await tab.getByLabel('I have reviewed this quote and agree to accept it.').check()
    }
    await a.getByRole('button', { name: 'Accept quote' }).click()
    await expect(a.getByText('This quote has been accepted.')).toBeVisible()
    await b.getByRole('button', { name: 'Accept quote' }).click()
    await expect(b.getByText('This quote can no longer be accepted.')).toBeVisible()
    expect(attempts).toBe(2)
    expect(publicRequests.map(request => request.body.expected_content_sha256)).toEqual([digest, digest])
    expect(publicRequests.every(request => request.credentials === '')).toBe(true)
  } finally { await a.close(); await b.close() }
  const receipt = await page.evaluate(async quoteId => {
    const response = await fetch(`/api/quotes/${quoteId}/versions/1/confirmation`)
    return response.json()
  }, quoteId)
  expect(receipt).toMatchObject({ state: 'ready', renderer_version: 'aeon-quote-document-1' })
})
