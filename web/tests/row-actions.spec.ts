// SPDX-License-Identifier: AGPL-3.0-only
// QL1 (AEON-109): row actions on the Quotes and Customers lists. Inline on hover
// and focus (duplicate, PDF, …); the … menu offers every action the quote's flow
// state allows, opens on right-click, the context-menu key and Shift+F10, walks
// with the arrow keys, explains what is unavailable, and undoes what removes
// through the event log. Axe in light and dark; nothing cut at 390.
import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'
import { crmData, mockCRM, HOFER, NORDSTERN } from './crm-fixtures'
import { mockQuotes, quoteWorld, Q } from './quote-list-fixtures'
import { fixtures, mockWork, watchErrors } from './work-fixtures'

async function setup(page: Page, options: { role?: 'admin' | 'member' } = {}) {
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await mockWork(page, fixtures())
  const crm = crmData()
  await mockCRM(page, crm, { role: options.role })
  const world = quoteWorld()
  const calls = await mockQuotes(page, world, { role: options.role })
  return { world, calls, crm }
}
async function openQuotes(page: Page) {
  await page.goto('/business/quotes')
  await expect(page.getByRole('grid', { name: 'Quotes' })).toBeVisible()
  await expect(page.locator('.quotes tbody .row').first()).toBeVisible()
}
async function openCustomers(page: Page) {
  await page.goto('/business/customers')
  await expect(page.getByRole('grid', { name: 'Customers' })).toBeVisible()
  await expect(page.locator('.customers tbody .row').first()).toBeVisible()
}
const quoteRow = (page: Page, id: string) => page.locator(`#quote-${id}`)
const menu = (page: Page) => page.getByRole('menu')
const items = (page: Page) => menu(page).getByRole('menuitem')
const toast = (page: Page) => page.locator('.toast').last()
const clipboard = (page: Page) => page.evaluate(() => navigator.clipboard.readText())
async function axe(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).exclude('.calendar-version').analyze()
  const summary = results.violations.map(v => `${v.id} (${v.impact}): ${v.help}\n${v.nodes.slice(0, 4).map(n => `    ${n.target.join(' ')} — ${n.failureSummary?.split('\n').slice(1, 2).join(' ').trim()}`).join('\n')}`)
  expect(summary, summary.join('\n')).toEqual([])
}
async function labels(page: Page) {
  return (await items(page).allInnerTexts()).map(t => t.replace(/\s+/g, ' ').trim())
}

test('hover shows duplicate, PDF and …; a duplicate can be opened or undone', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const errors = watchErrors(page)
  const { world, calls } = await setup(page)
  await openQuotes(page)
  const row = quoteRow(page, Q.issued)
  const duplicate = row.getByRole('button', { name: /^Duplicate / })
  await expect(duplicate).toBeHidden()
  await row.hover()
  await expect(duplicate).toBeVisible()
  await expect(row.getByRole('button', { name: /^Print .* PDF$/ })).toBeVisible()
  await expect(row.getByRole('button', { name: /^Actions for / })).toBeVisible()

  await duplicate.click()
  await expect(toast(page)).toContainText(/^Duplicated A\d+-11 as A\d+-21\./)
  await expect(page.locator('.quotes tbody .row')).toHaveCount(8)
  const copy = world.rows.at(-1)!.quote_node_id
  await expect(quoteRow(page, copy)).toHaveAttribute('aria-selected', 'true')
  await expect(toast(page).getByRole('button', { name: 'Open' })).toBeVisible()
  await toast(page).getByRole('button', { name: 'Undo' }).click()
  await expect(page.locator('.quotes tbody .row')).toHaveCount(7)
  await expect(quoteRow(page, copy)).toHaveCount(0)
  expect(calls.some(c => c.path.startsWith('/api/events/') && c.path.endsWith('/undo'))).toBe(true)
  expect(errors).toEqual([])
})

