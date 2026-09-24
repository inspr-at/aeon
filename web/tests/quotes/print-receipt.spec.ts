// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test } from '@playwright/test'
import fixture from './fixtures/synthetic-document.json' with { type: 'json' }

test('print entry keeps inline marks and accepted receipt evidence', async ({ page }) => {
  const document = structuredClone(fixture)
  document.sections[0]!.nodes[0]!.text = 'Bold 😀 and italic'
  document.sections[0]!.body = 'Bold 😀 and italic'
  Object.assign(document.sections[0]!.nodes[0]!, {
    marks: [
      { start: 0, end: 4, bold: true },
      { start: 5, end: 7, bold: true, italic: true },
      { start: 8, end: 11, italic: true },
    ],
  })
  const digest = 'a'.repeat(64)
  await page.route('**/payload', route => route.fulfill({ json: {
    document, offer_no: 'A260924-01', accepted: { name: 'Example Signer', company: 'Example Customer', at: '2026-09-24T12:00:00Z', digest },
  } }))
  await page.goto('/quote-print.html')
  await expect(page.locator('html')).toHaveAttribute('data-pdf-ready', 'true')
  const prose = page.locator('.quote-page .quote-prose-text').first()
  await expect(prose.locator('strong')).toHaveCount(2)
  await expect(prose.locator('strong em')).toHaveText('😀')
  await expect(prose.locator(':scope > em')).toHaveText('and')
  await expect(page.locator('.quote-page .quote-stamp')).toContainText('Example Signer')
  await expect(page.locator('.quote-page .quote-stamp')).toContainText(digest)
  await expect(page.locator('.quote-page .quote-print-qr')).toHaveCount(0)
})
