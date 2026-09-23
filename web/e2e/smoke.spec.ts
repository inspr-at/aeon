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

test('sign-in renders every image and violates no content security policy', async ({ page }) => {
  const violations: string[] = []
  page.on('console', (m) => { if (/Content Security Policy|violates the following/i.test(m.text())) violations.push(m.text()) })
  await page.goto('/signin')
  await page.waitForLoadState('networkidle')
  const broken = await page.$$eval('img', (l) => l.filter((i) => !(i.complete && i.naturalWidth > 0)).map((i) => i.src.slice(0, 60)))
  expect(broken).toEqual([])
  expect(violations).toEqual([])
})
