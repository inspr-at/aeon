// SPDX-License-Identifier: AGPL-3.0-only
// U18 (AEON-99): the Quotes tab. The list with search, status, customer, date
// and amount filters, sorting, resizable columns and the keyboard; creating a
// quote from the list and from a customer's page; honest empty, error and
// closed states; axe in light and dark; nothing cut at 390.
import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'
import { crmData, mockCRM, HOFER } from './crm-fixtures'
import { mockQuotes, quoteWorld, Q, type QuoteMockOptions } from './quote-list-fixtures'
import { fixtures, mockWork, watchErrors } from './work-fixtures'

async function setup(page: Page, options: QuoteMockOptions & { empty?: boolean; role?: 'admin' | 'member'; quoteRevision?: number; enabled?: string[] } = {}) {
  const work = fixtures()
  const workCalls = await mockWork(page, work)
  const crm = crmData({ quoteRevision: options.quoteRevision, enabled: options.enabled })
  await mockCRM(page, crm, { role: options.role, quoteRevision: options.quoteRevision })
  const world = quoteWorld({ empty: options.empty })
  const calls = await mockQuotes(page, world, options)
  return { world, calls, crm, work, workCalls }
}
async function openList(page: Page) {
  await page.goto('/business/quotes')
  await expect(page.getByRole('grid', { name: 'Quotes' })).toBeVisible()
  await expect(page.locator('.quotes tbody .row').first()).toBeVisible()
}
const rows = (page: Page) => page.locator('.quotes tbody .row')
const toolbar = (page: Page) => page.getByRole('search')
async function axe(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).exclude('.calendar-version').analyze()
  const summary = results.violations.map(v => `${v.id} (${v.impact}): ${v.help}\n${v.nodes.slice(0, 4).map(n => `    ${n.target.join(' ')} — ${n.failureSummary?.split('\n').slice(1, 2).join(' ').trim()}`).join('\n')}`)
  expect(summary, summary.join('\n')).toEqual([])
}

test('the list shows every live quote with number, title, customer, status, dates and amount', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const errors = watchErrors(page)
  await setup(page)
  await openList(page)
  // Archived quotes stay out until asked for; the newest number comes first.
  await expect(rows(page)).toHaveCount(7)
  const first = rows(page).first()
  await expect(first).toContainText('Onlineshop Erweiterung Weihnachten')
  await expect(first).toContainText('Bäckerei Hofer')
  await expect(first).toContainText('Draft')
  await expect(first).toContainText('880.00 EUR')
  const expired = page.locator(`#quote-${Q.expired}`)
  await expect(expired).toContainText('Expired')
  await expect(page.locator(`#quote-${Q.lumen} .c-amount`)).toHaveText('—')
  await expect(page.locator('.summary')).toContainText('7 quotes')
  await expect(page.locator('.summary')).toContainText('2 waiting for the customer')
  await expect(page.locator('.summary')).toContainText('87,140.00 EUR to accept')
  expect(errors).toEqual([])
})

