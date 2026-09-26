// SPDX-License-Identifier: AGPL-3.0-only
// Ticket keys in the release history (AEON-173): real links to the ticket's own
// page, a plain click opens the ticket in the app's side panel beside the
// history, keys this workspace does not have stay plain text.
import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { fixtures, mockWork, watchErrors } from './work-fixtures'
import { mockReleases, releaseHistory } from './releases-fixtures'

const sheet = (page: Page) => page.getByRole('dialog', { name: 'PAIMOS AEON releases' })
const panel = (page: Page) => sheet(page).getByRole('complementary', { name: 'Ticket details' })
const chips = (page: Page) => sheet(page).locator('.detail .tickets')
const TITLE = 'Connect Hetzner Cloud for managed provisioning'

// The second release names AEON-74 (no such ticket here) and PHAROS-11 (a ticket here).
async function open(page: Page) {
  const history = releaseHistory()
  const calls = await mockWork(page, fixtures())
  await mockReleases(page, history)
  await page.goto(`/releases/${history.releases[1].version}`)
  await expect(chips(page).getByRole('link', { name: `PHAROS-11: ${TITLE}` })).toBeVisible()
  return { history, calls }
}

test('a ticket key opens the ticket beside the history, with one lookup for the release', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  const errors = watchErrors(page)
  const { history, calls } = await open(page)
  const chip = chips(page).getByRole('link', { name: `PHAROS-11: ${TITLE}` })
  await expect(chip).toHaveAttribute('href', '/p/PHAROS/PHAROS-11')
  // The key in the list of changes is the same link.
  await expect(sheet(page).locator('.changes').getByRole('link', { name: `PHAROS-11: ${TITLE}` })).toHaveAttribute('href', '/p/PHAROS/PHAROS-11')
  const lookups = calls.filter(call => call.path === '/api/nodes/lookup' && call.query.has('keys'))
  expect(lookups).toHaveLength(1)
  expect(lookups[0].query.get('keys')!.split(',').sort()).toEqual(['AEON-74', 'PHAROS-11'])

  await chip.click()
  await expect(panel(page).getByRole('heading', { name: TITLE })).toBeVisible()
  await expect(panel(page).getByRole('button', { name: /Status: In progress/ })).toBeVisible()
  await expect(chip).toHaveAttribute('aria-current', 'true')
  // The history stays: same address, the list and the release beside the ticket.
  await expect(page).toHaveURL(`/releases/${history.releases[1].version}`)
  await expect(sheet(page).getByRole('listbox', { name: 'Releases, newest first' })).toBeVisible()
  const detail = (await sheet(page).locator('.detail').boundingBox())!
  const aside = (await panel(page).boundingBox())!
  expect(aside.x).toBeGreaterThanOrEqual(detail.x + detail.width)
  expect(aside.x + aside.width).toBeLessThanOrEqual(1440)

  // The panel's menus work inside the history (they live in its top layer).
  await panel(page).getByRole('button', { name: 'More actions' }).click()
  await expect(page.getByRole('menu', { name: 'Actions for PHAROS-11' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menu', { name: 'Actions for PHAROS-11' })).toHaveCount(0)
  await expect(panel(page)).toBeVisible()

  await panel(page).getByRole('button', { name: 'Close ticket details' }).click()
  await expect(panel(page)).toHaveCount(0)
  await expect(chip).toBeFocused()
  await expect(chip).not.toHaveAttribute('aria-current', 'true')
  expect(errors).toEqual([])
})

test('a modified click keeps the link: the ticket opens in a new tab, not the panel', async ({ page }) => {
  await open(page)
  const chip = chips(page).getByRole('link', { name: `PHAROS-11: ${TITLE}` })
  const [tab] = await Promise.all([page.context().waitForEvent('page'), chip.click({ modifiers: ['ControlOrMeta'] })])
  await tab.waitForURL('**/p/PHAROS/PHAROS-11')
  await tab.close()
  await expect(panel(page)).toHaveCount(0)
  await expect(sheet(page)).toBeVisible()
})

test('keys this workspace does not have stay plain text', async ({ page }) => {
  const { history } = await open(page)
  const plain = chips(page).getByText('AEON-74', { exact: true })
  await expect(plain).toHaveJSProperty('tagName', 'SPAN')
  await expect(plain).toHaveAttribute('data-tip', 'AEON-74 is not a ticket in this AEON workspace')
  await expect(sheet(page).locator('.changes').getByText('AEON-74', { exact: true }).first()).toHaveJSProperty('tagName', 'SPAN')
  // Another tracker's key (classic Paimos) likewise.
  await page.goto(`/releases/${history.releases[3].version}`)
  await expect(chips(page).getByText('PAI-1057', { exact: true })).toHaveJSProperty('tagName', 'SPAN')
  await expect(sheet(page).locator('.detail').getByRole('link', { name: /PAI-1057/ })).toHaveCount(0)
})

test('the keyboard opens the panel, Esc closes it and returns focus to the key', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  await open(page)
  const chip = chips(page).getByRole('link', { name: `PHAROS-11: ${TITLE}` })
  await chip.focus()
  await page.keyboard.press('Enter')
  await expect(panel(page).getByRole('heading', { name: TITLE })).toBeVisible()
  await expect(panel(page)).toBeFocused()
  // The history's own keys stay out of the panel: j does not move the release.
  const selected = sheet(page).getByRole('option', { selected: true })
  const before = await selected.getAttribute('id')
  await page.keyboard.press('j')
  await expect(selected).toHaveAttribute('id', before!)
  await page.keyboard.press('Escape')
  await expect(panel(page)).toHaveCount(0)
  await expect(chip).toBeFocused()
  await expect(sheet(page)).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(sheet(page)).toHaveCount(0)
})

test('on a phone the ticket is a full-screen sheet over the history', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await open(page)
  const chip = chips(page).getByRole('link', { name: `PHAROS-11: ${TITLE}` })
  await chip.click()
  await expect(panel(page).getByRole('heading', { name: TITLE })).toBeVisible()
  const box = (await panel(page).boundingBox())!
  expect(box).toEqual({ x: 0, y: 0, width: 390, height: 844 })
  await panel(page).getByRole('button', { name: 'Close ticket details' }).click()
  await expect(panel(page)).toHaveCount(0)
  await expect(chip).toBeFocused()
})

for (const colorScheme of ['light', 'dark'] as const) {
  for (const viewport of [{ width: 1440, height: 900 }, { width: 390, height: 844 }]) {
    test(`axe: a ticket open beside the history ${viewport.width} ${colorScheme}`, async ({ page }) => {
      await page.setViewportSize(viewport)
      await page.emulateMedia({ colorScheme, reducedMotion: 'reduce' })
      await open(page)
      if (viewport.width > 600) await chips(page).getByRole('link', { name: `PHAROS-11: ${TITLE}` }).hover()
      await sheet(page).locator('.changes').getByRole('link', { name: `PHAROS-11: ${TITLE}` }).click()
      await expect(panel(page).getByRole('heading', { name: TITLE })).toBeVisible()
      await page.waitForTimeout(250)
      const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).exclude('.calendar-version').analyze()
      const summary = results.violations.map(v => `${v.id} (${v.impact}): ${v.help}\n${v.nodes.slice(0, 4).map(n => `    ${n.target.join(' ')} — ${n.failureSummary?.split('\n').slice(1, 2).join(' ').trim()}`).join('\n')}`)
      expect(summary, summary.join('\n')).toEqual([])
    })
  }
}
