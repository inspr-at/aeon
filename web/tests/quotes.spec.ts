// SPDX-License-Identifier: AGPL-3.0-only
// Coordinator mounts this view with mocked contract routes during release QA.
import { test, expect, type Page } from '@playwright/test'

const quoteId = '10000000-0000-4000-8000-000000000001'
const projectId = '20000000-0000-4000-8000-000000000001'
const orgId = '30000000-0000-4000-8000-000000000001'
const contactId = '40000000-0000-4000-8000-000000000001'
const costId = '50000000-0000-4000-8000-000000000001'
const digest = 'a'.repeat(64)
async function setup(page: Page, customer = false) {
  const quote = { quote_node_id: quoteId, project_node_id: projectId, customer_org_node_id: orgId, current_version: 1, state: customer ? 'issued' : 'draft', revision: customer ? 3 : 2 }
  const version = { quote_node_id: quoteId, version: 1, recipient_contact_node_id: contactId, currency: 'EUR', title: 'Website work', terms_markdown: 'Pay in 30 days', lines: [{ position: 0, description: 'Design', cost_unit_node_id: costId, unit: 'hour', quantity: 3, rate_amount: 19.99, net_amount: 59.97, tax_rate: 0.2 }], subtotal: 59.97, tax_total: 11.994, total: 71.964, content_sha256: digest }
  const calls: { path: string; method: string; body: string | null }[] = []
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname, method = route.request().method()
    calls.push({ path, method, body: route.request().postData() })
    if (path === '/api/me') return route.fulfill({ json: { principal: { id: 'customer', name: 'Customer', kind: 'person', roles: customer ? ['customer'] : ['admin'] }, tenant: { id: 'tenant', name: 'Business' } } })
    if (path === '/api/version') return route.fulfill({ json: { version: '260923180522.0.0', scheme: 'inspr-calendar-v2' } })
    if (path === '/api/quotes' && method === 'GET') return route.fulfill({ json: [quote] })
    if (path === `/api/quotes/${quoteId}/versions` && method === 'GET') return route.fulfill({ json: [version] })
    if (path === `/api/quotes/${quoteId}/versions` && method === 'POST') { quote.current_version++; quote.revision++; return route.fulfill({ status: 201, json: { ...version, version: 2 } }) }
    if (path.endsWith('/issue')) { quote.state = 'issued'; quote.revision++; return route.fulfill({ json: quote }) }
    if (path.endsWith('/accept')) { quote.state = 'accepted'; quote.revision++; return route.fulfill({ status: 201, json: { accepted_content_sha256: digest } }) }
    return route.fulfill({ status: 404, json: { error: 'Uncontracted route' } })
  })
  await page.goto('/')
  await page.evaluate(async () => {
    const { router } = await import('/src/router.ts')
    const { default: view } = await import('/src/views/business/QuotesView.vue')
    router.addRoute({ path: '/business/quotes', component: view })
    await router.push('/business/quotes')
  })
  await expect(page.getByRole('heading', { name: 'Quotes' })).toBeVisible()
  return { calls, quote }
}

test('administrator freezes exact decimals and issues the current version', async ({ page }) => {
  const { calls } = await setup(page)
  await page.getByRole('button', { name: new RegExp(quoteId) }).click()
  await expect(page.getByLabel('Quote details')).toContainText('71.964 EUR')
  await page.getByRole('button', { name: 'Issue' }).click()
  await expect(page.getByText('Quote issued.')).toBeVisible()
  await page.getByLabel('Title').last().fill('Second offer')
  await page.getByLabel('Recipient contact ID').fill(contactId)
  await page.getByLabel('Currency').fill('EUR')
  await page.getByLabel('Description').fill('Design')
  await page.getByLabel('Cost unit ID').fill(costId)
  await page.getByLabel('Quantity').fill('0.1000')
  await page.getByLabel('Tax rate (0–1)').fill('0.20000')
  await page.getByRole('button', { name: 'Add line' }).click()
  await page.getByLabel('Description').last().fill('Review')
  await page.getByLabel('Cost unit ID').last().fill(costId)
  await page.getByLabel('Quantity').last().fill('2')
  await page.getByLabel('Tax rate (0–1)').last().fill('0')
  await page.getByRole('button', { name: 'Freeze version' }).click()
  await expect(page.getByText('Frozen version created.')).toBeVisible()
  const write = calls.find(c => c.method === 'POST' && c.path.endsWith('/versions'))
  expect(write?.body).toContain('"quantity":0.1000')
  expect(write?.body).toContain('"tax_rate":0.20000')
  expect(write?.body).toContain('"description":"Review"')
})

test('customer accepts only by sending the displayed frozen digest', async ({ page }) => {
  const { calls } = await setup(page, true)
  await page.getByRole('button', { name: new RegExp(quoteId) }).click()
  await expect(page.getByLabel('Quote details')).toContainText('Status: issued')
  await expect(page.getByLabel('Quote details')).toContainText(`SHA-256 ${digest}`)
  await expect(page.getByRole('button', { name: 'Freeze version' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Accept this exact offer' }).click()
  await expect(page.getByText('Acceptance recorded.')).toBeVisible()
  expect(JSON.parse(calls.find(c => c.path.endsWith('/accept'))!.body!)).toEqual({ expected_content_sha256: digest })
})
