// SPDX-License-Identifier: AGPL-3.0-only
// A session that ends during an edit removes the protected page and preserves
// its address so the person can return after signing in.
import { test, expect } from '@playwright/test'
import { fixtures, mockWork } from './work-fixtures'

const mod = process.platform === 'darwin' ? 'Meta' : 'Control'

for (const width of [1440, 390]) {
  test(`at ${width}px a save after the session ended goes to sign-in with the ticket address`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    await mockWork(page, fixtures())
    await page.goto('/p/PHAROS/PHAROS-12')
    const ws = page.getByRole('complementary', { name: 'Ticket details' })
    await ws.getByRole('button', { name: 'Add a description' }).click()
    const area = ws.getByLabel('Description, Markdown')
    await area.fill('Written while the session ends.')
    await expect(area).toHaveValue('Written while the session ends.')
    // The session ends: writes answer 401 from now on.
    await page.route('**/api/nodes/*', route => route.request().method() === 'PATCH' ? route.fulfill({ status: 401, json: { error: 'unauthorized' } }) : route.fallback())
    await page.keyboard.press(`${mod}+Enter`)
    await expect(page).toHaveURL(/\/signin\?error=expired&return=\/p\/PHAROS\/PHAROS-12/)
    await expect(page.getByRole('heading', { name: 'Sign in', level: 1 })).toBeVisible()
    await expect(page.getByText(/unauthorized/)).toHaveCount(0)
    await expect(ws).toHaveCount(0)
  })
}
