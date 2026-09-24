// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test, type Page } from '@playwright/test'

const sectionId = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
const nodeId = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'
const positionId = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc'
function documentFixture(text = 'First line') {
  return {
    schema_version: 1, minimum_writer_version: 1, title: 'Synthetic quote', subtitle: '', project_ref: '',
    offer_date: '2026-09-24', valid_until: '2026-10-24', currency: 'EUR',
    sender: { company: 'Example Studio', street: 'Example Road 1', postal_code: '1000', city: 'Test City', country: 'AT', email: 'studio@example.invalid' },
    recipient: { name: 'Sample Customer', address: 'Test Street', customer_no: 'C-1', country: 'AT' },
    legal: { intro: 'Sample introduction', accept_text: 'I accept.', vat_note: 'Terms apply.' }, layout: {},
    sections: [{ id: sectionId, heading: 'Scope', body: text, nodes: [{ id: nodeId, kind: 'paragraph', text }] }],
    positions: [{ id: positionId, pricing_source: 'manual', short_text: 'Work', long_text: '', quantity: '1', unit_label: 'hour', unit_price_cents: 100, total_cents: 100, currency: 'EUR' }],
    net_total_cents: 100,
  }
}
async function mount(page: Page, fixture = documentFixture(), expectReady = true) {
  await page.route('**/api/**', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{}' }))
  await page.goto('/')
  await page.evaluate(async doc => {
    const path = '/tests/quote-editor-harness.ts'
    const harness = await import(/* @vite-ignore */ path)
    harness.mountQuoteEditor(doc)
  }, fixture)
  if (expectReady) await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
}

test('plain paste, Unicode composition and history keep stable prose IDs', async ({ page }) => {
  await mount(page)
  const prose = page.getByRole('textbox', { name: 'Section 1 text' }).first()
  await prose.click()
  await page.keyboard.press('End')
  await page.evaluate(() => {
    const data = new DataTransfer()
    data.setData('text/plain', '\nPlain 😀 text')
    data.setData('text/html', '<b>Ignored HTML</b>')
    const el = document.querySelector('.quote-page .quote-prose')!
    el.dispatchEvent(new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData: data }))
  })
  await expect(prose).toContainText('Plain 😀 text')
  await expect(page.locator('.quote-page .quote-prose b')).toHaveCount(0)
  await prose.press('ControlOrMeta+z')
  await expect(prose).not.toContainText('Plain 😀 text')
  await prose.press('ControlOrMeta+Shift+z')
  await expect(prose).toContainText('Plain 😀 text')
  await page.evaluate(() => {
    const node = [...document.querySelectorAll('.quote-page .quote-prose [data-text-id]')].at(-1)!
    node.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true, data: '' }))
    node.textContent = `${node.textContent}漢`
    node.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertCompositionText', data: '漢', isComposing: true }))
    node.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true, data: '漢' }))
  })
  await expect(prose).toContainText('漢')
  const stored = await page.evaluate(() => (window as unknown as { quoteModel: { value: { sections: Array<{ nodes: Array<{ id: string; text: string }> }> } } }).quoteModel.value.sections[0]!.nodes)
  expect(stored[0].id).toBe(nodeId)
  expect(stored[1].text).toBe('Plain 😀 text漢')
})

test('wrapped Home/End and Shift+Home stay on the visual line', async ({ page }) => {
  await mount(page)
  await page.evaluate(async () => {
    const path = '/tests/quote-editor-harness.ts'
    const harness = await import(/* @vite-ignore */ path)
    harness.mountWrappedText()
  })
  const wrapped = page.getByRole('textbox', { name: 'Wrapped test' })
  await wrapped.focus()
  const original = await page.evaluate(() => {
    const el = document.querySelector('#wrapped-test [role=textbox]')!
    const text = el.firstChild!
    const range = document.createRange()
    range.setStart(text, 29); range.collapse(true)
    const selection = window.getSelection()!
    selection.removeAllRanges(); selection.addRange(range)
    return range.getBoundingClientRect().top
  })
  await page.keyboard.press('Home')
  const home = await page.evaluate(() => ({ offset: window.getSelection()!.focusOffset, top: window.getSelection()!.getRangeAt(0).getBoundingClientRect().top }))
  expect(home.offset).toBeGreaterThan(0)
  expect(home.offset).toBeLessThan(29)
  expect(Math.abs(home.top - original)).toBeLessThan(3)
  await page.keyboard.press('End')
  const end = await page.evaluate(() => window.getSelection()!.focusOffset)
  expect(end).toBeGreaterThan(home.offset)
  await page.keyboard.press('Shift+Home')
  const shifted = await page.evaluate(() => ({ anchor: window.getSelection()!.anchorOffset, focus: window.getSelection()!.focusOffset }))
  expect(shifted.anchor).toBe(end)
  expect(shifted.focus).toBe(home.offset)
})

test('oversized whole section sets the single readiness and overflow contract', async ({ page }) => {
  const long = documentFixture()
  long.sections[0]!.nodes = Array.from({ length: 40 }, (_, index) => ({ id: `dddddddd-dddd-4ddd-8ddd-${String(index).padStart(12, '0')}`, kind: 'paragraph', text: 'Long block '.repeat(45) }))
  long.sections[0]!.body = long.sections[0]!.nodes.map(node => node.text).join('\n')
  await mount(page, long, false)
  await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'false')
  await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-overflow', /Section/)
})