test('search, status, customer, date and amount narrow the list; archived joins on request', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await setup(page)
  await openList(page)
  const search = page.getByRole('searchbox', { name: 'Find a quote' })
  await search.fill('kasse')
  await expect(rows(page)).toHaveCount(1)
  await expect(rows(page).first().locator('mark')).toHaveText('Kasse')
  await search.fill('')

  await toolbar(page).getByRole('button', { name: 'Status' }).click()
  await page.getByRole('dialog', { name: 'Filter by status' }).getByLabel('Issued').check()
  await page.keyboard.press('Escape')
  await expect(rows(page)).toHaveCount(2)
  await toolbar(page).getByRole('button', { name: /^Customer/ }).click()
  await page.getByRole('dialog', { name: 'Filter by customer' }).getByLabel('Klinik Nordstern').check()
  await page.keyboard.press('Escape')
  await expect(rows(page)).toHaveCount(1)
  await expect(page.locator('.count')).toHaveText('1 of 7')
  await page.getByRole('button', { name: 'Clear', exact: true }).click()
  await expect(rows(page)).toHaveCount(7)

  // Date: a preset, then a range of your own.
  await toolbar(page).getByRole('button', { name: /^Date/ }).click()
  const dates = page.getByRole('dialog', { name: 'Filter by quote date' })
  await dates.getByLabel('Last 30 days').check()
  await expect(page.getByRole('button', { name: /^Last 30 days/ })).toBeVisible()
  await expect(rows(page)).toHaveCount(4)
  await dates.getByLabel('Between…').check()
  await expect(dates.getByLabel('From')).toBeVisible()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Clear', exact: true }).click()

  // Amount: bounds typed as people write them; a wrong one is named, not guessed.
  await toolbar(page).getByRole('button', { name: 'Amount' }).click()
  const amount = page.getByRole('dialog', { name: 'Filter by net amount' })
  await amount.getByLabel('At least').fill('5,000')
  await expect(rows(page)).toHaveCount(4)
  await amount.getByLabel('At most').fill('12.000,50')
  await expect(amount.getByRole('alert')).toHaveText('Type an amount such as 1500 or 1,500.50.')
  await amount.getByLabel('At most').fill('13000')
  await expect(rows(page)).toHaveCount(3)
  await page.keyboard.press('Escape')
  await expect(toolbar(page).getByRole('button', { name: /^5,000\.00\s–\s13,000\.00/ })).toBeVisible()
  await page.getByRole('button', { name: 'Clear', exact: true }).click()

  await page.getByRole('button', { name: /^Archived/ }).click()
  await expect(rows(page)).toHaveCount(8)
  await expect(page.locator(`#quote-${Q.archived}`)).toContainText('Archived')
})

test('headers sort (numbers newest first, words A to Z) and remember it', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { workCalls } = await setup(page)
  await openList(page)
  await page.locator('thead').getByRole('button', { name: 'Net amount' }).click()
  await expect(rows(page).first()).toContainText('74,300.00 EUR')
  await expect(page.locator('th.c-amount')).toHaveAttribute('aria-sort', 'descending')
  await page.locator('thead').getByRole('button', { name: 'Customer', exact: true }).click()
  await expect(rows(page).first()).toContainText('Atelier Lumen')
  await expect(page.locator('th.c-customer')).toHaveAttribute('aria-sort', 'ascending')
  await expect.poll(() => workCalls.some(c => c.method === 'PUT' && c.path === '/api/preferences/quotes' && JSON.stringify(c.body).includes('"customer"'))).toBe(true)
})

test('every column edge resizes by drag and by keyboard; the title takes the rest', async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 900 })
  const { workCalls } = await setup(page)
  await openList(page)
  const handles = page.getByRole('separator', { name: /^Resize .* column$/ })
  const count = await handles.count()
  expect(count).toBe(6)
  for (let i = 0; i < count; i++) {
    const handle = handles.nth(i)
    const name = await handle.getAttribute('aria-label')
    const before = Number(await handle.getAttribute('aria-valuenow'))
    const box = (await handle.boundingBox())!
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
    await page.mouse.down()
    await page.mouse.move(box.x + box.width / 2 + 30, box.y + box.height / 2, { steps: 4 })
    await page.mouse.up()
    await expect.poll(async () => Number(await handle.getAttribute('aria-valuenow')), { message: `${name} widens` }).toBeGreaterThan(before + 20)
    await handle.focus()
    const widened = Number(await handle.getAttribute('aria-valuenow'))
    await page.keyboard.press('ArrowLeft')
    await expect.poll(async () => Number(await handle.getAttribute('aria-valuenow')), { message: `${name} narrows by key` }).toBe(widened - 16)
  }
  await expect.poll(() => workCalls.filter(c => c.method === 'PUT' && c.path === '/api/preferences/quotes').length).toBeGreaterThan(0)
})

