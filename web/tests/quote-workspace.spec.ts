// SPDX-License-Identifier: AGPL-3.0-only
// U18 (AEON-99): the quote workspace docked beside the list and on its own page.
// One live session across both (unsaved work, undo, one presence lease); issue
// with its fingerprint; the customer link (created with an end, shown once,
// revoked); the acceptance receipt; revising into a new version; older versions;
// people on the quote; the conflict review; archive and duplicate; files.
import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'
import { crmData, mockCRM } from './crm-fixtures'
import { MIRA, Q, mockQuotes, presenceOf, quoteWorld, type QuoteMockOptions, type QuoteWorld } from './quote-list-fixtures'
import { SECTIONS } from './quote-inspector-fixtures'
import { fixtures, mockWork, watchErrors } from './work-fixtures'

async function setup(page: Page, options: QuoteMockOptions & { tune?: (world: QuoteWorld) => void } = {}) {
  const work = fixtures()
  const workCalls = await mockWork(page, work)
  const crm = crmData()
  await mockCRM(page, crm, { role: options.role })
  const world = quoteWorld()
  options.tune?.(world)
  const calls = await mockQuotes(page, world, options)
  return { world, calls, work, workCalls }
}
async function full(page: Page, id: string) {
  await page.goto(`/business/quotes/${id}`)
  await expect(page.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
}
const details = (page: Page) => page.getByRole('complementary', { name: 'Details' })
async function axe(page: Page) {
  const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).exclude('.calendar-version').analyze()
  const summary = results.violations.map(v => `${v.id} (${v.impact}): ${v.help}\n${v.nodes.slice(0, 4).map(n => `    ${n.target.join(' ')} — ${n.failureSummary?.split('\n').slice(1, 2).join(' ').trim()}`).join('\n')}`)
  expect(summary, summary.join('\n')).toEqual([])
}

