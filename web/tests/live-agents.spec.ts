// SPDX-License-Identifier: AGPL-3.0-only
// AEON-184: agents working on a project right now come alive on its card and
// its list row, say who works on what, and keep the page still otherwise.
// LIVE_SHOTS=<dir> also writes the design review screenshots there.
import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { mkdirSync } from 'node:fs'
import { fixtures, liveAgent, mockWork, watchErrors, type Fixtures, type LiveAgentMock } from './work-fixtures'

const HAUSV = '33333333-3333-4333-8333-333333333333'
const CAMY = '44444444-4444-4444-8444-444444444444'
const COORD = '55555555-5555-4555-8555-555555555555'
const OPS = '66666666-6666-4666-8666-666666666666'

test.beforeEach(async ({ page }) => { await page.clock.setSystemTime(new Date('2026-09-23T12:00:00Z')) })

// The fixture's four projects plus a few more, so a card grid fills its rows.
function world(live: 'busy' | 'none' = 'busy') {
  const data = fixtures()
  const ago = (hours: number) => new Date(Date.parse('2026-09-23T12:00:00Z') - hours * 3_600_000).toISOString()
  data.projects.push(
    { id: 'p-hausv', key: 'PRJ-40', title: 'Hausverwaltung', state: 'active', classic: 'HAUSV', description: 'Tenants, contracts and the yearly statements.', last: ago(0.2) },
    { id: 'p-janus', key: 'PRJ-41', title: 'Janus', state: 'active', classic: 'JANUS', description: 'Identity and access for every INSPR service.', last: ago(5) },
    { id: 'p-ops', key: 'PRJ-42', title: 'Operations', state: 'active', classic: 'OPS', description: 'Hosts, backups and the nightly checks.', last: ago(0.5) },
    { id: 'p-site', key: 'PRJ-43', title: 'inspr.at', state: 'active', classic: 'INSPR', description: 'The public site and its release notes.', last: ago(50) },
  )
  let n = 0
  for (const [project, key, states] of [
    ['p-hausv', 'HAUSV', ['in_progress', 'backlog', 'done', 'done', 'new']], ['p-janus', 'JANUS', ['done', 'backlog']],
    ['p-ops', 'OPS', ['in_progress', 'qa', 'done', 'backlog']], ['p-site', 'INSPR', ['done', 'done', 'done']],
    ['p-aeon', 'AEON', ['in_progress', 'done', 'done']],
  ] as const) {
    for (const state of states) data.nodes.push({ id: `n-x${++n}`, key: `${key}-${800 + n}`, kind_slug: 'ticket', title: `Ticket ${n}`, state, project, body: '', fields: {}, parent_id: project, created_at: ago(200), updated_at: ago(3) })
  }
  if (live === 'busy') data.live.push(...busyAgents())
  data.preferences.projects = { view: 'cards' }
  return data
}
function busyAgents(): LiveAgentMock[] {
  return [
    liveAgent({ project_id: 'p-hausv', session_id: 's-hausv', principal_id: HAUSV, name: 'hausv', ticket: { id: 'n-h1', key: 'HAUSV-887', title: 'Yearly statement preview for tenants' } }, 16),
    liveAgent({ project_id: 'p-aeon', session_id: 's-coord', principal_id: COORD, name: 'aeon-coordinator', role: 'coordinator', harness: 'claude' }, 140),
    liveAgent({ project_id: 'p-aeon', session_id: 's-camy', principal_id: CAMY, name: 'camy', harness: 'codex', ticket: { id: 'n-a184', key: 'AEON-184', title: 'Live agents on project cards and rows' } }, 4),
    liveAgent({ project_id: 'p-janus', session_id: 's-janus', principal_id: '77777777-7777-4777-8777-777777777777', name: 'janus-session', phase: 'starting', activity: 'unknown' }, 0.5),
    liveAgent({ project_id: 'p-ops', session_id: 's-ops', principal_id: OPS, name: 'ops', ticket: { id: 'n-o1', key: 'OPS-212', title: 'Rotate the backup keys on csb1' } }, 42),
    liveAgent({ project_id: 'p-ops', session_id: 's-ops2', principal_id: '88888888-8888-4888-8888-888888888888', name: 'pharos-session', harness: 'claude', ticket: { id: 'n-o2', key: 'OPS-219', title: 'Nightly check for hsb2' } }, 8),
    liveAgent({ project_id: 'p-ops', session_id: 's-ops3', principal_id: '99999999-9999-4999-8999-999999999999', name: 'grok-scout', harness: 'grok', ticket: { id: 'n-o3', key: 'OPS-220', title: 'Disk usage report' } }, 2),
  ]
}
const card = (page: Page, key: string) => page.locator(`.card[data-project-id="${key}"]`)
const row = (page: Page, id: string) => page.locator(`.project-item[data-project-id="${id}"]`)
async function cards(page: Page, data: Fixtures, options: Parameters<typeof mockWork>[2] = {}) {
  const calls = await mockWork(page, data, options)
  await page.goto('/')
  await expect(page.locator('.card').first()).toBeVisible()
  return calls
}

