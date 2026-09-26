// SPDX-License-Identifier: AGPL-3.0-only
// Coordinator: PLAYWRIGHT_PORT=5833 JP1_SCREENSHOT_DIR=<scratchpad> npm test -- tests/journey-overflow.spec.ts
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { test, expect, type Locator, type Page } from '@playwright/test'
import { fixtures, mockWork, watchErrors } from './work-fixtures'
import { journeyWorld, mockJourney } from './journey-fixtures'

const longTitle = 'List queries can hang on a bad plan right after bulk writes; keep every backlog row and its separator inside the card while the ticket panel is open. '.repeat(3).trim()
const unbrokenTitle = 'UnbrokenTicketTitle'.repeat(24)
type Stage = 'backlog' | 'plan' | 'build' | 'deploy' | 'access'

async function open(page: Page, stage: Stage, theme: 'light' | 'dark', panel: boolean) {
  const work = fixtures()
  work.preferences.theme = { choice: theme }
  work.preferences.layout = { panel: page.viewportSize()!.width >= 1800 ? 880 : 560 }
  for (const node of work.nodes.filter(n => n.project === 'p-pharos')) {
    node.title = node.id === 'n-2' ? unbrokenTitle : longTitle
  }
  // Exercise collapsed/expanded groups and their newly rendered separators.
  const source = work.nodes.find(n => n.id === 'n-1')!
  for (let i = 0; i < 6; i++) work.nodes.push({ ...source, id: `overflow-${i}`, key: `PHAROS-${100 + i}` })
  await mockWork(page, work)
  const world = journeyWorld(stage === 'backlog' ? 'open' : stage === 'access' ? 'deploy' : stage)
  for (const walker of Object.values(world.walkers)) {
    for (const ticket of walker.tickets) ticket.title = ticket.ticket_node_id === 'n-2' ? unbrokenTitle : longTitle
    for (const feature of walker.features) feature.title = longTitle
  }
  if (stage === 'deploy' || stage === 'access') {
    world.journey.stage = stage
    world.journey.stages = world.journey.stages.map(s => ({ ...s, state: s.key === stage ? 'current' : s.state, handoff_id: s.key === stage ? 'h-1' : null }))
    if (stage === 'access') world.journey.next_action = { key: 'approve_permit', label: 'Approve permit', stage, available: false, reason: 'A bounded permit needs approval.', approval_request_id: null }
    world.handoffs['h-1'] = {
      id: 'h-1', project_node_id: 'p-pharos', release_node_id: 'r-2', stage,
      operation: stage === 'deploy' ? 'deploy' : 'apply', plugin_id: 'provider'.repeat(30),
      attempt: 1, authority_epoch: 1, state: 'failed', expires_at: '2026-09-26T12:00:00Z',
      result: { outcome: 'failed', blocker_code: 'readiness'.repeat(30), completed_at: '2026-09-26T11:00:00Z' },
    }
  }
  await mockJourney(page, world)
  await page.goto(`/p/PHAROS${panel ? '/PHAROS-11' : ''}?view=journey`)
  await expect(page.getByRole('navigation', { name: 'Project journey' })).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
  const ticketPanel = page.getByRole('complementary', { name: 'Ticket details' })
  if (panel) await expect(ticketPanel).toBeVisible()
  else await expect(ticketPanel).toHaveCount(0)
}

// Check row geometry and scroll width, not just clipping at a card edge. The
// separator is a row box-shadow, so its width is bounded by the same rectangle.
async function contained(rows: Locator) {
  expect(await rows.count()).toBeGreaterThan(0)
  const overflow = await rows.evaluateAll(elements => elements.flatMap(element => {
    const row = element as HTMLElement
    const card = row.closest<HTMLElement>('.j-card')!
    const box = row.getBoundingClientRect(), bounds = card.getBoundingClientRect()
    const padding = getComputedStyle(card)
    const left = bounds.left + parseFloat(padding.paddingLeft)
    const right = bounds.right - parseFloat(padding.paddingRight)
    return box.left < left - 1 || box.right > right + 1 || row.scrollWidth > row.clientWidth + 1
      ? [{ text: row.textContent?.slice(0, 80), left: box.left, right: box.right, cardLeft: left, cardRight: right, scroll: row.scrollWidth, client: row.clientWidth }]
      : []
  }))
  expect(overflow).toEqual([])
}