test('the menu of a never-issued draft offers everything; deleting it can be undone', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await setup(page)
  await openQuotes(page)
  const row = quoteRow(page, Q.draft)
  await row.hover()
  const more = row.getByRole('button', { name: /^Actions for / })
  await more.click()
  await expect(more).toHaveAttribute('aria-expanded', 'true')
  expect(await labels(page)).toEqual([
    'Open beside the list Enter', 'Open on its own page Shift Enter', 'Copy quote number', 'Duplicate as a new draft', 'Print or save as PDF', 'Issue…', 'Archive', 'Delete draft',
  ])
  await expect(items(page).first()).toBeFocused()
  // Arrow keys, Home and End walk the menu; a letter jumps.
  await page.keyboard.press('ArrowDown')
  await expect(items(page).nth(1)).toBeFocused()
  await page.keyboard.press('End')
  await expect(items(page).last()).toBeFocused()
  await page.keyboard.press('ArrowDown')
  await expect(items(page).first()).toBeFocused()
  await page.keyboard.press('ArrowUp')
  await expect(items(page).last()).toBeFocused()
  await page.keyboard.press('Home')
  await page.keyboard.press('a')
  await expect(items(page).filter({ hasText: 'Archive' })).toBeFocused()

  await items(page).filter({ hasText: 'Delete draft' }).click()
  await expect(menu(page)).toHaveCount(0)
  await expect(row).toHaveCount(0)
  expect(calls.find(c => c.method === 'DELETE')?.query.get('expected_revision')).toBe('2')
  await expect(toast(page)).toContainText(/^Deleted draft A\d+-12\./)
  await toast(page).getByRole('button', { name: 'Undo' }).click()
  await expect(row).toBeVisible()
  await expect(row).toHaveAttribute('aria-selected', 'true')
})

test('an issued quote is kept: archive instead of delete, revise, and its customer link', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await setup(page)
  await openQuotes(page)
  // Right-click opens the same menu where the pointer is.
  const row = quoteRow(page, Q.issued)
  const box = (await row.locator('td.c-customer').boundingBox())!
  await row.locator('td.c-customer').click({ button: 'right' })
  await expect(menu(page)).toBeVisible()
  const panel = (await page.locator('.floating').boundingBox())!
  expect(Math.abs(panel.x - (box.x + box.width / 2))).toBeLessThan(2)
  await expect(items(page).filter({ hasText: 'Create customer link' })).toBeVisible()
  expect(await labels(page)).not.toContain('Delete draft')
  expect(await labels(page)).toContain('Revise as version 2…')
  expect(await labels(page)).toContain('Archive')

  await items(page).filter({ hasText: 'Create customer link' }).click()
  await expect(toast(page)).toContainText(/^Created a customer link for A\d+-11, open until .+, and copied it\./)
  expect(await clipboard(page)).toMatch(/\/offers\/sel-demo\/tok-link-\d+$/)
  expect(calls.some(c => c.method === 'POST' && c.path.endsWith('/versions/1/public-link'))).toBe(true)

  // Now the menu copies it.
  await row.locator('td.c-customer').click({ button: 'right' })
  await expect(items(page).filter({ hasText: 'Copy customer link' })).toBeVisible()
  await items(page).filter({ hasText: 'Copy customer link' }).click()
  await expect(toast(page)).toContainText(/^Copied the customer link of A\d+-11\./)

  // Archive: gone from the list, back with Undo.
  await row.locator('td.c-customer').click({ button: 'right' })
  await items(page).filter({ hasText: 'Archive' }).click()
  await expect(row).toHaveCount(0)
  await expect(toast(page)).toContainText(/^Archived A\d+-11\. It keeps its versions, evidence and files\./)
  await toast(page).getByRole('button', { name: 'Undo' }).click()
  await expect(row).toBeVisible()
})

test('Shift+F10 and the context-menu key open the cursor row’s menu; Escape hands focus back', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await setup(page)
  await openQuotes(page)
  const grid = page.getByRole('grid', { name: 'Quotes' })
  await grid.focus()
  await page.keyboard.press('j')
  await expect(quoteRow(page, Q.draft)).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('Shift+F10')
  await expect(menu(page)).toHaveAccessibleName(/^Actions for A\d+-12$/)
  // Anchored at the row's … button, ending at its right edge.
  const more = (await quoteRow(page, Q.draft).locator('.row-more').boundingBox())!
  const panel = (await page.locator('.floating').boundingBox())!
  expect(Math.abs(panel.x + panel.width - (more.x + more.width))).toBeLessThan(2)
  await page.keyboard.press('Escape')
  await expect(menu(page)).toHaveCount(0)
  await expect(grid).toBeFocused()
  await page.keyboard.press('ContextMenu')
  await expect(menu(page)).toBeVisible()
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(new RegExp(`/business/quotes\\?quote=${Q.draft}$`))
})