test('keyboard: j/k move, Enter docks, Escape closes, / searches, n starts a quote', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await setup(page)
  await openList(page)
  await page.getByRole('grid', { name: 'Quotes' }).focus()
  await expect(rows(page).first()).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('j')
  await expect(rows(page).nth(1)).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('k')
  await expect(rows(page).first()).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(new RegExp(`/business/quotes\\?quote=${Q.second}$`))
  const dock = page.locator('.quote-dock')
  await expect(dock.getByRole('region', { name: /^Quote A/ })).toBeVisible()
  await expect(dock.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
  await page.getByRole('grid', { name: 'Quotes' }).focus()
  await page.keyboard.press('Escape')
  await expect(page).toHaveURL(/\/business\/quotes$/)
  await expect(dock).toHaveCount(0)
  await page.keyboard.press('/')
  await expect(page.getByRole('searchbox', { name: 'Find a quote' })).toBeFocused()
  await page.keyboard.press('Escape')
  await page.keyboard.press('Escape')
  await page.keyboard.press('n')
  await expect(page.getByRole('dialog', { name: 'New quote' })).toBeVisible()
})

test('a new quote from the list: choose the customer by typing, name it, open it', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await setup(page)
  await openList(page)
  await page.getByRole('button', { name: 'New quote' }).click()
  const dialog = page.getByRole('dialog', { name: 'New quote' })
  await expect(dialog.getByRole('combobox', { name: 'Customer' })).toBeFocused()
  await dialog.getByRole('button', { name: 'Create quote' }).click()
  await expect(dialog.getByText('Choose who the quote is for.')).toBeVisible()
  await dialog.getByRole('combobox', { name: 'Customer' }).fill('hof')
  await expect(dialog.getByRole('option')).toHaveCount(1)
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('ArrowUp')
  await page.keyboard.press('Enter')
  await expect(dialog.getByText('Bäckerei Hofer')).toBeVisible()
  await expect(dialog.getByLabel('Title')).toBeFocused()
  await dialog.getByLabel('Title').fill('Neue Filiale in Leoben')
  await expect(dialog.getByLabel('Project')).toBeVisible()
  await dialog.getByRole('button', { name: 'Create quote' }).click()
  await expect.poll(() => calls.find(c => c.method === 'POST' && c.path === '/api/quotes')?.body).toEqual({ title: 'Neue Filiale in Leoben', customer_org_node_id: HOFER })
  await expect(page).toHaveURL(/\/business\/quotes\/c0ffee00-0000-4000-8000-0000000001\d\d$/)
  await expect(page.getByRole('region', { name: 'Quote editor' })).toBeVisible()
})

test('quote creation sends an incomplete sender to business settings', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const { crm, calls } = await setup(page)
  crm.quoteSettings.sender.email = ''
  await page.goto('/business/quotes')
  await page.getByRole('button', { name: 'New quote' }).click()
  const dialog = page.getByRole('dialog', { name: 'New quote' })
  await expect(dialog.getByRole('alert')).toContainText('Set up the sender first')
  await expect(dialog.getByRole('button', { name: 'Create quote' })).toBeDisabled()
  await dialog.getByRole('link', { name: 'Open Settings › Business' }).click()
  await expect(page).toHaveURL(/\/settings\/business/)
  expect(calls.filter(call => call.method === 'POST' && call.path === '/api/quotes')).toHaveLength(0)
})

test('the Quotes tab shows recent acceptances for offer creators', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const { calls, world } = await setup(page)
  await page.goto('/business/quotes')
  const notices = page.locator('.acceptance-notices')
  await expect(notices.getByText('Recently accepted offers you created')).toBeVisible()
  await notices.locator('summary').click()
  await expect(notices.getByRole('link', { name: /version 1/ })).toHaveAttribute('href', `/business/quotes/${Q.accepted}`)
  expect(calls.some(call => call.path === '/api/quotes/acceptances' && call.query.get('created_by_me') === 'true')).toBe(true)
  world.notices.length = 0
  await page.reload()
  await expect(notices).toHaveCount(0)
})