test('a card comes alive while its agents work, says who and on what, and idle cards stay still', async ({ page }) => {
  const errors = watchErrors(page)
  await cards(page, world())
  const hausv = card(page, 'p-hausv')
  const chip = hausv.getByRole('button', { name: '1 agent working: hausv on HAUSV-887. Who works on what' })
  await expect(chip).toBeVisible()
  await expect(chip).toContainText('hausv')
  await expect(chip).toContainText('HAUSV-887')
  await expect(chip).toContainText('16m')
  // The card's own link says it too, for whoever walks the cards.
  await expect(hausv.getByRole('link')).toHaveAccessibleName(/HAUSV Hausverwaltung, .*1 agent working: hausv on HAUSV-887/)
  // The lead is the agent on a ticket; the coordinator follows.
  await expect(card(page, 'p-aeon').getByRole('button', { name: /^2 agents working: camy on AEON-184, aeon-coordinator\./ })).toBeVisible()
  await expect(card(page, 'p-aeon').locator('.live-chip .more')).toHaveCount(0)
  await expect(card(page, 'p-ops').locator('.live-bot')).toHaveCount(3)
  // Idle projects show no chip and no robot.
  for (const id of ['p-pharos', 'p-frozen', 'p-site']) await expect(card(page, id).locator('.live')).toHaveCount(0)
  // Recently active people stay in the card, hidden under the chip.
  await expect(card(page, 'p-pharos').locator('.people')).toBeVisible()
  expect(errors).toEqual([])
})

test('the chip never moves the card: the same box with and without agents', async ({ page }) => {
  const data = world('none')
  await cards(page, data)
  const before = await card(page, 'p-hausv').boundingBox()
  const footBefore = await card(page, 'p-hausv').locator('.card-foot').boundingBox()
  data.live.push(...busyAgents())
  await page.clock.fastForward(21_000)
  await expect(card(page, 'p-hausv').locator('.live-chip')).toBeVisible()
  expect(await card(page, 'p-hausv').boundingBox()).toEqual(before)
  expect(await card(page, 'p-hausv').locator('.card-foot').boundingBox()).toEqual(footBefore)
  // The chip sits on the footer line, inside the card.
  const chip = (await card(page, 'p-hausv').locator('.live-chip').boundingBox())!
  expect(chip.y).toBeGreaterThan(footBefore!.y)
  expect(chip.y + chip.height).toBeLessThanOrEqual(before!.y + before!.height)
  expect(chip.x + chip.width).toBeLessThan(before!.x + before!.width)
})

