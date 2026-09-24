// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test, type Page } from '@playwright/test'

const digest = 'a'.repeat(64)
const document = {
  schema_version: 1, minimum_writer_version: 1, title: 'Synthetic quote', subtitle: 'A neutral example', project_ref: 'TEST-1',
  offer_date: '2026-09-24', valid_until: '2026-10-24', currency: 'EUR',
  sender: { company: 'Example Studio', street: 'Sample Lane 1', postal_code: '1000', city: 'Test City', country: 'AT' },
  recipient: { name: 'Sample Customer', address: 'Demo Street 2' },
  legal: { intro: 'A test introduction.', accept_text: 'I accept this quote.', vat_note: 'Tax according to agreed terms.' },
  layout: {}, sections: [{ id: '11111111-1111-4111-8111-111111111111', heading: 'Scope', body: 'Work', nodes: [{ id: '22222222-2222-4222-8222-222222222222', kind: 'paragraph', text: 'Work' }] }],
  positions: [{ id: '33333333-3333-4333-8333-333333333333', pricing_source: 'manual', short_text: 'Service', long_text: 'Synthetic service', quantity: '1.00', unit_label: 'item', unit_price_cents: 100, total_cents: 100, currency: 'EUR' }], net_total_cents: 100,
}
async function mount(page: Page, acceptable: boolean) {
  await page.route('**/api/**', route => {
    if (route.request().url().endsWith('/accept')) return route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ version: 1, content_sha256: digest, accepted_at: '2026-09-24T12:00:00Z', confirmation_state: 'pending' }) })
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ document, offer_no: 'A260924-1', version: 1, content_sha256: digest, state: acceptable ? 'issued' : 'issued', expires_at: '2026-10-24T00:00:00Z', acceptable, receipt_ready: false }) })
  })
  await page.goto('/')
  await page.evaluate(async () => {
    const harness = await import(/* @vite-ignore */ '/tests/quote-public-harness.ts')
    harness.mountPublicQuote()
  })
}

test('scoped document renders read-only and acceptance submits displayed digest', async ({ page }) => {
  let accepted: unknown
  await mount(page, true)
  await page.route('**/api/public/quotes/*/*/accept', route => { accepted = route.request().postDataJSON(); return route.fulfill({ status: 201, contentType: 'application/json', body: '{}' }) })
  await expect(page.getByRole('heading', { name: 'Synthetic quote' }).first()).toBeVisible()
  await expect(page.getByRole('region', { name: 'Quote document' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Accept quote' })).toBeDisabled()
  await page.getByLabel('Your name').fill('Customer Example')
  await page.getByLabel('I have reviewed this quote and agree to accept it.').check()
  await page.getByRole('button', { name: 'Accept quote' }).click()
  await expect.poll(() => accepted).toBeTruthy()
  expect(accepted).toMatchObject({ version: 1, expected_content_sha256: digest, name: 'Customer Example', confirm: true })
})

test('expired but readable document has no acceptance form', async ({ page }) => {
  await mount(page, false)
  await expect(page.getByRole('heading', { name: 'Synthetic quote' }).first()).toBeVisible()
  await expect(page.getByText('Acceptance is closed.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Accept quote' })).toHaveCount(0)
})