test('a new quote from a customer’s page names the customer and needs no choice', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await setup(page)
  await page.goto(`/business/customers/${HOFER}`)
  await page.getByRole('button', { name: 'New quote' }).first().click()
  const dialog = page.getByRole('dialog', { name: 'New quote' })
  await expect(dialog.locator('.fixed-customer')).toContainText('Bäckerei Hofer')
  await expect(dialog.getByLabel('Title')).toBeFocused()
  await dialog.getByLabel('Title').fill('Kassensystem Filiale 10')
  await expect(dialog.getByRole('button', { name: 'Create quote' })).toBeEnabled()
  await page.keyboard.press('Control+Enter')
  await expect.poll(() => calls.find(c => c.method === 'POST' && c.path === '/api/quotes')?.body).toEqual({ title: 'Kassensystem Filiale 10', customer_org_node_id: HOFER })
  await expect(page).toHaveURL(/\/business\/quotes\/c0ffee00/)
})

test('without a sender the dialog says what to set up first and creates nothing', async ({ page }) => {
  await setup(page, { quoteRevision: 0 })
  await openList(page)
  await page.getByRole('button', { name: 'New quote' }).click()
  const dialog = page.getByRole('dialog', { name: 'New quote' })
  await expect(dialog.getByText('Set up the sender first')).toBeVisible()
  await expect(dialog.getByRole('link', { name: 'Open Settings › Business' })).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Create quote' })).toBeDisabled()
})

test('honest states: no quotes yet, not allowed, and a failed load with a retry', async ({ page }) => {
  await setup(page, { empty: true })
  await page.goto('/business/quotes')
  await expect(page.getByRole('heading', { name: 'No quotes yet' })).toBeVisible()
  await expect(page.locator('.state').getByRole('button', { name: 'New quote' })).toBeVisible()

  await page.unrouteAll({ behavior: 'ignoreErrors' })
  await setup(page, { listStatus: 403 })
  await page.goto('/business/quotes')
  await expect(page.getByRole('heading', { name: 'Quotes are not open to you' })).toBeVisible()

  await page.unrouteAll({ behavior: 'ignoreErrors' })
  await setup(page, { listStatus: 500 })
  await page.goto('/business/quotes')
  await expect(page.getByRole('heading', { name: 'Quotes could not be loaded' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible()
})

test('phones get one card per quote, nothing cut, opening goes to the quote’s page', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await setup(page)
  await page.goto('/business/quotes')
  const cards = page.getByRole('list', { name: 'Quotes' }).getByRole('listitem')
  await expect(cards).toHaveCount(7)
  const cut = await page.evaluate(() => [...document.querySelectorAll<HTMLElement>('.biz-page *')].filter(el => {
    const r = el.getBoundingClientRect(), c = getComputedStyle(el)
    if (r.width <= 1 || r.height <= 1 || c.display === 'none' || el.closest('svg')) return false
    // A dot list clips its leading dots past its own left edge on purpose.
    return !el.classList.contains('dot-list') && (r.left < -0.5 || r.right > innerWidth + 0.5)
  }).map(el => `${el.tagName}.${el.className}`))
  expect(cut).toEqual([])
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await cards.first().getByRole('link').click()
  await expect(page).toHaveURL(new RegExp(`/business/quotes/${Q.second}$`))
})

for (const colorScheme of ['light', 'dark'] as const) {
  test(`axe: the list, its filters and the new-quote dialog in ${colorScheme}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme })
    await page.setViewportSize({ width: 1440, height: 900 })
    await setup(page)
    await openList(page)
    await axe(page)
    await toolbar(page).getByRole('button', { name: 'Amount' }).click()
    await axe(page)
    await page.keyboard.press('Escape')
    await page.getByRole('button', { name: 'New quote' }).click()
    await expect(page.getByRole('dialog', { name: 'New quote' })).toBeVisible()
    await axe(page)
  })
}
