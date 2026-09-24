// SPDX-License-Identifier: AGPL-3.0-only
// A chosen theme is a server preference: it survives a reload instead of falling
// back to System (Markus, 2026-09-24: "it keeps going to system all the time").
import { test, expect } from '@playwright/test'
import { fixtures, mockWork } from './work-fixtures'

test('a chosen theme is saved and restored after a reload', async ({ page }) => {
  await mockWork(page, fixtures())
  let stored: unknown = null
  const writes: unknown[] = []
  // Registered after the shared mocks, so this handler wins for the theme key.
  await page.route('**/api/preferences/theme', async route => {
    if (route.request().method() === 'PUT') {
      stored = route.request().postDataJSON().value
      writes.push(stored)
      await route.fulfill({ json: { key: 'theme', value: stored } })
    } else {
      await route.fulfill({ json: { key: 'theme', value: stored } })
    }
  })
  await page.goto('/')
  await expect(page.locator('html')).not.toHaveAttribute('data-theme', /.+/)
  await page.getByRole('button', { name: 'Switch to dark theme' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect.poll(() => writes.length).toBe(1)
  expect(writes[0]).toEqual({ choice: 'dark' })
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
})