test('unavailable actions stay visible with their reason: a revising draft, a member', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { world, calls } = await setup(page, { role: 'member' })
  // The second draft is being revised as version 2 of an issued quote.
  Object.assign(world.rows.find(r => r.quote_node_id === Q.second)!, { current_version: 1 })
  await openQuotes(page)
  await quoteRow(page, Q.second).click({ button: 'right' })
  const issue = items(page).filter({ hasText: 'Issue as version 2…' })
  await expect(issue).toHaveAttribute('aria-disabled', 'true')
  await expect(issue).toHaveAttribute('data-tip', 'Only a workspace admin issues quotes.')
  await expect(issue).toHaveAccessibleDescription('Only a workspace admin issues quotes.')
  // The tooltip says why beside the menu, never over its other entries.
  await issue.hover()
  await expect(page.locator('.tooltip')).toHaveText('Only a workspace admin issues quotes.')
  const tip = (await page.locator('.tooltip').boundingBox())!, box = (await page.locator('.floating').boundingBox())!
  expect(tip.x).toBeGreaterThanOrEqual(box.x + box.width)
  expect(await labels(page)).not.toContain('Archive')
  expect(await labels(page)).not.toContain('Delete draft')
  // Pressing it does nothing but say why, under the entry.
  await issue.click({ force: true })
  await expect(menu(page)).toBeVisible()
  await expect(issue.locator('.row-menu-reason')).toBeVisible()
  expect(calls.some(c => c.path.endsWith('/finalize'))).toBe(false)
  await page.keyboard.press('Escape')

  // An admin sees delete on the revising draft as unavailable, with the reason.
  await page.unrouteAll({ behavior: 'ignoreErrors' })
  const again = await setup(page)
  Object.assign(again.world.rows.find(r => r.quote_node_id === Q.second)!, { current_version: 1 })
  await openQuotes(page)
  await quoteRow(page, Q.second).click({ button: 'right' })
  const remove = items(page).filter({ hasText: 'Delete draft' })
  await expect(remove).toHaveAttribute('aria-disabled', 'true')
  await expect(remove).toHaveAccessibleDescription('Version 1 was issued and is kept. Archive the quote instead.')
})

test('issuing and revising from the menu ask first and use the server’s guards', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await setup(page)
  await openQuotes(page)
  await quoteRow(page, Q.draft).click({ button: 'right' })
  await items(page).filter({ hasText: 'Issue…' }).click()
  const confirm = page.getByRole('dialog', { name: /^Issue A\d+-12 as version 1\?$/ })
  await expect(confirm).toBeVisible()
  await confirm.getByRole('button', { name: 'Issue quote' }).click()
  await expect(toast(page)).toContainText(/^Issued A\d+-12 as version 1\./)
  expect(calls.find(c => c.path.endsWith('/finalize'))?.body).toMatchObject({ expected_quote_revision: 2, expected_draft_revision: 3 })
  await expect(quoteRow(page, Q.draft)).toContainText('Issued')

  await quoteRow(page, Q.accepted).click({ button: 'right' })
  await items(page).filter({ hasText: 'Revise as version 2…' }).click()
  await page.getByRole('dialog', { name: 'Revise as version 2?' }).getByRole('button', { name: 'Revise' }).click()
  await expect(page).toHaveURL(new RegExp(`/business/quotes/${Q.accepted}$`))
})

test('PDF from the list opens the quote on its page and prints it once', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await setup(page)
  await openQuotes(page)
  const row = quoteRow(page, Q.issued)
  await row.hover()
  await row.getByRole('button', { name: /^Print .* PDF$/ }).click()
  await expect(page).toHaveURL(new RegExp(`/business/quotes/${Q.issued}$`))
  await expect.poll(() => page.evaluate(() => (window as unknown as { printed: number }).printed)).toBe(1)
})

test('phones: every card has the … menu, inside the screen', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await setup(page)
  await page.goto('/business/quotes')
  const cards = page.getByRole('list', { name: 'Quotes' }).getByRole('listitem')
  await expect(cards).toHaveCount(7)
  const more = cards.nth(1).getByRole('button', { name: /^Actions for / })
  const size = (await more.boundingBox())!
  expect(size.width).toBeGreaterThanOrEqual(36)
  await more.click()
  await expect(menu(page)).toBeVisible()
  // A phone opens a quote on its own page; there is no room beside the list.
  await expect(items(page).first()).toHaveText('Open')
  const panel = (await page.locator('.floating').boundingBox())!
  expect(panel.x).toBeGreaterThanOrEqual(8)
  expect(panel.x + panel.width).toBeLessThanOrEqual(390 - 8 + 0.5)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await items(page).filter({ hasText: 'Delete draft' }).click()
  await expect(cards).toHaveCount(6)
  await toast(page).getByRole('button', { name: 'Undo' }).click()
  await expect(cards).toHaveCount(7)
})

