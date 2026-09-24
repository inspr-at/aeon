// SPDX-License-Identifier: AGPL-3.0-only
// U18 (AEON-99) closes the P4/P7 rows of the QP8 parity matrix
// (web/tests/quotes/parity-matrix.json): inspector icons (PAI-1066, F15.07),
// zoom (PAI-1064.AC06), the footer mark (F08.03), inline styles across surfaces
// (F15.09), and the draft/issue distribution rules as the UI shows them (F04).
import { expect, test, type Page } from '@playwright/test'
import { fixtures, mockWork } from './work-fixtures'
import { mockQuoteEditor, NODES, QUOTE_ID, quoteDocument, type QuoteDoc } from './quote-inspector-fixtures'
import { crmData, mockCRM } from './crm-fixtures'
import { Q, mockPublicQuote, mockQuotes, quoteWorld } from './quote-list-fixtures'

const inspector = (page: Page) => page.getByRole('complementary', { name: 'Format' })
async function open(page: Page, doc?: QuoteDoc) {
  await mockWork(page, fixtures())
  const mock = await mockQuoteEditor(page, doc)
  await page.goto(`/business/quotes/${QUOTE_ID}`)
  await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
  return mock
}
async function selectText(page: Page, node: string, from: number, to = from) {
  const el = page.locator(`.quote-page [data-text-id="${node}"]`)
  await el.scrollIntoViewIfNeeded(); await el.click()
  await page.evaluate(({ node, from, to }) => {
    const el = document.querySelector(`.quote-page [data-text-id="${node}"]`)!
    const at = (offset: number) => { let rest = offset; const w = document.createTreeWalker(el, NodeFilter.SHOW_TEXT); let t: Node | null; while ((t = w.nextNode())) { const l = t.textContent!.length; if (rest <= l) return [t, rest] as const; rest -= l } return [el, 0] as const }
    const [a, ao] = at(from), [b, bo] = at(to)
    getSelection()!.setBaseAndExtent(a, ao, b, bo)
  }, { node, from, to })
  await page.keyboard.press('Shift+ArrowRight'); await page.keyboard.press('Shift+ArrowLeft')
}
// Every action control in the panel (and an open section menu): its name and its icon.
interface Control { role: string; name: string; label: string; icon: null | { hidden: string | null; focusable: string | null; box: string | null; stroke: string | null; size: number; dy: number } }
const controls = (page: Page) => page.evaluate(() => [...document.querySelectorAll<HTMLElement>('.quote-inspector-slot button, .quote-inspector-slot a, .quote-inspector-slot [role="radio"], .floating [role="menuitem"]')]
  .filter(el => el.getAttribute('role') !== 'tab' && !el.closest('.outline-row .row-main') && !el.classList.contains('row-main') && !el.closest('.date-field, .picker'))
  .map(el => {
    const svg = el.querySelector(':scope > svg, :scope > span > svg')
    const r = el.getBoundingClientRect(), s = svg?.getBoundingClientRect()
    const label = [...el.childNodes].filter(n => n.nodeType === Node.TEXT_NODE || (n instanceof HTMLElement && !n.matches('svg, kbd, .kbd'))).map(n => n.textContent ?? '').join('').trim()
    return {
      role: el.getAttribute('role') ?? el.tagName.toLowerCase(), name: el.getAttribute('aria-label') ?? el.innerText.trim(), label,
      icon: svg && s ? { hidden: svg.getAttribute('aria-hidden'), focusable: svg.getAttribute('focusable'), box: svg.getAttribute('viewBox'), stroke: svg.getAttribute('stroke-width'), size: Math.round(s.width), dy: Math.abs((s.top + s.height / 2) - (r.top + r.height / 2)) } : null,
    }
  }))