test('hover and focus show who works on what; an agent opens its session', async ({ page }) => {
  await cards(page, world())
  const chip = card(page, 'p-aeon').locator('.live-chip')
  await chip.hover()
  const pop = page.getByRole('dialog', { name: 'Agents working on Aeon' })
  await expect(pop).toBeVisible()
  await expect(pop).toContainText('2 agents working')
  const camy = pop.getByRole('link', { name: 'camy, Codex, working for 4m. Open the session' })
  await expect(camy).toHaveAttribute('href', '/agents/s-camy')
  await expect(pop.getByRole('link', { name: /AEON-184/ })).toHaveAttribute('href', '/p/AEON/AEON-184')
  await expect(pop.getByRole('link', { name: /AEON-184/ })).toContainText('Live agents on project cards and rows')
  await expect(pop.getByRole('link', { name: /^aeon-coordinator, Claude, working for 2h 20m/ })).toBeVisible()
  await expect(pop.getByText('No ticket bound')).toBeVisible()
  // Leaving closes it; the keyboard opens it on focus and walks into it with Enter.
  await page.mouse.move(5, 5)
  await expect(pop).toHaveCount(0)
  await chip.focus()
  await page.keyboard.press('Shift+Tab')
  await page.keyboard.press('Tab')
  await expect(chip).toBeFocused()
  await expect(pop).toBeVisible()
  await page.keyboard.press('Enter')
  await expect(camy).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(pop).toHaveCount(0)
  await expect(chip).toBeFocused()
  await chip.click()
  await expect(pop).toBeVisible()
  await camy.click()
  await expect(page).toHaveURL('/agents/s-camy')
})

test('without permission to know the agent the chip still says an agent works', async ({ page }) => {
  const data = world('none')
  data.live.push(liveAgent({ project_id: 'p-hausv', harness: 'claude', ticket: { id: 'n-h1', key: 'HAUSV-887', title: 'Yearly statement preview' } }, 3))
  await cards(page, data)
  const chip = card(page, 'p-hausv').getByRole('button', { name: '1 agent working: Claude agent on HAUSV-887. Who works on what' })
  await expect(chip).toContainText('Claude agent')
  await chip.click()
  const pop = page.getByRole('dialog', { name: 'Agents working on Hausverwaltung' })
  // No session to open: the agent is a line, not a link.
  await expect(pop.getByRole('link')).toHaveCount(1)
  await expect(pop.getByRole('link')).toHaveAttribute('href', '/p/HAUSV/HAUSV-887')
})

test('list rows carry a compact live chip: robots and the ticket key', async ({ page }) => {
  const data = world()
  data.preferences.projects = { view: 'list' }
  await mockWork(page, data)
  await page.goto('/')
  const hausv = row(page, 'p-hausv')
  await expect(hausv.getByRole('button', { name: /^1 agent working: hausv on HAUSV-887/ })).toContainText('HAUSV-887')
  await expect(row(page, 'p-ops').locator('.live-bot')).toHaveCount(2)
  await expect(row(page, 'p-ops').locator('.live-chip .more')).toHaveText('+1')
  await expect(row(page, 'p-pharos').locator('.live')).toHaveCount(0)
  // The chip ends where the project column ends, before the counts.
  const chip = (await hausv.locator('.live-chip').boundingBox())!
  const open = (await hausv.locator('.stat').first().boundingBox())!
  expect(chip.x + chip.width).toBeLessThanOrEqual(open.x)
  const text = (await hausv.locator('.project-text').boundingBox())!
  expect(chip.y).toBeGreaterThanOrEqual(text.y - 6)
})