test('customers: a new quote inline, and a menu to open, copy the number or archive', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const errors = watchErrors(page)
  const { crm } = await setup(page)
  await openCustomers(page)
  const row = page.locator(`#customer-${HOFER}`)
  await row.hover()
  await row.getByRole('button', { name: 'New quote for Bäckerei Hofer' }).click()
  const dialog = page.getByRole('dialog', { name: 'New quote' })
  await expect(dialog).toBeVisible()
  await expect(dialog.locator('.fixed-customer')).toContainText('Bäckerei Hofer')
  await dialog.getByRole('button', { name: 'Cancel' }).click()

  await row.getByRole('button', { name: 'Actions for Bäckerei Hofer' }).click()
  expect(await labels(page)).toEqual(['Open Enter', 'New quote for this customer…', 'Copy customer number', 'Archive'])
  await items(page).filter({ hasText: 'Copy customer number' }).click()
  expect(await clipboard(page)).toBe('K-0042')

  await row.click({ button: 'right' })
  await items(page).filter({ hasText: 'Archive' }).click()
  await expect(row).toHaveCount(0)
  await expect(toast(page)).toContainText('Archived Bäckerei Hofer.')
  const toggle = page.getByRole('button', { name: /^Archived/ })
  await expect(toggle).toContainText('1')
  await toggle.click()
  await expect(row).toBeVisible()
  await expect(row.locator('.archived-chip')).toHaveText('Archived')
  await row.click({ button: 'right' })
  await expect(items(page).filter({ hasText: 'New quote for this customer…' })).toHaveAttribute('aria-disabled', 'true')
  await page.keyboard.press('Escape')
  await toast(page).getByRole('button', { name: 'Undo' }).click()
  await expect(row.locator('.archived-chip')).toHaveCount(0)
  expect(crm.customers.find(c => c.id === HOFER)?.archived).toBe(false)

  // Keyboard: Shift+F10 on the cursor row.
  await page.getByRole('grid', { name: 'Customers' }).focus()
  await page.keyboard.press('j')
  await page.keyboard.press('Shift+F10')
  await expect(menu(page)).toHaveAccessibleName(/^Actions for /)
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/\/business\/customers\/org-/)
  expect(errors).toEqual([])
})

test('customers on phones: the … menu on every card, nothing cut', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await setup(page)
  await page.goto('/business/customers')
  const card = page.locator(`#customer-${NORDSTERN}`)
  await card.getByRole('button', { name: 'Actions for Klinik Nordstern' }).click()
  await expect(menu(page)).toBeVisible()
  const panel = (await page.locator('.floating').boundingBox())!
  expect(panel.x + panel.width).toBeLessThanOrEqual(390 - 8 + 0.5)
  await items(page).filter({ hasText: 'Archive' }).click()
  await expect(card).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

for (const colorScheme of ['light', 'dark'] as const) {
  test(`axe: row actions and menus in ${colorScheme}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme })
    await page.setViewportSize({ width: 1440, height: 900 })
    const { world } = await setup(page)
    Object.assign(world.rows.find(r => r.quote_node_id === Q.second)!, { current_version: 1 })
    await openQuotes(page)
    await quoteRow(page, Q.draft).hover()
    await axe(page)
    await quoteRow(page, Q.second).click({ button: 'right' })
    await expect(items(page).filter({ hasText: 'Delete draft' })).toHaveAttribute('aria-disabled', 'true')
    await axe(page)
    await page.keyboard.press('Escape')
    await quoteRow(page, Q.issued).click({ button: 'right' })
    await expect(items(page).filter({ hasText: /customer link/ })).not.toHaveAttribute('aria-disabled', 'true')
    await axe(page)
    await page.keyboard.press('Escape')
    await openCustomers(page)
    await page.locator(`#customer-${HOFER}`).hover()
    await axe(page)
    await page.locator(`#customer-${HOFER}`).click({ button: 'right' })
    await axe(page)
  })
}