// Floating and sheet panels cover the paper: they close to reach the text and open again.
async function onPaper(page: Page, act: () => Promise<void>) {
  const floating = await page.getByRole('button', { name: 'Close format' }).count() > 0
  if (floating) await page.getByRole('button', { name: 'Close format' }).click()
  await act()
  if (floating) await page.getByRole('button', { name: 'Format panel' }).click()
}
async function contexts(page: Page, visit: (name: string) => Promise<void>) {
  await onPaper(page, () => selectText(page, NODES[4]!, 2))
  await visit('text: numbered item')
  await onPaper(page, () => selectText(page, NODES[7]!, 2))
  await visit('text: bulleted item')
  await inspector(page).getByRole('tab', { name: 'Section' }).click()
  await visit('section')
  await inspector(page).getByRole('tab', { name: 'Document' }).click()
  await visit('document')
  const floating = await page.getByRole('button', { name: 'Close format' }).count() > 0
  if (floating) await page.getByRole('button', { name: 'Close format' }).click()
  await page.getByRole('textbox', { name: 'Heading section 2' }).click()
  await page.getByRole('button', { name: 'Section 2 actions' }).click()
  await visit('section menu')
  await page.keyboard.press('Escape')
  if (floating) await page.getByRole('button', { name: 'Format panel' }).click()
}

test('inspector icons: labelled actions carry decorative icons from the product set, sized like Undo', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await open(page)
  const undo = await page.getByRole('button', { name: 'Undo', exact: true }).locator('svg').evaluate(svg => ({ size: Math.round(svg.getBoundingClientRect().width), stroke: svg.getAttribute('stroke-width'), box: svg.getAttribute('viewBox') }))
  const seen = new Set<string>()
  await contexts(page, async context => {
    for (const control of await controls(page)) {
      expect(control.name, `${context}: every control has a name`).not.toBe('')
      if (!control.icon) continue
      seen.add(control.label || control.name)
      expect(control.icon, `${context}: ${control.name}`).toMatchObject({ hidden: 'true', focusable: 'false', box: undo.box, stroke: undo.stroke })
      expect(Math.abs(control.icon.size - undo.size), `${context}: ${control.name} is Undo-sized`).toBeLessThanOrEqual(1)
      expect(control.icon.dy, `${context}: ${control.name} icon is centred`).toBeLessThanOrEqual(1.5)
    }
  })
  // Section actions, list types, indentation, insertion, numbering and templates.
  for (const label of ['None', 'Bullets', 'Numbers', 'Outdent', 'Indent', 'Bold', 'Italic', 'Clear formatting', 'Automatic', 'Continue', 'Start at', 'Add a section at the end', 'Add section below', 'Delete section', 'Edit templates']) {
    expect([...seen], `${label} has an icon`).toContain(label)
  }
  expect([...seen].some(label => label.startsWith('Move up')) && [...seen].some(label => label.startsWith('Move down'))).toBe(true)
})

test('inspector icons: numbering and reset actions are consistent, nothing doubled or drawn with text glyphs', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await open(page)
  await contexts(page, async context => {
    const groups = await page.evaluate(() => [...document.querySelectorAll<HTMLElement>('.quote-inspector-slot [role="radiogroup"]')].map(group => ({
      name: group.getAttribute('aria-label') ?? '',
      options: [...group.querySelectorAll<HTMLElement>('[role="radio"]')].map(radio => ({ icons: radio.querySelectorAll('svg').length, glyph: radio.classList.contains('glyph') })),
    })))
    for (const group of groups) {
      // A group is all icons, all glyph previews (1, I, a, •) or all words; never a mix.
      const kinds = new Set(group.options.map(o => o.icons ? 'icon' : o.glyph ? 'glyph' : 'word'))
      expect(kinds.size, `${context}: ${group.name} is consistent`).toBe(1)
      expect(group.options.every(o => o.icons <= 1), `${context}: ${group.name} has one icon per option`).toBe(true)
    }
    for (const control of await controls(page)) {
      expect(control.label, `${context}: ${control.name} draws no icon with text`).not.toMatch(/[‹›✕×↺↻←→⟲⟳+]/)
    }
  })
  // A moved marker offers Reset with its icon, like the other resets.
  await selectText(page, NODES[7]!, 2)
  await inspector(page).getByRole('tab', { name: 'Text' }).click()
  await inspector(page).getByRole('spinbutton', { name: 'Marker across' }).fill('2')
  await inspector(page).getByRole('spinbutton', { name: 'Marker across' }).press('Enter')
  await expect(inspector(page).getByRole('button', { name: 'Reset' }).locator('svg')).toHaveAttribute('aria-hidden', 'true')
})