async function cardsFit(page: Page) {
  const result = await page.locator('.journey-view').evaluate(view => {
    const bounds = view.getBoundingClientRect()
    const cards = [...view.querySelectorAll<HTMLElement>('.j-card, .gate-card')].filter(el => el.getClientRects().length)
    const outside = cards.filter(card => {
      const r = card.getBoundingClientRect()
      return r.left < bounds.left - 1 || r.right > bounds.right + 1 || card.scrollWidth > card.clientWidth + 1
    }).map(card => card.getAttribute('aria-label') ?? card.getAttribute('aria-labelledby'))
    const overlaps = cards.flatMap((card, index) => cards.slice(index + 1).filter(other => {
      const a = card.getBoundingClientRect(), b = other.getBoundingClientRect()
      return Math.min(a.right, b.right) - Math.max(a.left, b.left) > 1 && Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top) > 1
    }).map(other => `${card.className} / ${other.className}`))
    return { outside, overlaps }
  })
  expect(result).toEqual({ outside: [], overlaps: [] })
}

async function fullTitleOnFocus(page: Page, title: Locator) {
  await expect(title).toHaveAccessibleName(longTitle)
  await expect(title).toHaveCSS('text-overflow', 'ellipsis')
  await expect(title).toHaveCSS('white-space', 'nowrap')
  expect(await title.evaluate(el => el.scrollWidth > el.clientWidth && el.clientWidth > 0)).toBe(true)
  // Keyboard modality makes the shared tooltip host show on focus-visible.
  await title.scrollIntoViewIfNeeded()
  await page.keyboard.press('Tab')
  await title.focus()
  await expect(page.locator('.tooltip')).toContainText(longTitle)
  await title.blur()
  await title.hover()
  await expect(page.locator('.tooltip')).toContainText(longTitle)
  await page.mouse.move(0, 0)
}

for (const width of [1280, 1440, 1999]) {
  for (const theme of ['light', 'dark'] as const) {
    for (const panel of [false, true]) {
      for (const stage of ['backlog', 'plan', 'build', 'deploy', 'access'] as const) {
        test(`${stage}: contained at ${width}, ${theme}, ticket panel ${panel ? 'open' : 'closed'}`, async ({ page }, testInfo) => {
          const errors = watchErrors(page)
          await page.setViewportSize({ width, height: 1000 })
          await page.emulateMedia({ colorScheme: theme })
          await open(page, stage, theme, panel)
          if (stage === 'backlog') {
            const card = page.getByRole('region', { name: 'Backlog · what release 1 is chosen from' })
            await expect(card.locator('.row-title').first()).toHaveText(longTitle)
            await expect(page.getByRole('region', { name: 'Decision: Open release 1' })).toBeVisible()
            await contained(card.locator('.j-rows > li'))
            await fullTitleOnFocus(page, card.locator('.row-title').first())
            await card.getByRole('button', { name: /Show \d+ more/ }).click()
            await expect(card.locator('.row-title')).toHaveCount(9)
            await contained(card.locator('.j-rows > li'))
          } else if (stage === 'plan' || stage === 'build') {
            const titles = page.locator('.release-tickets .tk:visible .title')
            await expect(titles.first()).toHaveText(longTitle)
            await contained(page.locator('.release-tickets .tk:visible'))
            await contained(page.locator('.release-list .j-timeline > li'))
            await fullTitleOnFocus(page, titles.first())
          } else {
            const handoffs = page.getByRole('region', { name: stage === 'deploy' ? 'Handoffs · deploy and verify' : 'Handoffs · prepare and apply' })
            await expect(handoffs.getByRole('listitem')).toHaveCount(1)
            await contained(handoffs.getByRole('listitem'))
            await contained(page.locator('.j-kv .j-checks > li'))
          }
          await cardsFit(page)
          const dir = process.env.JP1_SCREENSHOT_DIR ?? testInfo.outputDir
          mkdirSync(dir, { recursive: true })
          await page.locator('#journey-title').scrollIntoViewIfNeeded()
          await page.screenshot({ path: join(dir, `jp1-${stage}-${width}-${theme}-panel-${panel ? 'open' : 'closed'}.png`), fullPage: true, animations: 'disabled' })
          expect(errors).toEqual([])
        })
      }
    }
  }
}
