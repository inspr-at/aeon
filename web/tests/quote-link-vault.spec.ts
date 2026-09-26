// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test, type Page } from '@playwright/test'

const id = '11111111-1111-4111-8111-111111111111'
const link = { id: '22222222-2222-4222-8222-222222222222', public_tenant: 'opaque-selector', quote_node_id: id, version: 1, target_content_sha256: 'a'.repeat(64), expires_at: '2030-01-01T00:00:00Z', copy_unavailable_reason: 'key_not_configured' }

async function mount(page: Page) {
  await page.route('**/api/**', route => {
    if (route.request().url().includes(`/api/quotes/${id}/versions/1/public-link`)) return route.fallback()
    const path = new URL(route.request().url()).pathname
    return route.fulfill({ json: path === '/api/me'
      ? { principal: { id: id, name: 'Test Person' }, tenant: { id: 'test', name: 'Test Workspace' } }
      : path === '/api/approvals' ? []
        : ['/api/projects', '/api/nodes', '/api/project-groups', '/api/harness-sessions', '/api/plugins'].includes(path) ? { items: [] } : {} })
  })
  await page.goto('/')
  await page.evaluate(async () => {
    const harness = await import(/* @vite-ignore */ '/tests/quote-link-card-harness.ts')
    harness.mountQuoteLinkCard()
  })
}

test('no-key link explains why re-copy and QR are unavailable', async ({ page }) => {
  await page.route(`**/api/quotes/${id}/versions/1/public-link`, route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(link) }))
  await mount(page)
  await expect(page.getByText('This server has no customer-link key. This link cannot be copied or shown as a QR code again.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'QR code' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Revoke link' })).toBeVisible()
})

test('new one-time link asks admin to save it while QR remains available', async ({ page }) => {
  await page.route(`**/api/quotes/${id}/versions/1/public-link`, route => route.fulfill({ status: route.request().method() === 'POST' ? 201 : 404, contentType: 'application/json', body: route.request().method() === 'POST' ? JSON.stringify({ ...link, path: '/offers/opaque-selector/' + 'A'.repeat(43), token: 'A'.repeat(43) }) : '{}' }))
  await mount(page)
  await page.getByRole('button', { name: 'Create link' }).click()
  await expect(page.getByText('Copy or save this link now.', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: 'QR code' }).click()
  await expect(page.getByRole('img', { name: 'QR code of the customer link' })).toBeVisible()
})
