// SPDX-License-Identifier: AGPL-3.0-only
// U24 (AEON-139): the journey of a project imported from classic Paimos, and the
// stages every project has not reached yet. Each stage shows real content or an
// honest state with the way on; none is empty or a dead end. Mocked data only.
import { expect, test, type Page } from '@playwright/test'
import { fixtures, mockWork } from './work-fixtures'
import { journeyWorld, mockJourney, PROJECT, type JourneyStart, type WorldOptions, type JourneyWorld } from './journey-fixtures'

const STAGES = ['inspire', 'shape', 'requirements', 'plan', 'build', 'deploy', 'access', 'live'] as const
const BODY = 'Fleet management and host access for a synthetic studio.\n\nEvery host is provisioned after an approved price, and cleanup leaves nothing behind.'
async function open(page: Page, start: JourneyStart, stage: string | null, options: WorldOptions = {}, tune?: (world: JourneyWorld) => void, width = 1440) {
  await page.setViewportSize({ width, height: 900 })
  await mockWork(page, fixtures())
  const world = journeyWorld(start, options)
  world.projectBody = BODY
  world.knowledge = [{ id: 'k-1', key: 'PHAROS-40', type: 'runbook', kind: 'runbook', slug: 'deploy-release', title: 'Deploy a release to production', status: 'active', state: 'active', project: { id: PROJECT, key: 'PRJ-17', title: 'Pharos' }, excerpt: '', link_count: 0, created_at: new Date().toISOString(), updated_at: new Date().toISOString(), updated_by: null, imported: true }]
  tune?.(world)
  await mockJourney(page, world)
  await page.goto(`/p/PHAROS?view=journey${stage ? `&stage=${stage}` : ''}`)
  await expect(page.getByRole('navigation', { name: 'Project journey' })).toBeVisible()
  await expect(page.locator('.journey-view .skeleton')).toHaveCount(0)
  return world
}

test('Inspire of an imported project shows what it brought: its work, its description and its knowledge', async ({ page }) => {
  await open(page, 'build', 'inspire', { derived: true })
  await expect(page.getByRole('heading', { name: 'Where it came from' })).toBeVisible()
  const brought = page.getByRole('list', { name: 'What the project brought' })
  // PHAROS-10 is the one epic; four tickets stand (the cancelled one does not), one of them done.
  await expect(brought).toContainText('1epic')
  await expect(brought).toContainText('4tickets · 1 done')
  await expect(brought).toContainText('1knowledge entry')
  await expect(page.locator('.origin .description')).toContainText('Every host is provisioned after an approved price')
  await expect(page.getByRole('link', { name: 'Deploy a release to production' })).toHaveAttribute('href', '/p/PHAROS/knowledge/runbook/deploy-release')
  const card = page.getByRole('region', { name: 'Imported: Started in Paimos' })
  await card.getByRole('button', { name: 'Go to Build' }).click()
  await expect(page).toHaveURL(/stage=build/)
})

test('Shape of an imported project: the description is the brief of record, decided before it came here', async ({ page }) => {
  await open(page, 'build', 'shape', { derived: true })
  await expect(page.getByText('Brief · the project’s description')).toBeVisible()
  await expect(page.locator('.origin .description')).toContainText('Fleet management and host access')
  await expect(page.getByRole('region', { name: 'Decided: Go, before it came here' })).toBeVisible()
  await expect(page.getByRole('combobox')).toHaveValue('professional')
})

test('Requirements of an imported project: its epics stand as the features, with how far their tickets are', async ({ page }) => {
  await open(page, 'build', 'requirements', { derived: true })
  const features = page.getByRole('region', { name: /Features · 1 · imported epics/ })
  await expect(features).toContainText('Guarded multi-cloud provisioning')
  await expect(features).toContainText('0 of 2 tickets done')
  await expect(features.getByRole('link', { name: 'PHAROS-10' })).toHaveAttribute('href', '/p/PHAROS/PHAROS-10')
  await expect(features).toContainText('2 tickets are not tied to an epic.')
  await expect(page.getByRole('region', { name: 'Imported: Features came with the project' })).toBeVisible()
  // No empty "0 functional requirements" boxes beside it.
  await expect(page.getByText(/Functional · 0/)).toHaveCount(0)
})

test('Plan before release 1 shows the backlog it is chosen from, grouped by epic, open tickets only', async ({ page }) => {
  await open(page, 'open', null, { derived: true })
  const backlog = page.getByRole('region', { name: 'Backlog · what release 1 is chosen from' })
  await expect(backlog).toContainText('3 open tickets')
  await expect(backlog).toContainText('Guarded multi-cloud provisioning')
  await expect(backlog).toContainText('Not tied to an epic')
  await expect(backlog.getByRole('button', { name: 'Beacon health probes' })).toHaveCount(0)
  await expect(backlog.getByRole('button', { name: 'Retire the old dashboard' })).toHaveCount(0)
  await expect(page.getByRole('region', { name: 'Decision: Open release 1' })).toBeVisible()
  await backlog.getByRole('button', { name: 'Add an Oracle Cloud connector' }).click()
  await expect(page).toHaveURL(/\/p\/PHAROS\/PHAROS-12/)
})

test('a stage not reached yet says what comes first and leads there', async ({ page }) => {
  await open(page, 'open', 'deploy', { derived: true })
  const later = page.getByRole('region', { name: 'Later: Not yet' })
  await expect(later).toContainText('The journey is at Plan: open release 1 comes first.')
  // No empty handoff card before anything was handed off.
  await expect(page.getByText('Handoffs · deploy and verify')).toHaveCount(0)
  await later.getByRole('button', { name: 'Go to Plan' }).click()
  await expect(page).toHaveURL(/stage=plan/)
  await expect(page.getByRole('region', { name: 'Decision: Open release 1' })).toBeVisible()
})

test('Live of an imported project shows the latest released release while the journey is still building', async ({ page }) => {
  await open(page, 'build', 'live', { derived: true })
  await expect(page.getByRole('heading', { name: 'Release 1 is live' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Live now: Release 1 is the latest released' })).toContainText('1 release of 2 went live before the journey was recorded here.')
  await expect(page.locator('.release-tickets')).toContainText('Beacon health probes')
})

test('Build and Live of a project without releases are one card each, never an empty stage', async ({ page }) => {
  await open(page, 'inspire', 'build', {}, world => { world.releases = [] })
  await expect(page.getByRole('region', { name: 'Later: Not yet' })).toContainText('The journey is at Inspire: continue intake comes first.')
  await expect(page.getByText('No release is being built')).toHaveCount(0)
  await page.goto('/p/PHAROS?view=journey&stage=live')
  await expect(page.getByRole('region', { name: 'Later: Not yet' })).toContainText('Nothing from this project has gone live here yet.')
})

for (const width of [1440, 390]) {
  test(`every stage of an imported project has content at ${width}px, and nothing is cut`, async ({ page }) => {
    await open(page, 'open', null, { derived: true }, undefined, width)
    for (const stage of STAGES) {
      await page.goto(`/p/PHAROS?view=journey&stage=${stage}`)
      await expect(page.locator('.journey-view .stage-head')).toBeVisible()
      await expect(page.locator('.journey-view .skeleton')).toHaveCount(0)
      const cards = page.locator('.journey-view .j-card, .journey-view .gate-card')
      expect(await cards.count(), stage).toBeGreaterThan(0)
      const text = await page.locator('.journey-view').innerText()
      expect(text, stage).not.toMatch(/undefined|NaN|\[object/)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), stage).toBe(true)
    }
  })
}
