// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test } from '@playwright/test'

test('page title is PAIMOS AEON', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveTitle(/PAIMOS AEON/)
})

test('GET /api/health returns status ok', async ({ request }) => {
  const response = await request.get('/api/health')
  expect(response.ok()).toBeTruthy()
  const body = (await response.json()) as { status: string }
  expect(body.status).toBe('ok')
})
