// SPDX-License-Identifier: AGPL-3.0-only
import { readFileSync } from 'node:fs'
import { expect, test } from '@playwright/test'
import fixture from './quotes/fixtures/synthetic-document.json' with { type: 'json' }
import { defaultProfile } from '../src/lib/quotes/profile'
import { fixtures, mockWork } from './work-fixtures'
import { businessData, mockBusiness } from './business-fixtures'
import { mockSettings, settingsData } from './settings-fixtures'

test('a frozen classic profile prints its font and geometry under the production CSP', async ({ page }) => {
  const assetId = '11111111-1111-4111-8111-111111111111'
  const font = readFileSync(new URL('../src/assets/fonts/jetbrains.woff2', import.meta.url))
  const definition = defaultProfile()
  definition.fonts = [{ role: 'body', family: 'Synthetic Sans', weight: 400, style: 'normal', asset_id: assetId }]
  definition.footer.page_number_format = 'PAGE {page} OF {total}'
  const document = { ...structuredClone(fixture), profile: { id: '22222222-2222-4222-8222-222222222222', revision: 3, definition } }
  await page.route('**/quote-print.html', async route => {
    const response = await route.fetch()
    await route.fulfill({ response, headers: { ...response.headers(), 'content-security-policy': "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'" } })
  })
  await page.route('**/payload', route => route.fulfill({ json: { document, offer_no: 'Q-EXAMPLE-01' } }))
  await page.route(`**/api/quote-profiles/assets/${assetId}`, route => route.fulfill({ body: font, contentType: 'font/woff2' }))
  await page.goto('/quote-print.html')
  await expect(page.locator('html')).toHaveAttribute('data-pdf-ready', 'true')
  expect(Number.parseFloat(await page.locator('.quote-page').first().evaluate(el => getComputedStyle(el).paddingTop))).toBeCloseTo(68.03, 1)
  await expect(page.locator('.quote-page-header').first()).toContainText('Q-EXAMPLE-01')
  await expect(page.locator('.quote-page-footer').first()).toContainText('PAGE 1 OF')
  expect(await page.evaluate(() => Array.from(document.fonts).some(face => face.family.includes('QuoteProfile') && face.status === 'loaded'))).toBe(true)
})

test('Business settings saves a synthetic profile and selects it for new quotes', async ({ page }) => {
  await mockWork(page, fixtures())
  await mockBusiness(page, businessData())
  const settings = settingsData()
  await mockSettings(page, settings)
  const id = '33333333-3333-4333-8333-333333333333'
  let saved: { name: string; definition: ReturnType<typeof defaultProfile> } | null = null
  await page.route('**/api/quote-profiles', async route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: saved ? [{ id, revision: 1, archived: false, ...saved }] : [] })
    const body = route.request().postDataJSON() as typeof saved
    saved = body
    return route.fulfill({ status: 201, json: { id, revision: 1, archived: false, ...body } })
  })
  await page.route('**/api/quotes/settings', async route => {
    if (route.request().method() !== 'PATCH') return route.fallback()
    const body = route.request().postDataJSON() as { default_profile_id: string }
    settings.quotes = { ...settings.quotes, revision: settings.quotes.revision + 1, default_profile_id: body.default_profile_id }
    return route.fulfill({ json: settings.quotes })
  })
  // U19: the Business card opens the profile editor; a new profile is created there.
  await page.goto('/settings/business#quote-profiles')
  await page.locator('#quote-profiles').getByRole('link', { name: 'New profile' }).click()
  await expect(page).toHaveURL('/settings/business/profiles/new')
  await page.getByLabel('Name', { exact: true }).fill('Beispiel Stahl GmbH')
  await page.getByRole('button', { name: 'Create profile' }).click()
  await expect(page.locator('.toast').last()).toContainText('Created Beispiel Stahl GmbH.')
  expect(saved?.definition.schema).toBe('inspr.document-profile.v1')
  await page.getByRole('checkbox', { name: /Default for new quotes/ }).check()
  await expect(page.locator('.toast').last()).toContainText('New quotes start with Beispiel Stahl GmbH.')
  expect(settings.quotes.default_profile_id).toBe(id)
})