test('starting, stopping and stale agents: polls every 20 seconds and says what changed', async ({ page }) => {
  const data = world('none')
  const calls = await cards(page, data)
  const reads = () => calls.filter(c => c.path === '/api/harness-sessions/live').length
  await expect.poll(reads).toBe(1)
  await expect(page.locator('.live')).toHaveCount(0)
  data.live.push(liveAgent({ project_id: 'p-janus', session_id: 's-j', principal_id: CAMY, name: 'camy', ticket: { id: 'n-j', key: 'JANUS-9', title: 'Sessions' } }, 1))
  await page.clock.fastForward(20_500)
  await expect.poll(reads).toBe(2)
  await expect(card(page, 'p-janus').locator('.live-chip')).toBeVisible()
  await expect(page.locator('[aria-live="polite"].sr-only')).toHaveText('camy started working on Janus.')
  data.live.splice(0)
  await page.clock.fastForward(20_500)
  await expect(card(page, 'p-janus').locator('.live')).toHaveCount(0)
  await expect(page.locator('[aria-live="polite"].sr-only')).toHaveText('No agent is working on Janus any more.')
  // A heartbeat older than two minutes on the server's clock no longer counts.
  data.live.push(liveAgent({ project_id: 'p-site', name: 'late', heartbeat_at: new Date(Date.parse('2026-09-23T12:00:00Z') - 150_000).toISOString() }))
  await page.clock.fastForward(20_500)
  await expect.poll(reads).toBe(4)
  await expect(card(page, 'p-site').locator('.live')).toHaveCount(0)
  // A hidden tab asks nothing and catches up when shown again.
  await page.evaluate(() => { Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true }); document.dispatchEvent(new Event('visibilitychange')) })
  await page.clock.fastForward(65_000)
  expect(reads()).toBe(4)
  await page.evaluate(() => { Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true }); document.dispatchEvent(new Event('visibilitychange')) })
  await expect.poll(reads).toBe(5)
})

test('a server without the read, or a person without access, gets a still page', async ({ page }) => {
  const errors = watchErrors(page)
  const calls = await cards(page, world(), { liveStatus: 403 })
  await expect(page.locator('.card')).toHaveCount(7)
  await page.clock.fastForward(45_000)
  expect(calls.filter(c => c.path === '/api/harness-sessions/live')).toHaveLength(1)
  await expect(page.locator('.live')).toHaveCount(0)
  expect(errors).toEqual([])
})

test.describe('motion', () => {
  test.use({ reducedMotion: 'no-preference' })
  test('the robots move on the compositor and out of step', async ({ page }) => {
    await cards(page, world())
    const bots = card(page, 'p-ops').locator('.live-bot')
    const names = await bots.first().locator('.bob').evaluate(el => getComputedStyle(el).animationName)
    expect(names).toMatch(/^bot-bob/)
    const lags = await bots.evaluateAll(els => els.map(el => getComputedStyle(el).getPropertyValue('--lag')))
    expect(new Set(lags).size).toBe(3)
    // Only transforms and opacity animate, in the chips and the cards' auras.
    const properties = await page.evaluate(() => [...new Set(document.getAnimations().flatMap(a => {
      const effect = a.effect as KeyframeEffect | null
      const target = effect?.target as Element | null
      if (!effect || !target?.closest('.live, li.card.live')) return []
      return effect.getKeyframes().flatMap(k => Object.keys(k).filter(p => !['offset', 'easing', 'composite', 'computedOffset'].includes(p)))
    }))].sort())
    expect(properties.length).toBeGreaterThan(0)
    expect(properties.every(p => ['opacity', 'transform', 'translate'].includes(p)), properties.join(',')).toBe(true)
  })
})

test('reduced motion keeps the robots still and the chip clearly working', async ({ page }) => {
  await cards(page, world())
  const bot = card(page, 'p-hausv').locator('.live-bot').first()
  expect(await bot.locator('.bob').evaluate(el => getComputedStyle(el).animationName)).toBe('none')
  expect(await card(page, 'p-hausv').locator('.typing i').first().evaluate(el => getComputedStyle(el).animationName)).toBe('none')
  await expect(card(page, 'p-hausv').locator('.live-chip')).toContainText('HAUSV-887')
  expect(Number(await bot.locator('.tip').evaluate(el => getComputedStyle(el).opacity))).toBe(1)
})