test('docked beside the list, then on its own page and back: one session, unsaved work and undo kept', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const errors = watchErrors(page)
  const { calls } = await setup(page)
  await page.goto('/business/quotes')
  await page.locator(`#quote-${Q.draft} .c-title`).click()
  await expect(page).toHaveURL(new RegExp(`\\?quote=${Q.draft}$`))
  const dock = page.locator('.quote-dock')
  await expect(dock.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
  // The list stays where it was, beside the quote.
  await expect(page.getByRole('grid', { name: 'Quotes' })).toBeVisible()
  await expect(page.locator(`#quote-${Q.draft}`)).toHaveClass(/open/)
  const title = dock.getByRole('textbox', { name: 'Angebotstitel' })
  await title.click()
  await page.keyboard.press('End')
  await page.keyboard.type(' 2027')
  await expect(dock.getByRole('button', { name: 'Save draft' })).toBeVisible()

  await dock.getByRole('button', { name: 'Open on its own page' }).click()
  await expect(page).toHaveURL(new RegExp(`/business/quotes/${Q.draft}$`))
  const editor = page.getByRole('region', { name: 'Quote editor' })
  await expect(editor.getByRole('textbox', { name: 'Angebotstitel' })).toHaveText('Relaunch des Kundenportals 2027')
  await expect(editor.getByRole('button', { name: 'Save draft' })).toBeVisible()
  await expect(editor.getByRole('button', { name: 'Undo', exact: true })).toBeEnabled()
  // The history made in the panel is the one here: undo steps back through it.
  await editor.getByRole('button', { name: 'Undo', exact: true }).click()
  await expect(editor.getByRole('textbox', { name: 'Angebotstitel' })).toHaveText('Relaunch des Kundenportals 202')
  await editor.getByRole('button', { name: 'Redo', exact: true }).click()
  await expect(editor.getByRole('textbox', { name: 'Angebotstitel' })).toHaveText('Relaunch des Kundenportals 2027')

  await editor.getByRole('button', { name: 'More actions' }).click()
  await page.getByRole('menuitem', { name: 'Show beside the list' }).click()
  await expect(page).toHaveURL(new RegExp(`\\?quote=${Q.draft}$`))
  await expect(dock.getByRole('textbox', { name: 'Angebotstitel' })).toHaveText('Relaunch des Kundenportals 2027')
  // One presence session the whole way: joined once, never left.
  expect(calls.filter(c => c.method === 'POST' && c.path.endsWith('/presence'))).toHaveLength(1)
  expect(calls.filter(c => c.method === 'DELETE' && c.path.includes('/presence/'))).toHaveLength(0)
  if (await dock.getByRole('button', { name: 'Save draft' }).isEnabled()) await dock.getByRole('button', { name: 'Save draft' }).click()
  await expect.poll(() => calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft')).length).toBeGreaterThanOrEqual(1)
  await expect(page.locator(`#quote-${Q.draft}`)).toContainText('Relaunch des Kundenportals')
  expect(errors).toEqual([])
})

test('autosave waits 700 ms, navigation saves first, and unload warns for local edits', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await setup(page)
  await full(page, Q.draft)
  const title = page.getByRole('textbox', { name: 'Angebotstitel' })
  await title.fill('Autosave evidence')
  await expect(page.getByRole('status').filter({ hasText: 'Unsaved changes' }).first()).toBeVisible()
  expect(await page.evaluate(() => { const event = new Event('beforeunload', { cancelable: true }); window.dispatchEvent(event); return event.defaultPrevented })).toBe(true)
  await expect.poll(() => calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft')).length).toBe(1)
  await expect(page.getByRole('status').filter({ hasText: 'Saved' }).first()).toBeVisible()
  await title.fill('Navigation evidence')
  await page.getByRole('link', { name: 'Back to Quotes' }).click()
  await expect(page).toHaveURL(/\/business\/quotes$/)
  expect(calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft')).length).toBe(2)
  expect(calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft')).at(-1)?.body).toMatchObject({ document: { title: 'Navigation evidence' } })
})

test('invalid quantities block automatic save, explicit save, print and navigation save', async ({ page }) => {
  const { calls } = await setup(page)
  await full(page, Q.draft)
  await page.getByRole('textbox', { name: 'Angebotstitel' }).fill('Edited while a number is invalid')
  const quantity = page.getByRole('textbox', { name: 'Menge Position 1' })
  await quantity.fill('1.234')
  expect(await quantity.evaluate((input: HTMLInputElement) => input.checkValidity())).toBe(false)
  await page.waitForTimeout(900)
  expect(calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft'))).toHaveLength(0)
  await page.getByRole('button', { name: 'Save draft' }).click()
  await page.getByRole('button', { name: 'PDF', exact: true }).click()
  expect(calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft'))).toHaveLength(0)
  await page.getByRole('link', { name: 'Back to Quotes' }).click()
  await expect(page.getByRole('dialog', { name: 'Leave with unsaved changes?' })).toBeVisible()
  expect(calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft'))).toHaveLength(0)
  await page.getByRole('dialog', { name: 'Leave with unsaved changes?' }).getByRole('button', { name: 'Cancel' }).click()
  await quantity.fill('1.25')
  await expect.poll(() => calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft')).length).toBe(1)
})

test('the paper edits its cover, dates, recipient, sender contact, positions and acceptance text', async ({ page }) => {
  const { calls, world } = await setup(page)
  await full(page, Q.draft)
  const fields: [string, string][] = [
    ['Angebotstitel', 'Neues Angebot'], ['Untertitel', 'Eine Beschreibung'],
    ['Firma des Kunden', 'Neue Firma'], ['Kundenanschrift', 'Neue Straße 2'],
    ['Kundenkontakt', 'Dana Beispiel'], ['Land des Kunden', 'Österreich'],
    ['Ansprechpartner', 'Alex Neu'], ['Projektreferenz', 'P-44'],
    ['Einleitung', 'Wir bieten an.'], ['Umsatzsteuerhinweis', 'USt. laut Gesetz.'],
    ['Annahmetext', 'Hiermit angenommen.'], ['Leistung Position 1', 'Neue Leistung'],
    ['Einheit Position 1', 'Tag'], ['Beschreibung Position 1', 'Langer Text'],
  ]
  for (const [name, value] of fields) await page.getByRole('textbox', { name, exact: true }).fill(value)
  await page.getByRole('textbox', { name: 'Menge Position 1' }).fill('1.5')
  await page.getByRole('textbox', { name: 'Einzelpreis Position 1' }).fill('99.99')
  for (const [name, date] of [['Angebotsdatum', '25.09.2026'], ['Gültig bis', '30.10.2026']]) {
    await page.getByRole('button', { name: new RegExp(`^${name}:`) }).click()
    await page.getByRole('dialog', { name }).getByRole('textbox', { name: 'Date' }).fill(date)
    await page.getByRole('dialog', { name }).getByRole('button', { name: 'Set' }).click()
  }
  await page.getByRole('button', { name: 'Save draft' }).click()
  await expect.poll(() => calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft')).length).toBeGreaterThan(0)
  const saved = calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft')).at(-1)?.body as { document: Record<string, unknown> }
  expect(saved.document).toMatchObject({
    title: 'Neues Angebot', subtitle: 'Eine Beschreibung', project_ref: 'P-44', offer_date: '2026-09-25', valid_until: '2026-10-30',
    recipient: { name: 'Neue Firma', address: 'Neue Straße 2', contact: 'Dana Beispiel', country: 'Österreich', customer_no: 'K26091' },
    sender: { contact_person: 'Alex Neu' },
    legal: { intro: 'Wir bieten an.', vat_note: 'USt. laut Gesetz.', accept_text: 'Hiermit angenommen.' },
  })
  expect((saved.document.positions as Record<string, unknown>[])[0]).toMatchObject({ short_text: 'Neue Leistung', unit_label: 'Tag', long_text: 'Langer Text', quantity: '1.5', unit_price_cents: 9999 })
  await expect(page.locator('.quote-document')).toContainText('1 / 3')
  // The number is dated two days back from today (quoteWorld), so it is read, not written here.
  await expect(page.locator('.quote-document')).toContainText(world.rows.find(r => r.quote_node_id === Q.draft)!.offer_no!)
})

test('issuing freezes the saved draft with its fingerprint and offers the customer link', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await setup(page)
  await full(page, Q.draft)
  await page.getByRole('button', { name: 'Details' }).click()
  await expect(details(page).getByRole('heading', { name: 'Draft' })).toBeVisible()
  await details(page).getByRole('button', { name: 'Issue quote…' }).click()
  const confirm = page.getByRole('dialog', { name: /^Issue A\d+-12 as version 1\?/ })
  await expect(confirm).toContainText('Nothing is sent')
  await confirm.getByRole('button', { name: 'Issue quote' }).click()
  await expect.poll(() => calls.find(c => c.path.endsWith('/finalize'))?.body).toEqual({ expected_quote_revision: 2, expected_draft_revision: 3, expected_document_sha256: `${'d'.repeat(60)}0003` })
  await expect(page.locator('.state-chip').first()).toContainText('Issued')
  await expect(details(page).getByRole('heading', { name: 'Issued' })).toBeVisible()
  await expect(details(page).locator('.d-digest').first()).toContainText('55555555…')
  await expect(details(page).getByRole('heading', { name: 'Customer link' })).toBeVisible()
  await expect(details(page).getByRole('textbox', { name: 'Customer link' })).toHaveValue(/\/offers\/sel-demo\/tok-link-auto-/)
  // Frozen: nothing on the paper can be edited, and Format steps aside.
  await expect(page.getByRole('button', { name: 'Format panel' })).toHaveCount(0)
  await expect(page.getByRole('textbox', { name: 'Angebotstitel' })).toHaveCount(0)
  await expect(page.locator('.quote-document')).toContainText('Relaunch des Kundenportals')
})

test('an issued quote has no advertised reject action', async ({ page }) => {
  await setup(page)
  await full(page, Q.issued)
  await page.getByRole('button', { name: 'Details' }).click()
  await expect(page.getByRole('button', { name: /decline|reject/i })).toHaveCount(0)
})

test('the customer link: an end date, a stable copy and exact QR; revoking asks first', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await setup(page)
  await full(page, Q.issued)
  const card = details(page).locator('section', { has: page.getByRole('heading', { name: 'Customer link' }) })
  await expect(card).toContainText('Whoever has it can read the quote and accept it')
  await expect(card).toContainText('verifier and admin-only copy are stored separately')
  await card.getByLabel('Link ends after').selectOption('14')
  await card.getByRole('button', { name: 'Create link' }).click()
  await expect.poll(() => calls.some(c => c.method === 'POST' && c.path.endsWith('/public-link'))).toBe(true)
  const post = calls.find(c => c.method === 'POST' && c.path.endsWith('/public-link'))!
  const days = (Date.parse(String((post.body as { expires_at: string }).expires_at)) - Date.now()) / 86_400_000
  expect(days).toBeGreaterThan(13.9); expect(days).toBeLessThan(14.1)
  const url = card.getByRole('textbox', { name: 'Customer link' })
  await expect(url).toHaveValue(/\/offers\/sel-demo\/tok-link-\d+$/)
  const originalUrl = await url.inputValue()
  await card.getByRole('button', { name: 'QR code' }).click()
  const encoded = await page.evaluate(async () => {
    const { decodeQuoteQr } = await import('/tests/quotes/qr-decode-harness.ts')
    return decodeQuoteQr('#quote-link-qr')
  })
  expect(encoded).toBe(await url.inputValue())
  await card.getByRole('button', { name: 'Copy' }).click()
  await expect(card.getByRole('button', { name: 'Copied' })).toBeVisible()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toMatch(/\/offers\/sel-demo\/tok-link-\d+$/)
  await expect(card.getByText('Active', { exact: true })).toBeVisible()

  await page.reload()
  await expect(card.getByText(/^Opens this version until/)).toBeVisible()
  await expect(card.getByRole('textbox', { name: 'Customer link' })).toHaveValue(originalUrl)
  await card.getByRole('button', { name: 'Revoke link' }).click()
  await page.getByRole('dialog', { name: 'Revoke the customer link?' }).getByRole('button', { name: 'Revoke link' }).click()
  await expect(card.getByText(/^Revoked on/)).toBeVisible()
  await expect(card.getByRole('button', { name: 'Create a new link' })).toBeVisible()
})

test('an accepted quote shows its acceptance, the receipt as fixed evidence and its versions', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await setup(page)
  await full(page, Q.accepted)
  await expect(details(page).getByRole('heading', { name: 'Accepted' })).toBeVisible()
  await expect(details(page)).toContainText('through the customer link')
  const receipt = details(page).locator('section', { has: page.getByRole('heading', { name: 'Acceptance receipt' }) })
  await expect(receipt.locator('.pill')).toHaveText('Ready')
  await expect(receipt).toContainText('It is kept exactly as made.')
  await expect(receipt.locator('.d-digest')).toContainText('fefefefe…')
  await expect(receipt.getByRole('link', { name: 'Download receipt (PDF)' })).toHaveAttribute('href', `/api/quotes/${Q.accepted}/versions/1/confirmation/receipt`)
  await expect(receipt).toContainText('Email is off')
  const versions = details(page).locator('section', { has: page.getByRole('heading', { name: 'Versions' }) })
  await expect(versions.getByText('Version 1')).toBeVisible()
  await expect(versions).toContainText('accepted')
})

test('a receipt still being made updates by itself; a failed one can be made again', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { world, calls } = await setup(page, { tune: w => { w.jobs.get(`${Q.accepted}:1`)!.state = 'failed'; delete w.jobs.get(`${Q.accepted}:1`)!.receipt_sha256 } })
  await full(page, Q.accepted)
  const receipt = details(page).locator('section', { has: page.getByRole('heading', { name: 'Acceptance receipt' }) })
  await expect(receipt).toContainText('Making the receipt failed after 1 try. The acceptance itself is recorded and safe.')
  await receipt.getByRole('button', { name: 'Make it again' }).click()
  await expect.poll(() => calls.some(c => c.path.endsWith('/confirmation/retry'))).toBe(true)
  await expect(receipt).toContainText('The receipt is being made.')
  Object.assign(world.jobs.get(`${Q.accepted}:1`)!, { state: 'ready', receipt_sha256: 'fe'.repeat(32) })
  await expect(receipt.locator('.pill')).toHaveText('Ready', { timeout: 9000 })
})

test('revising an issued quote opens a new draft; older versions stay readable', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await setup(page)
  await full(page, Q.issued)
  await details(page).getByRole('button', { name: 'Revise as version 2…' }).click()
  await page.getByRole('dialog', { name: 'Revise as version 2?' }).getByRole('button', { name: 'Revise' }).click()
  await expect.poll(() => calls.find(c => c.path.endsWith('/draft/branch'))?.body).toEqual({ expected_quote_revision: 2, expected_version: 1, expected_content_sha256: expect.stringMatching(/^2{8}/) })
  await expect(page.locator('.state-chip').first()).toContainText('Revising')
  await expect(page.getByRole('button', { name: 'Save draft' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Format panel' })).toBeVisible()
  await page.getByRole('button', { name: 'Details' }).click()
  await expect(details(page)).toContainText('Version 1 stays exactly as it was issued')
  await details(page).getByRole('button', { name: 'View' }).click()
  await expect(page.getByText('You are reading version 1 as it was issued. It cannot change.')).toBeVisible()
  await expect(page.getByRole('textbox', { name: 'Angebotstitel' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Back to the draft' }).first().click()
  await expect(page.getByRole('textbox', { name: 'Angebotstitel' })).toBeVisible()
  await expect(page.getByText('You are reading version 1')).toHaveCount(0)
})

test('people on the quote: avatars in their colour, a list of who does what', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await setup(page, { tune: w => { w.presence = presenceOf([{ id: MIRA.id, name: MIRA.name, mode: 'editing', section: SECTIONS[1] }, { id: '33333333-3333-4333-8333-333333333333', name: 'Jonas Berger', mode: 'viewing' }]) } })
  await full(page, Q.draft)
  const people = page.getByRole('button', { name: '2 other people here, 1 editing' })
  await expect(people).toBeVisible()
  await people.click()
  const list = page.getByRole('dialog', { name: 'People on this quote' })
  await expect(list.getByText('Mira Kovač')).toBeVisible()
  await expect(list.getByText('Editing')).toBeVisible()
  await expect(list.getByText('Viewing')).toBeVisible()
  await expect(list).toContainText('Presence is a courtesy, not a lock.')
  // Her section is outlined on the paper with her name.
  await page.keyboard.press('Escape')
  await page.locator(`[data-section-id="${SECTIONS[1]}"]`).first().scrollIntoViewIfNeeded()
  await expect(page.locator('.quote-presence-overlays .label')).toHaveText('Mira Kovač · in this section')
})

test('a save that meets a newer draft: review each overlap, keep mine, save the result', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await setup(page, { conflictNext: { quote: Q.draft, apply: doc => { doc.title = 'Portal-Relaunch (Mira)' } } })
  await full(page, Q.draft)
  const title = page.getByRole('textbox', { name: 'Angebotstitel' })
  await title.click()
  await page.keyboard.press('End')
  await page.keyboard.type(' – neu')
  await page.getByRole('button', { name: 'Save draft' }).click()
  await expect(page.getByText(/changed a place you changed too/)).toBeVisible()
  await page.getByRole('button', { name: 'Review changes' }).click()
  const dialog = page.getByRole('dialog', { name: 'Choose which changes stay' })
  await expect(dialog.getByRole('group', { name: 'Title' })).toBeVisible()
  await expect(dialog.getByText('“Relaunch des Kundenportals – neu”')).toBeVisible()
  await expect(dialog.getByText('“Portal-Relaunch (Mira)”')).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Save the result' })).toBeDisabled()
  await dialog.getByLabel(/^Mine/).check()
  await dialog.getByRole('button', { name: 'Save the result' }).click()
  await expect.poll(() => {
    const saves = calls.filter(c => c.method === 'PATCH' && c.path.endsWith('/draft'))
    return (saves.at(-1)?.body as { document?: { title: string } } | undefined)?.document?.title
  }).toBe('Relaunch des Kundenportals – neu')
  await expect(dialog).toBeHidden()
})

test('archive and duplicate live in the quote’s menu', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const { calls } = await setup(page)
  await full(page, Q.second)
  await page.getByRole('button', { name: 'More actions' }).click()
  // Archiving asks nothing: the toast undoes it (QL1).
  await page.getByRole('menuitem', { name: 'Archive' }).click()
  await expect.poll(() => calls.find(c => c.method === 'PATCH' && c.path.endsWith('/visibility'))?.body).toEqual({ expected_revision: 2, archived: true })
  await expect(page.locator('.state-chip', { hasText: 'Archived' })).toBeVisible()
  await expect(page.locator('.toast').last().getByRole('button', { name: 'Undo' })).toBeVisible()
  await page.getByRole('button', { name: 'More actions' }).click()
  await page.getByRole('menuitem', { name: 'Duplicate as a new quote' }).click()
  await expect.poll(() => calls.find(c => c.path.endsWith('/duplicate'))?.body).toEqual({ expected_revision: 3 })
  await expect(page).toHaveURL(/\/business\/quotes\/c0ffee00-0000-4000-8000-0000000001\d\d$/)
})

test('files on a quote use the attachment strip', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const work = fixtures()
  work.attachments[Q.draft] = [{ id: 'att-brief', node_id: Q.draft, sha256: 'a'.repeat(64), name: 'briefing.pdf', content_type: 'application/pdf', size: 48_000, width: null, height: null, caption: '', position: '1024', created_by: '11111111-1111-4111-8111-111111111111', created_at: new Date().toISOString(), updated_at: new Date().toISOString(), deleted_at: null }] as never
  await mockWork(page, work)
  await mockCRM(page, crmData())
  await mockQuotes(page, quoteWorld())
  await full(page, Q.draft)
  await page.getByRole('button', { name: 'Details' }).click()
  const files = details(page).locator('section', { has: page.getByRole('heading', { name: 'Files' }) })
  await expect(files).toContainText('briefing.pdf')
  await expect(files.getByRole('button', { name: 'Add' })).toBeVisible()
})

test('a member reads issued quotes and cannot issue or share', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await setup(page, { role: 'member' })
  await full(page, Q.issued)
  await expect(details(page)).toContainText('An admin shares quotes with customers through a link.')
  await expect(details(page).getByRole('button', { name: /Revise/ })).toHaveCount(0)
})

test('a second tenant browser opens its own draft while the first tenant keeps its edits', async ({ browser }) => {
  const a = await browser.newContext(), b = await browser.newContext()
  try {
    const first = await a.newPage(), second = await b.newPage()
    await setup(first)
    await setup(second, { tune: world => { world.drafts.get(Q.draft)!.document.title = 'Tenant B quote' } })
    await second.route('**/api/me', route => route.fulfill({ json: { principal: { id: 'bbbbbbbb-0000-4000-8000-bbbbbbbbbbbb', name: 'Tenant B editor', kind: 'person', roles: ['admin'] }, tenant: { id: 'tenant-b', name: 'Tenant B' } } }))
    await Promise.all([full(first, Q.draft), full(second, Q.draft)])
    await expect(first.getByRole('textbox', { name: 'Angebotstitel' })).toHaveText('Relaunch des Kundenportals')
    await expect(second.getByRole('textbox', { name: 'Angebotstitel' })).toHaveText('Tenant B quote')
    await first.getByRole('textbox', { name: 'Angebotstitel' }).fill('Tenant A edit')
    await expect(second.getByRole('textbox', { name: 'Angebotstitel' })).toHaveText('Tenant B quote')
  } finally { await a.close(); await b.close() }
})

test('a representative quote with 20 sections, 100 positions and 12 collaborators opens within ten seconds', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await setup(page, { tune: world => {
    const draft = world.drafts.get(Q.draft)!
    const section = draft.document.sections[0]!
    draft.document.sections = Array.from({ length: 20 }, (_, i) => ({ ...structuredClone(section), id: `11111111-1111-4111-8111-${String(i + 1).padStart(12, '0')}`, heading: `Section ${i + 1}`, nodes: [{ ...structuredClone(section.nodes[0]!), id: `22222222-2222-4222-8222-${String(i + 1).padStart(12, '0')}` }] }))
    const position = draft.document.positions[0]!
    draft.document.positions = Array.from({ length: 100 }, (_, i) => ({ ...structuredClone(position), id: `33333333-3333-4333-8333-${String(i + 1).padStart(12, '0')}`, short_text: `Position ${i + 1}` }))
    world.presence = Array.from({ length: 12 }, (_, i) => ({ session_id: `44444444-4444-4444-8444-${String(i + 1).padStart(12, '0')}`, principal_id: `55555555-5555-4555-8555-${String(i + 1).padStart(12, '0')}`, name: `Editor ${i + 1}`, mode: 'editing', observed_revision: 3, expires_at: '2099-01-01', anchor: { section_id: draft.document.sections[i]!.id, observed_revision: 3, fidelity: 'section' } }))
  } })
  const started = Date.now()
  await full(page, Q.draft)
  expect(Date.now() - started).toBeLessThan(10_000)
  await expect(page.locator('.quote-page .quote-positions tbody')).toHaveCount(100)
  await expect(page.getByRole('button', { name: '12 other people here, 12 editing' })).toBeVisible()
  await expect(page.locator('.quote-presence-overlays .mark')).toHaveCount(1)
})

test('phones: the details come up as a sheet, nothing is cut at 390', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await setup(page)
  await full(page, Q.accepted)
  await page.getByRole('button', { name: 'Details' }).click()
  await expect(details(page)).toBeVisible()
  const cut = await page.evaluate(() => [...document.querySelectorAll<HTMLElement>('.titlebar *, .quote-inspector-slot *')].filter(el => {
    const r = el.getBoundingClientRect(), c = getComputedStyle(el)
    if (r.width <= 1 || r.height <= 1 || c.display === 'none' || el.closest('svg')) return false
    return r.left < -0.5 || r.right > innerWidth + 0.5 || ((c.overflowX === 'hidden' || c.overflowX === 'auto') && el.scrollWidth > el.clientWidth + 1 && c.textOverflow !== 'ellipsis')
  }).map(el => `${el.tagName}.${el.className}`))
  expect(cut).toEqual([])
  await details(page).getByRole('button', { name: 'Close details' }).click()
  await expect(details(page)).toHaveCount(0)
})

for (const colorScheme of ['light', 'dark'] as const) {
  test(`axe: the workspace with details, the link card and the review dialog in ${colorScheme}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme })
    await page.setViewportSize({ width: 1440, height: 900 })
    await setup(page, { tune: w => { w.presence = presenceOf([{ id: MIRA.id, name: MIRA.name, mode: 'editing' }]) } })
    await full(page, Q.accepted)
    await axe(page)
    await page.goto(`/business/quotes?quote=${Q.issued}`)
    await expect(page.locator('.quote-dock .quote-document')).toHaveAttribute('data-quote-ready', 'true')
    await page.locator('.quote-dock').getByRole('button', { name: 'Details' }).click()
    await axe(page)
  })
}