test('inspector icons: labels stay whole on one line without overflow at every panel width', async ({ page }) => {
  for (const [width, height] of [[1440, 900], [1024, 800], [390, 844]] as const) {
    await page.setViewportSize({ width, height })
    if (width === 1440) await open(page); else await page.reload()
    await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
    if (width < 1100) { await selectText(page, NODES[4]!, 2); await page.getByRole('button', { name: 'Format panel' }).click() }
    await contexts(page, async context => {
      const problems = await page.evaluate(() => {
        const out: string[] = []
        const panel = document.querySelector<HTMLElement>('.quote-inspector-slot .panel')
        if (panel && panel.scrollWidth > panel.clientWidth + 1) out.push('panel scrolls sideways')
        for (const el of document.querySelectorAll<HTMLElement>('.quote-inspector-slot button, .quote-inspector-slot [role="radio"], .quote-inspector-slot a, .floating [role="menuitem"]')) {
          const r = el.getBoundingClientRect()
          if (r.width < 1 || el.getAttribute('role') === 'tab') continue
          if (el.scrollWidth > el.clientWidth + 1) out.push(`${el.innerText.trim() || el.getAttribute('aria-label')} is cut`)
          for (const span of el.querySelectorAll<HTMLElement>(':scope > span')) {
            // Section titles in the outline end in an ellipsis by design (the full title is on the page).
            if (getComputedStyle(span).textOverflow === 'ellipsis') continue
            if (span.getClientRects().length > 1 || span.scrollWidth > span.clientWidth + 1) out.push(`${span.innerText.trim()} wraps or is cut`)
          }
        }
        return out
      })
      expect(problems, `${width}px ${context}`).toEqual([])
    })
  }
})

test('inspector icons: every context behaves as before and nothing reaches the saved document or the PDF', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await open(page)
  await selectText(page, NODES[2]!, 2)
  await inspector(page).getByRole('radio', { name: 'Bullets' }).click()
  await expect(inspector(page).getByRole('radio', { name: 'Bullets' })).toHaveAttribute('aria-checked', 'true')
  // Keyboard still walks the list types.
  await inspector(page).getByRole('radio', { name: 'Bullets' }).focus()
  await page.keyboard.press('ArrowRight')
  await expect(inspector(page).getByRole('radio', { name: 'Numbers' })).toHaveAttribute('aria-checked', 'true')
  await inspector(page).getByRole('button', { name: 'Indent' }).click()
  await inspector(page).getByRole('tab', { name: 'Section' }).click()
  const before = await page.locator('.quote-page .quote-section').count()
  await inspector(page).getByRole('button', { name: 'Add a section at the end' }).click()
  await expect(page.locator('.quote-page .quote-section')).toHaveCount(before + 1)
  await page.getByRole('button', { name: 'Undo', exact: true }).click()
  await expect(page.locator('.quote-page .quote-section')).toHaveCount(before)
  await inspector(page).getByRole('tab', { name: 'Document' }).click()
  const saves = () => calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft'))
  expect(saves()).toHaveLength(0)
  await page.getByRole('button', { name: 'Save draft' }).click()
  await expect.poll(() => saves().length).toBe(1)
  const body = saves()[0]!.body as { document: QuoteDoc }
  const node = body.document.sections.flatMap(s => s.nodes).find(n => n.id === NODES[2]) as { kind: string; marker?: string; depth?: number }
  expect(node).toMatchObject({ kind: 'item', marker: 'decimal', depth: 1 })
  expect(JSON.stringify(body)).not.toMatch(/<svg|aria-hidden|quote-icon/)
  await page.emulateMedia({ media: 'print' })
  await expect(page.locator('.quote-inspector-slot')).toBeHidden()
  await expect(page.locator('.quote-page').first()).toBeVisible()
})