for (const colorScheme of ['light', 'dark'] as const) {
  test(`live chips and their details pass axe in ${colorScheme}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme })
    await cards(page, world())
    await card(page, 'p-aeon').locator('.live-chip').click()
    await expect(page.getByRole('dialog', { name: 'Agents working on Aeon' })).toBeVisible()
    const results = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).exclude('.version-coordinate').exclude('.calendar-version').analyze()
    const summary = results.violations.map(v => `${v.id} (${v.impact}): ${v.help}\n${v.nodes.slice(0, 4).map(n => `    ${n.target.join(' ')} — ${n.failureSummary?.split('\n').slice(1, 2).join(' ').trim()}`).join('\n')}`)
    expect(summary, summary.join('\n')).toEqual([])
  })

  test(`a phone shows the live robots on cards and rows without scrolling sideways in ${colorScheme}`, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.emulateMedia({ colorScheme })
    const data = world()
    await cards(page, data)
    await expect(card(page, 'p-hausv').locator('.live-chip')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const chip = (await card(page, 'p-hausv').locator('.live-chip').boundingBox())!
    const time = (await card(page, 'p-hausv').locator('.activity').boundingBox())!
    expect(chip.x + chip.width).toBeLessThan(time.x)
    await page.getByRole('radio', { name: 'List view' }).click()
    await expect(row(page, 'p-hausv').locator('.live-chip')).toBeVisible()
    await expect(row(page, 'p-hausv').locator('.live-chip .key')).toBeHidden()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}

// ---------- Design review screenshots (LIVE_SHOTS=<dir>) ----------
const shots = process.env.LIVE_SHOTS ?? ''
test.describe('screenshots', () => {
  test.skip(!shots, 'LIVE_SHOTS names the folder')
  test.use({ reducedMotion: 'no-preference' })
  for (const colorScheme of ['light', 'dark'] as const) {
    for (const width of [1600, 390]) {
      for (const view of ['cards', 'list'] as const) {
        test(`shot ${view} ${width} ${colorScheme}`, async ({ page }) => {
          mkdirSync(shots, { recursive: true })
          await page.setViewportSize({ width, height: width > 600 ? 1000 : 1400 })
          await page.emulateMedia({ colorScheme })
          const data = world()
          data.preferences.projects = { view }
          await mockWork(page, data)
          await page.goto('/')
          await expect(page.locator('.live-chip').first()).toBeVisible()
          await page.waitForTimeout(700)
          await page.screenshot({ path: `${shots}/la1-${view}-${width}-${colorScheme}.png` })
        })
      }
    }
    test(`shot popover ${colorScheme}`, async ({ page }) => {
      mkdirSync(shots, { recursive: true })
      await page.setViewportSize({ width: 1600, height: 1000 })
      await page.emulateMedia({ colorScheme })
      await mockWork(page, world())
      await page.goto('/')
      await card(page, 'p-ops').locator('.live-chip').hover()
      await expect(page.getByRole('dialog', { name: 'Agents working on Operations' })).toBeVisible()
      await page.waitForTimeout(400)
      await page.screenshot({ path: `${shots}/la1-popover-${colorScheme}.png` })
    })
  }
  test.describe('frames', () => {
    test.use({ deviceScaleFactor: 3 })
    for (const colorScheme of ['light', 'dark'] as const) {
      test(`shot frames ${colorScheme}`, async ({ page }) => {
        mkdirSync(shots, { recursive: true })
        await page.setViewportSize({ width: 1600, height: 1000 })
        await page.emulateMedia({ colorScheme })
        await mockWork(page, world())
        await page.goto('/')
        const target = card(page, 'p-ops')
        await expect(target.locator('.live-chip')).toBeVisible()
        const box = (await target.boundingBox())!
        for (const [i, t] of [0, 480, 960, 1440].entries()) {
          // Every animation paused at the same moment of its own loop.
          await page.evaluate(ms => { for (const a of document.getAnimations()) { a.pause(); a.currentTime = ms } }, 3000 + t)
          await page.screenshot({ path: `${shots}/la1-frame-${colorScheme}-${i + 1}.png`, clip: { x: box.x, y: box.y + box.height - 62, width: box.width * .62, height: 62 } })
        }
        // Pointed at, the robots smile back (before the details open).
        await page.evaluate(() => { for (const a of document.getAnimations()) a.play() })
        await target.locator('.live-chip').hover()
        await page.waitForTimeout(150)
        await page.screenshot({ path: `${shots}/la1-hover-${colorScheme}.png`, clip: { x: box.x, y: box.y + box.height - 62, width: box.width * .62, height: 62 } })
      })
    }
  })
})
