// SPDX-License-Identifier: AGPL-3.0-only
// U18 (AEON-99): the page a customer opens from a quote link, as a real route.
// The sender's name, the frozen document scaled to the screen, the acceptance
// bound to the displayed fingerprint, ended and unknown links, the receipt once
// accepted; axe in light and dark at desktop and 390; and both content security
// policies: the production one on the app's pages and the stricter public one
// here, with no violation reported by the browser.
import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'
import { crmData, mockCRM } from './crm-fixtures'
import { Q, mockPublicQuote, mockQuotes, quoteWorld } from './quote-list-fixtures'
import { fixtures, mockWork } from './work-fixtures'

const LINK = '/offers/sel-demo/tok-example'
// The production policy (internal/httpapi) and a stricter one for public pages.
// The Vite dev server injects CSS as <style> elements, so these runs allow inline
// styles; the production build ships CSS files and needs no such allowance.
const DEV_STYLES = "style-src 'self' 'unsafe-inline'"
export const PRODUCTION_CSP = "default-src 'self'; img-src 'self' blob: data:"
export const PUBLIC_CSP = "default-src 'none'; script-src 'self'; connect-src 'self'; img-src 'self' data:; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

async function withPolicy(page: Page, policy: string) {
  await page.addInitScript(() => {
    const seen: string[] = []
    ;(window as unknown as { cspViolations: string[] }).cspViolations = seen
    document.addEventListener('securitypolicyviolation', event => { seen.push(`${event.violatedDirective} ${event.blockedURI}`) })
  })
  await page.route('**/*', async route => {
    if (route.request().resourceType() !== 'document') return route.fallback()
    const response = await route.fetch()
    await route.fulfill({ response, headers: { ...response.headers(), 'content-security-policy': `${policy}; ${DEV_STYLES}` } })
  })
}
const violations = (page: Page) => page.evaluate(() => (window as unknown as { cspViolations: string[] }).cspViolations)
async function axe(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  const summary = results.violations.map(v => `${v.id} (${v.impact}): ${v.help}\n${v.nodes.slice(0, 4).map(n => `    ${n.target.join(' ')} — ${n.failureSummary?.split('\n').slice(1, 2).join(' ').trim()}`).join('\n')}`)
  expect(summary, summary.join('\n')).toEqual([])
}
async function open(page: Page, state: Parameters<typeof mockPublicQuote>[1]) {
  await mockWork(page, fixtures())
  const posted = await mockPublicQuote(page, state)
  await page.goto(LINK)
  return posted
}

test('the customer sees the sender, the facts and the frozen document, and accepts exactly what is shown', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  const posted = await open(page, { acceptable: true })
  await expect(page.locator('.pq-sender')).toHaveText('Beispiel Studio GmbH')
  await expect(page.locator('#pq-title')).toHaveText('Relaunch des Kundenportals')
  await expect(page.locator('.pq-facts')).toContainText(/5\s?560,00\sEUR/)
  await expect(page.getByRole('region', { name: 'Quote document' })).toBeVisible()
  await expect(page).toHaveTitle(/^Quote A260922-12 from Beispiel Studio GmbH/)
  expect(await page.evaluate(() => document.querySelector('meta[name="robots"]')?.getAttribute('content'))).toBe('noindex, nofollow, noarchive')

  await page.getByRole('button', { name: 'Review and accept' }).click()
  await expect(page.getByLabel('Your name')).toBeFocused()
  const submit = page.getByRole('button', { name: 'Accept quote' })
  await expect(submit).toBeDisabled()
  await page.getByLabel('I have reviewed this quote and agree to accept it.').check()
  await submit.click()
  await expect(page.getByText('Please enter your name.')).toBeVisible()
  expect(posted).toHaveLength(0)
  await page.getByLabel('Your name').fill('Jana Hofer')
  await page.getByLabel(/^Company/).fill('Hofer Backwaren GmbH')
  await submit.click()
  await expect.poll(() => posted.length).toBe(1)
  expect(posted[0]).toMatchObject({ version: 1, expected_content_sha256: expect.stringMatching(/^1{8}/), name: 'Jana Hofer', company: 'Hofer Backwaren GmbH', confirm: true })
  await expect(page.getByRole('heading', { name: 'Thank you, Jana Hofer' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Thank you, Jana Hofer' }).locator('..')).toBeFocused()
  await expect(page.locator('.pq-status')).toContainText('Accepted')
})

test('at 390 the paper shrinks to the screen and nothing scrolls sideways or is cut', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await open(page, { acceptable: true })
  await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  const cut = await page.evaluate(() => [...document.querySelectorAll<HTMLElement>('.public-quote *')].filter(el => {
    const r = el.getBoundingClientRect(), c = getComputedStyle(el)
    if (r.width <= 1 || r.height <= 1 || c.display === 'none' || el.closest('svg') || el.closest('.pq-paper')) return false
    return r.left < -0.5 || r.right > innerWidth + 0.5
  }).map(el => `${el.tagName}.${el.className}`))
  expect(cut).toEqual([])
  const paper = (await page.locator('.pq-paper').boundingBox())!
  expect(paper.x).toBeGreaterThanOrEqual(0)
  expect(paper.x + paper.width).toBeLessThanOrEqual(390)
})

test('an ended link reads, but cannot accept; an unknown link says so plainly', async ({ page }) => {
  await open(page, { acceptable: false, linkEnded: true })
  await expect(page.getByText(/Acceptance is closed\. This link ended on/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Accept quote' })).toHaveCount(0)
  await page.unrouteAll({ behavior: 'ignoreErrors' })
  await open(page, { acceptable: false, missing: true })
  await expect(page.getByRole('heading', { name: 'This link does not open a quote' })).toBeVisible()
})

test('once accepted, the page says when and offers the receipt', async ({ page }) => {
  await open(page, { acceptable: false, accepted: true, receiptReady: true })
  await expect(page.locator('.pq-status')).toContainText(/^Accepted on /)
  await expect(page.getByRole('link', { name: /^Receipt \(PDF\)/ })).toHaveAttribute('href', '/api/public/quotes/sel-demo/tok-example/pdf')
})

for (const colorScheme of ['light', 'dark'] as const) {
  test(`axe: the customer page at desktop and 390 in ${colorScheme}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme })
    await page.setViewportSize({ width: 1280, height: 900 })
    await open(page, { acceptable: true })
    await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
    await axe(page)
    await page.setViewportSize({ width: 390, height: 844 })
    await axe(page)
  })
}

test('content security: the public page runs under the strict public policy without a violation', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await withPolicy(page, PUBLIC_CSP)
  await open(page, { acceptable: true })
  await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
  await page.getByLabel('Your name').fill('Jana Hofer')
  await page.getByLabel('I have reviewed this quote and agree to accept it.').check()
  await page.getByRole('button', { name: 'Accept quote' }).click()
  await expect(page.getByRole('heading', { name: 'Thank you, Jana Hofer' })).toBeVisible()
  expect(await violations(page)).toEqual([])
})

test('content security: the Quotes list and a docked quote run under the production policy', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await withPolicy(page, PRODUCTION_CSP)
  await mockWork(page, fixtures())
  await mockCRM(page, crmData())
  await mockQuotes(page, quoteWorld())
  await page.goto(`/business/quotes?quote=${Q.accepted}`)
  await expect(page.locator('.quote-dock .quote-document')).toHaveAttribute('data-quote-ready', 'true')
  await page.locator('.quote-dock').getByRole('button', { name: 'Details' }).click()
  await expect(page.getByRole('heading', { name: 'Acceptance receipt' })).toBeVisible()
  expect(await violations(page)).toEqual([])
})
