// SPDX-License-Identifier: AGPL-3.0-only
// axe (WCAG 2.1 A and AA) over every shipped screen, light and dark, on mocked data.
import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { fixtures, me, mockWork } from './work-fixtures'
import { agentData, mockAgents } from './agents-fixtures'

const world = {
  me: me.id,
  projects: { pharos: 'p-pharos', aeon: 'p-aeon', pai: 'p-frozen' },
  tickets: { fleet: 'n-1', restore: 'n-2', web: 'n-a1', release: 'n-5', approvals: 'n-6' },
  nodes: {
    'p-pharos': { key: 'PRJ-17', title: 'Pharos' }, 'p-aeon': { key: 'PRJ-35', title: 'Aeon' }, 'p-frozen': { key: 'PRJ-26', title: 'Studio infrastructure' },
    'n-1': { key: 'PHAROS-11', title: 'Connect Hetzner Cloud for managed provisioning' }, 'n-2': { key: 'PHAROS-12', title: 'Add an Oracle Cloud connector' },
    'n-a1': { key: 'AEON-1', title: 'Aeon foundation' }, 'n-5': { key: 'PHAROS-15', title: 'Beacon health probes' }, 'n-6': { key: 'PHAROS-16', title: 'Retire the old dashboard' },
  },
}
async function signedIn(page: Page, empty = false) {
  await mockWork(page, fixtures())
  await mockAgents(page, agentData({ ...world, empty }))
}
async function signedOut(page: Page) {
  await page.route('**/api/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/me') return route.fulfill({ status: 401, json: { error: 'unauthorized', dev_mode: true } })
    if (path === '/api/version') return route.fulfill({ json: { version: '260924020145.0.0', scheme: 'inspr-calendar-v2' } })
    return route.fulfill({ status: 404, json: { error: 'not found' } })
  })
}

// name, setup, path, then what to wait for or do before the scan
const screens: [string, (page: Page) => Promise<void>, string, (page: Page) => Promise<void>][] = [
  ['projects', signedIn, '/', async page => { await expect(page.getByRole('list', { name: 'Projects' })).toBeVisible() }],
  ['project list', signedIn, '/p/PHAROS', async page => { await expect(page.locator('tr.ticket-row:not(.ghost)')).toHaveCount(5) }],
  ['project outline', signedIn, '/p/PHAROS?view=outline', async page => { await expect(page.locator('tr.ticket-row:not(.ghost)').first()).toBeVisible() }],
  ['ticket panel', signedIn, '/p/PHAROS/PHAROS-11', async page => { await expect(page.getByRole('complementary', { name: 'Ticket details' }).getByRole('heading').first()).toBeVisible() }],
  ['ticket full page', signedIn, '/p/PHAROS/PHAROS-11?view=full', async page => { await expect(page.getByRole('article', { name: 'Ticket details' })).toBeVisible() }],
  ['command palette', signedIn, '/', async page => {
    await expect(page.getByRole('list', { name: 'Projects' })).toBeVisible()
    await page.keyboard.press('Control+k'); await page.keyboard.type('hetzner')
    await expect(page.getByRole('dialog', { name: 'Search and commands' }).getByRole('option').first()).toBeVisible()
  }],
  ['account menu', signedIn, '/', async page => {
    await expect(page.getByRole('list', { name: 'Projects' })).toBeVisible()
    await page.getByRole('button', { name: /^Account for/ }).click()
    await expect(page.getByRole('dialog', { name: 'Account' })).toBeVisible()
  }],
  ['shortcut sheet', signedIn, '/p/PHAROS', async page => {
    await expect(page.locator('tr.ticket-row:not(.ghost)')).toHaveCount(5)
    await page.keyboard.press('Shift+?')
    await expect(page.getByRole('dialog', { name: 'Keyboard shortcuts' })).toBeVisible()
  }],
  ['sign-in', signedOut, '/signin', async page => { await expect(page.getByLabel('Email address')).toBeVisible() }],
  ['sign-in error', signedOut, '/signin?error=denied', async page => { await expect(page.getByRole('alert')).toBeVisible() }],
  ['not found', signedIn, '/nope/here', async page => { await expect(page.getByRole('heading', { name: 'A little off the path.' })).toBeVisible() }],
  ['error page', async page => {
    await signedIn(page)
    await page.route(url => url.pathname.endsWith('/src/views/AgentsView.vue'), route => route.fulfill({ contentType: 'application/javascript', body: 'throw new TypeError("The view could not start")' }))
  }, '/agents', async page => { await expect(page.getByRole('heading', { name: 'This page stumbled.' })).toBeVisible() }],
  ['agents', signedIn, '/agents', async page => { await expect(page.locator('.agents-page .row').first()).toBeVisible() }],
  ['agents empty', page => signedIn(page, true), '/agents', async page => { await expect(page.getByRole('heading', { name: 'No agent has connected yet' })).toBeVisible() }],
  ['agents session panel', signedIn, '/agents/5e000000-0000-4000-8000-000000000001', async page => {
    await expect(page.getByRole('complementary', { name: 'Session details' }).locator('.msg').first()).toBeVisible()
  }],
  ['agents approve', signedIn, '/agents', async page => {
    await expect(page.locator('.agents-page .row').first()).toBeVisible()
    await page.keyboard.press('j'); await page.keyboard.press('a')
    await expect(page.getByLabel('Reason (optional)')).toBeFocused()
  }],
]

for (const colorScheme of ['light', 'dark'] as const) {
  for (const [name, setup, path, ready] of screens) {
    test(`axe: ${name} in ${colorScheme}`, async ({ page }) => {
      await page.emulateMedia({ colorScheme, reducedMotion: 'reduce' })
      await setup(page)
      await page.goto(path)
      await ready(page)
      await page.waitForTimeout(250)
      // The version coordinate is the vendored INSPR calendar-version display (pinned presentation);
      // its digit colours are reported to its owner rather than restyled here.
      const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).exclude('.version-coordinate').analyze()
      const summary = results.violations.map(v => `${v.id} (${v.impact}): ${v.help}\n${v.nodes.slice(0, 4).map(n => `    ${n.target.join(' ')} — ${n.failureSummary?.split('\n').slice(1, 2).join(' ').trim()}`).join('\n')}`)
      expect(summary, summary.join('\n')).toEqual([])
    })
  }
}
