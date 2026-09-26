// SPDX-License-Identifier: AGPL-3.0-only
// QA4 (AEON-140): a session that ends while someone edits keeps what they typed
// and offers sign-in in a new tab, instead of the bare word "unauthorized".
import { test, expect } from '@playwright/test'
import { fixtures, mockWork } from './work-fixtures'

const mod = process.platform === 'darwin' ? 'Meta' : 'Control'

for (const width of [1440, 390]) {
  test(`at ${width}px a save after the session ended keeps the draft and offers sign-in`, async ({ page, context }) => {
    await page.setViewportSize({ width, height: 900 })
    await mockWork(page, fixtures())
    await page.goto('/p/PHAROS/PHAROS-12')
    const ws = page.getByRole('complementary', { name: 'Ticket details' })
    await ws.getByRole('button', { name: 'Add a description' }).click()
    const area = ws.getByLabel('Description, Markdown')
    await area.fill('Written while the session ends.')
    // The session ends: writes answer 401 from now on.
    await page.route('**/api/nodes/*', route => route.request().method() === 'PATCH' ? route.fulfill({ status: 401, json: { error: 'unauthorized' } }) : route.fallback())
    await page.keyboard.press(`${mod}+Enter`)
    const recovery = page.getByText('Your session has ended. Sign in again in a new tab; what you typed stays on this page.')
    await expect(recovery).toBeVisible()
    await expect(page.getByText(/was not saved: your session has ended/)).toBeVisible()
    await expect(page.getByText(/unauthorized/)).toHaveCount(0)
    await expect(area).toHaveValue('Written while the session ends.')
    const opened = context.waitForEvent('page')
    await page.getByRole('button', { name: 'Sign in' }).click()
    expect(new URL((await opened).url()).pathname + new URL((await opened).url()).search).toBe('/signin?error=expired')
    await expect(area).toHaveValue('Written while the session ends.')
  })
}