test('zoom: fit width, fit page, 25 % and 800 % keep the caret, panel, print size and data; phones fit the width', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await open(page)
  const zoomTo = async (choice: string) => {
    await page.getByRole('button', { name: /^Zoom \d+ %/ }).click()
    const pop = page.getByRole('dialog', { name: 'Zoom' })
    if (/^\d+$/.test(choice)) await pop.getByRole('group', { name: 'Zoom presets' }).getByRole('button', { name: choice, exact: true }).click()
    else await pop.getByRole('button', { name: choice }).click()
  }
  const zoom = () => page.locator('.paper-stack').evaluate(el => Number(getComputedStyle(el).zoom))
  const type = async (letter: string) => {
    const el = page.locator(`.quote-page [data-text-id="${NODES[0]}"]`)
    await el.scrollIntoViewIfNeeded(); await el.click()
    await page.keyboard.press('End'); await page.keyboard.type(letter)
    await expect(el).toContainText(new RegExp(`${letter}$`))
  }
  for (const [choice, check] of [['Fit width', (z: number) => z > 0.9], ['Fit page', (z: number) => z > 0.2 && z < 1], ['25', (z: number) => z === 0.25], ['800', (z: number) => z === 8]] as const) {
    await zoomTo(choice)
    await expect.poll(async () => check(await zoom()), { message: choice }).toBe(true)
    await expect(inspector(page)).toBeVisible()
    await type(choice === '800' ? 'Z' : 'Y')
    // The desk scrolls to what the caret is in.
    const scroll = await page.locator('.quote-desk').evaluate(el => ({ h: el.scrollHeight > el.clientHeight || el.scrollWidth > el.clientWidth }))
    if (choice === '800') expect(scroll.h).toBe(true)
  }
  // Zooming alone never changed the quote; typing did, and only that is saved.
  expect(calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft'))).toHaveLength(0)
  await page.emulateMedia({ media: 'print' })
  await expect.poll(zoom).toBe(1)
  await page.emulateMedia({ media: 'screen' })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.reload()
  await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
  await expect(page.getByRole('button', { name: /^Zoom \d+ %/ })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})

test('footer mark: sized 18..96 mm and moved -6..10 mm in tenths; the clicked page’s mark is the one selected; legacy values stay', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const doc = quoteDocument()
  doc.layout = { logo_file_id: 'att-mark', logo_width_mm: '22.5' } as never
  const { calls } = await open(page, doc)
  const marks = page.getByRole('button', { name: /^Company mark on page \d+$/ })
  const pages = Number(await page.locator('.quote-document').getAttribute('data-page-count'))
  await expect(marks).toHaveCount(pages)
  expect(pages).toBeGreaterThan(1)
  const mm = (value: number) => value * 96 / 25.4
  const markWidth = () => marks.first().evaluate(el => el.getBoundingClientRect().width / Number(getComputedStyle(document.querySelector('.paper-stack')!).zoom))
  await expect.poll(markWidth).toBeCloseTo(mm(22.5), 0)
  const last = page.getByRole('button', { name: `Company mark on page ${pages}` })
  await last.scrollIntoViewIfNeeded()
  await last.click()
  await expect(last).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('button', { name: 'Company mark on page 1' })).toHaveAttribute('aria-pressed', 'false')
  await expect(inspector(page).getByRole('tab', { name: 'Document' })).toHaveAttribute('aria-selected', 'true')
  await expect(inspector(page)).toContainText(`The mark on page ${pages} is selected.`)
  const width = inspector(page).getByRole('spinbutton', { name: 'Mark width' })
  await expect(width).toHaveValue('22.5')
  await width.fill('96.1'); await width.press('Enter')
  await expect(inspector(page).getByText('Use 18 to 96 mm.')).toBeVisible()
  await width.fill('40.5'); await width.press('Enter')
  await expect.poll(markWidth).toBeCloseTo(mm(40.5), 0)
  const offset = inspector(page).getByRole('spinbutton', { name: 'Mark up or down' })
  await offset.fill('10.1'); await offset.press('Enter')
  await expect(inspector(page).getByText('Use −6 to 10 mm.')).toBeVisible()
  await offset.fill('-2.5'); await offset.press('Enter')
  await expect(marks.nth(1)).toHaveAttribute('style', /translateY\(-2\.5mm\)/)
  await page.getByRole('button', { name: 'Save draft' }).click()
  await expect.poll(() => (calls.find(c => c.method === 'PATCH' && c.path.endsWith('/draft'))?.body as { document?: { layout?: unknown } } | undefined)?.document?.layout).toEqual({ logo_file_id: 'att-mark', logo_width_mm: '40.5', logo_offset_mm: '-2.5' })
})

