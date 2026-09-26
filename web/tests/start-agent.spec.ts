// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { mockStartAgent } from './start-agent-fixtures'

const dialog = (page: Page) => page.getByRole('dialog', { name: 'Start agent', exact: true })
async function open(page: Page, ticket = false) {
  await page.goto(ticket ? '/p/PHAROS/PHAROS-11' : '/agents')
  await page.getByRole('button', { name: 'Start agent', exact: true }).click()
  await expect(dialog(page)).toBeVisible()
}
async function choose(page: Page, ticket = false) {
  if (!ticket) {
    await dialog(page).getByRole('searchbox').fill('PHAROS-11')
    await dialog(page).getByRole('button', { name: /PHAROS-11 Connect Hetzner/ }).click()
  }
  await dialog(page).getByLabel('Agent', { exact: true }).selectOption({ label: 'Studio builder' })
  await dialog(page).getByLabel('Model profile').selectOption('model-enabled')
}

test('creates, readies and queues a ticket, then follows claim and managed registration', async ({ page }) => {
  const mock = await mockStartAgent(page)
  await open(page)
  await choose(page)
  await dialog(page).getByLabel('Account optional').selectOption(mock.account.id)
  await expect(dialog(page).getByRole('option', { name: /Retired model/ })).toHaveCount(0)
  await expect(dialog(page).getByText('Account available', { exact: true })).toBeVisible()
  await dialog(page).getByRole('button', { name: 'Queue run', exact: true }).dblclick()
  await expect(dialog(page).getByRole('heading', { name: 'Queued', exact: true })).toBeVisible()
  expect(mock.calls.filter(c => c.method === 'POST' && c.path === '/api/work-orders')).toHaveLength(1)
  expect(mock.calls.filter(c => c.method === 'POST' && c.path.endsWith('/runs'))).toHaveLength(1)
  expect(mock.calls.find(c => c.method === 'POST' && c.path.endsWith('/runs'))?.body).toEqual({ agent_principal_id: mock.agentId, model_profile_id: mock.profile.id, requested_account_id: mock.account.id })
  expect(mock.calls.find(c => c.method === 'POST' && c.path === '/api/work-orders')?.body?.parent_id).toBe('n-1')
  mock.claim()
  await dialog(page).getByRole('button', { name: 'Refresh status' }).click()
  await expect(dialog(page).getByRole('heading', { name: 'Claimed', exact: true })).toBeVisible()
  await expect(dialog(page).getByRole('link', { name: 'Open session' })).toHaveCount(0)
  mock.claim(true)
  await dialog(page).getByRole('button', { name: 'Refresh status' }).click()
  await expect(dialog(page).getByRole('heading', { name: 'Managed session connected' })).toBeVisible()
  await expect(dialog(page).getByRole('link', { name: 'Open session' })).toHaveAttribute('href', '/agents/managed-1')
})

test('ticket panel preselects its ticket and retry reuses the created work order', async ({ page }) => {
  const mock = await mockStartAgent(page, { failQueue: true })
  await open(page, true)
  await expect(dialog(page).getByText('PHAROS-11', { exact: true })).toBeVisible()
  await expect(dialog(page).getByRole('searchbox')).toHaveCount(0)
  await choose(page, true)
  await dialog(page).getByRole('button', { name: 'Queue run', exact: true }).click()
  await expect(dialog(page).getByRole('alert')).toContainText('Temporary queue failure')
  await expect(dialog(page).getByLabel('Agent', { exact: true })).toHaveValue(mock.agentId)
  mock.state.failQueue = false
  await dialog(page).getByRole('button', { name: 'Queue run', exact: true }).click()
  await expect(dialog(page).getByRole('heading', { name: 'Queued', exact: true })).toBeVisible()
  expect(mock.calls.filter(c => c.method === 'POST' && c.path === '/api/work-orders')).toHaveLength(1)
})

for (const scenario of [{ offline: true, label: 'No daemon online' }, { unavailable: true, label: 'No eligible account' }]) {
  test(`reports ${scenario.label} separately from queueing`, async ({ page }) => {
    await mockStartAgent(page, scenario)
    await open(page)
    await choose(page)
    await expect(dialog(page).getByText(scenario.label, { exact: true })).toBeVisible()
    await expect(dialog(page).getByRole('button', { name: 'Queue run', exact: true })).toBeEnabled()
    await dialog(page).getByRole('button', { name: 'Queue run', exact: true }).click()
    await expect(dialog(page).getByRole('heading', { name: 'Queued', exact: true })).toBeVisible()
    await dialog(page).getByRole('button', { name: 'Close', exact: true }).click()
    await expect(page.getByRole('region', { name: 'Runs awaiting a session' })).toContainText('Queued')
  })
}

test('failed prerequisites keep launch disabled and read-only people have no action', async ({ page }) => {
  await mockStartAgent(page, { forbidden: true })
  await open(page)
  await expect(dialog(page).getByRole('alert')).toContainText('Permission denied')
  await expect(dialog(page).getByRole('button', { name: 'Queue run', exact: true })).toBeDisabled()
  await page.unrouteAll({ behavior: 'wait' })
  await mockStartAgent(page, { readOnly: true })
  await page.goto('/agents')
  await expect(page.getByRole('heading', { name: 'Agents', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Start agent', exact: true })).toHaveCount(0)
})

for (const theme of ['light', 'dark']) {
  test(`keyboard, axe and phone layout in ${theme}`, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.emulateMedia({ colorScheme: theme as 'light' | 'dark' })
    await mockStartAgent(page)
    await open(page, true)
    await choose(page, true)
    await page.evaluate(theme => document.documentElement.dataset.theme = theme, theme)
    await expect(dialog(page).getByText('Account available', { exact: true })).toBeVisible()
    const result = await new AxeBuilder({ page }).include('.launch-dialog').withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'best-practice']).analyze()
    expect(result.violations).toEqual([])
    expect(await dialog(page).evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
    await page.keyboard.press('Escape')
    await expect(dialog(page)).not.toBeVisible()
    await expect(page.getByRole('button', { name: 'Start agent', exact: true })).toBeFocused()
  })
}
