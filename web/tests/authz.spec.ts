// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect } from '@playwright/test'
import { fixtures, me, mockWork } from './work-fixtures'

const ticket = '/p/PHAROS/PHAROS-12'
const details = (page: import('@playwright/test').Page) => page.getByRole('complementary', { name: 'Ticket details' })

test('an admin label does not expose an action without its permission', async ({ page }) => {
  await mockWork(page, fixtures(), { readOnly: true })
  await page.route('**/api/me', route => route.fulfill({ json: { principal: { id: me.id, name: me.name, kind: 'person', roles: ['admin'] }, tenant: { id: 't1', name: 'INSPR Studio' } } }))
  await page.goto(ticket)
  await expect(details(page).getByRole('heading', { name: 'Add an Oracle Cloud connector' })).toBeVisible()
  await expect(details(page).getByRole('button', { name: /^(?:Edit|Add a) description$/ })).toHaveCount(0)
  await expect(details(page).getByLabel('Add a comment')).toBeDisabled()
})

test('a deprecated viewer label does not suppress a granted action', async ({ page }) => {
  await mockWork(page, fixtures())
  await page.route('**/api/me', route => route.fulfill({ json: { principal: { id: me.id, name: me.name, kind: 'person', roles: ['viewer'] }, tenant: { id: 't1', name: 'INSPR Studio' } } }))
  await page.goto(ticket)
  await expect(details(page).getByRole('heading', { name: 'Add an Oracle Cloud connector' })).toBeVisible()
  await expect(details(page).getByRole('button', { name: /^(?:Edit|Add a) description$/ })).toBeVisible()
})