test('inline styles persist through save and reload and render the same on the customer’s page', async ({ page, context }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await open(page)
  // Part of a word bold, a passage italic, one of them cleared again.
  await selectText(page, NODES[0]!, 4, 12)
  await inspector(page).getByRole('button', { name: 'Bold' }).click()
  await selectText(page, NODES[1]!, 0, 4)
  await page.keyboard.press(process.platform === 'darwin' ? 'Meta+i' : 'Control+i')
  await selectText(page, NODES[2]!, 0, 3)
  await inspector(page).getByRole('button', { name: 'Bold' }).click()
  await inspector(page).getByRole('button', { name: 'Clear formatting' }).click()
  await page.getByRole('button', { name: 'Save draft' }).click()
  const saves = () => calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft'))
  await expect.poll(() => saves().length).toBe(1)
  const saved = (saves()[0]!.body as { document: QuoteDoc }).document
  const marksOf = (doc: QuoteDoc, i: number) => (doc.sections.flatMap(s => s.nodes).find(n => n.id === NODES[i]) as { marks?: unknown }).marks
  expect(marksOf(saved, 0)).toEqual([{ start: 4, end: 12, bold: true }])
  expect(marksOf(saved, 1)).toEqual([{ start: 0, end: 4, italic: true }])
  expect(marksOf(saved, 2)).toBeUndefined()
  const shown = (p: Page, i: number) => p.locator(`.quote-page [data-text-id="${NODES[i]}"]`).evaluate(el => el.innerHTML.replace(/<!--.*?-->/g, ''))
  const editor = [await shown(page, 0), await shown(page, 1)]
  await page.reload()
  await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
  expect([await shown(page, 0), await shown(page, 1)]).toEqual(editor)
  // The customer's page renders the saved document with the same runs.
  const customer = await context.newPage()
  await mockWork(customer, fixtures())
  await customer.route('**/api/public/quotes/**', route => route.fulfill({ json: { document: saved, offer_no: 'A260924-1', version: 1, content_sha256: 'a'.repeat(64), state: 'issued', expires_at: new Date(Date.now() + 864e5).toISOString(), acceptable: true, receipt_ready: false } }))
  await customer.goto('/offers/sel-demo/tok')
  await expect(customer.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
  expect([await shown(customer, 0), await shown(customer, 1)]).toEqual(editor)
  await customer.close()
})

test('a draft offers no customer capability; issuing sends nothing; the link goes out by copy, QR or PDF', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await mockWork(page, fixtures())
  await mockCRM(page, crmData())
  const calls = await mockQuotes(page, quoteWorld())
  await page.goto(`/business/quotes/${Q.draft}`)
  await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
  await page.getByRole('button', { name: 'Details' }).click()
  const details = page.getByRole('complementary', { name: 'Details' })
  await expect(details.getByRole('heading', { name: 'Customer link' })).toHaveCount(0)
  await expect(details).toContainText('Only people in this workspace can see it.')
  await details.getByRole('button', { name: 'Issue quote…' }).click()
  const confirm = page.getByRole('dialog', { name: /^Issue / })
  await expect(confirm).toContainText('Nothing is sent: you share it with a customer link or as a PDF.')
  await confirm.getByRole('button', { name: 'Issue quote' }).click()
  await expect(details.getByRole('heading', { name: 'Customer link' })).toBeVisible()
  await expect(details).toContainText('Nothing was sent.')
  expect(calls.some(c => /mail|send/.test(c.path))).toBe(false)
  await details.getByRole('button', { name: 'Create link' }).click()
  await expect(details.getByLabel('Copy it now: it is shown only this once')).toHaveValue(/\/offers\//)
  await details.getByRole('button', { name: 'QR code' }).click()
  await expect(details.getByRole('img', { name: 'QR code of the customer link' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'PDF' })).toBeVisible()
})

test('the customer page and the Quotes tab are reachable routes, not parked ones', async ({ page }) => {
  await mockWork(page, fixtures())
  await mockCRM(page, crmData())
  await mockQuotes(page, quoteWorld())
  await mockPublicQuote(page, { acceptable: true })
  await page.goto('/business/quotes')
  await expect(page).toHaveURL(/\/business\/quotes$/)
  await expect(page.getByRole('grid', { name: 'Quotes' })).toBeVisible()
  await page.goto(`/business/quotes/${Q.issued}`)
  await expect(page.getByRole('region', { name: 'Quote editor' })).toBeVisible()
  await page.goto('/offers/sel-demo/tok-example')
  await expect(page.locator('#pq-title')).toBeVisible()
})
