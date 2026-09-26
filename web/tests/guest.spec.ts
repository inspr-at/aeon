// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect } from '@playwright/test'
import { mockGuestPermissions } from './authz-fixtures'
import { fixtures, me, mockWork, watchErrors } from './work-fixtures'

const ticket = '/p/PHAROS/PHAROS-12'
const details = (page: import('@playwright/test').Page) => page.getByRole('complementary', { name: 'Ticket details' })

// ADR-003 P2: a guest bound to one project has no workspace permissions. The
// server already filters what it sees; the app must still load, read the
// project and let the guest comment, and treat workspace areas that answer
// 403 (members, project groups, plugins, people) as simply not there.
async function asGuest(page: import('@playwright/test').Page) {
  await mockWork(page, fixtures(), { readOnly: true })
  const forbidden = { status: 403, json: { error: 'permission denied', code: 'forbidden', reason: 'This action needs a permission you do not hold' } }
  for (const path of ['**/api/members*', '**/api/project-groups*', '**/api/plugins*', '**/api/business/principals*', '**/api/agent-keys*', '**/api/roles*']) {
    await page.route(path, route => route.fulfill(forbidden))
  }
  await page.route('**/api/me/permissions*', route => {
    const projectId = new URL(route.request().url()).searchParams.get('project_id') ?? undefined
    return route.fulfill({ json: mockGuestPermissions(projectId, ['p-pharos']) })
  })
  await page.route('**/api/me', route => route.fulfill({ json: { principal: { id: me.id, name: me.name, kind: 'person', roles: [] }, tenant: { id: 't1', name: 'INSPR Studio' } } }))
}

test('a project guest reads the project and may comment, nothing more', async ({ page }) => {
  const errors = watchErrors(page)
  await asGuest(page)
  await page.goto(ticket)
  await expect(details(page).getByRole('heading', { name: 'Add an Oracle Cloud connector' })).toBeVisible()
  await expect(details(page).getByLabel('Add a comment')).toBeEnabled()
  await expect(details(page).getByRole('button', { name: /^(?:Edit|Add a) description$/ })).toHaveCount(0)
  expect(errors).toEqual([])
})

test('a project guest opens the workspace without errors', async ({ page }) => {
  const errors = watchErrors(page)
  await asGuest(page)
  await page.goto('/')
  await expect(page.getByRole('link', { name: /Pharos/ }).first()).toBeVisible()
  await expect(page.getByText(/something went wrong/i)).toHaveCount(0)
  expect(errors).toEqual([])
})
