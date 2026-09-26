// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect, type Page } from '@playwright/test'
import { fixtures, mockWork } from './work-fixtures'
import { knowledgeWorld, mockKnowledge } from './knowledge-fixtures'

// Always Playwright's bundled Chromium; software WebGL also works on CI.
test.use({ launchOptions: { args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] } })
async function setup(page: Page) {
  await mockWork(page, fixtures())
  const world = knowledgeWorld()
  await mockKnowledge(page, world)
  const nodes = world.entries.filter(n => n.project === 'p-pharos' && n.status !== 'archived').map(n => ({
    id: n.id, key: n.key, type: n.type, kind: 'knowledge', slug: n.slug, title: n.title, status: n.status, degree: 2, updated_at: n.updated_at,
  }))
  const edges = nodes.slice(1).map((n, i) => ({ source: nodes[i].id, target: n.id, kind: 'mention', label: 'mentions' }))
  const calls: URLSearchParams[] = []
  await page.route('**/api/knowledge/graph?*', route => {
    const params = new URL(route.request().url()).searchParams; calls.push(params)
    const include = params.get('include') === 'tickets'
    return route.fulfill({ json: { nodes: [...nodes, ...(include ? [{ id: 'ticket-1', key: 'PHAROS-11', type: 'ticket', kind: 'ticket', slug: '', title: 'Linked ticket', degree: 1, status: 'done', updated_at: nodes[0].updated_at }] : [])], edges: [...edges, ...(include ? [{ source: nodes[0].id, target: 'ticket-1', kind: 'relation', label: 'cites' }] : [])], truncated: false } })
  })
  return { calls, nodes }
}
const toggle = (page: Page, name: 'Entries' | 'Graph') => page.getByRole('group', { name: 'Knowledge display', exact: true }).getByRole('button', { name, exact: true })
const canvas = (page: Page) => page.locator('.kg-canvas')
// U25: on wide screens (1200px and up) a selected entry opens in the preview pane beside the graph.
const pane = (page: Page) => page.locator('.entry-page.dock')
// The heading exists only once this entry's fetch has landed. The graph can be
// ready while that chunk and request are still queued behind it.
async function dockedHeading(page: Page, title: string) {
  const dock = pane(page)
  await expect(dock).toHaveAttribute('data-loaded', 'true', { timeout: 15_000 })
  await expect(dock.getByRole('heading', { level: 1 })).toHaveText(title)
}
// The graph fits its camera and picks the bubble under the pointer on animation frames.
const frames = (page: Page, count = 3) => page.evaluate(n => new Promise<void>(done => { const step = (left: number) => left ? requestAnimationFrame(() => step(left - 1)) : done(); step(n) }), count)
// The canvas box once layout has stopped moving (the pane opening narrows it).
async function settledBox(page: Page) {
  let last = ''
  await expect.poll(async () => { const now = JSON.stringify(await canvas(page).boundingBox()); const same = now === last; last = now; return same }, { intervals: [120] }).toBe(true)
  return JSON.parse(last) as { x: number; y: number; width: number; height: number }
}
async function ready(page: Page) {
  // Cold lazy-module compilation and a software GPU can exceed the default 5s
  // when the coordinator runs the suite with four browser workers.
  await expect(canvas(page)).toHaveAttribute('data-ready', 'true', { timeout: 15_000 })
  await expect(page.locator('.kg-state')).toHaveCount(0)
}

async function holdNextSessionCheck(page: Page) {
  let entered!: () => void, release!: () => void
  const requested = new Promise<void>(resolve => { entered = resolve })
  const allowed = new Promise<void>(resolve => { release = resolve })
  let held = false
  await page.route('**/api/me', async route => {
    if (!held) { held = true; entered(); await allowed }
    await route.fallback()
  })
  return { requested, release }
}

async function recordCancelledNavigations(page: Page) {
  await page.evaluate(async () => {
    const { router } = await import('/src/router.ts')
    const failures: number[] = []
    Object.assign(window, { navigationFailures: failures })
    router.afterEach((_to, _from, failure) => { if (failure) failures.push(failure.type) })
  })
}

test('graph toggle and filters preserve the URL; selection, keyboard open and history work', async ({ page }) => {
  const loaded: string[] = []
  page.on('request', request => loaded.push(new URL(request.url()).pathname))
  const { calls } = await setup(page)
  await page.goto('/p/PHAROS/knowledge')
  await expect(page.locator('.k-row')).toHaveCount(8)
  expect(loaded.some(path => /3d-force-graph|force-graph|three-spritetext/.test(path))).toBe(false)
  await toggle(page, 'Graph').click(); await expect(page).toHaveURL(/mode=graph/); await ready(page)
  await expect(canvas(page)).toHaveAttribute('data-dimension', '2d')
  await expect(canvas(page)).toHaveAttribute('data-motion', 'still')
  await expect(page.getByRole('button', { name: 'Resume motion' })).toBeDisabled()
  await page.getByRole('navigation', { name: 'Kinds of knowledge' }).getByRole('button', { name: /Runbooks/ }).click()
  await expect(page).toHaveURL(/mode=graph.*type=runbook/)
  await expect(canvas(page)).toHaveAttribute('aria-label', /2 entries, 1 link/)
  await page.getByRole('searchbox', { name: 'Search knowledge in Pharos' }).fill('Deploy')
  await expect(page.locator('.kg-results')).toContainText('1 match')
  await page.locator('.kg-results').getByRole('button', { name: 'Deploy a release to production' }).click()
  await expect(page).toHaveURL(/entry=runbook\/deploy-release/)
  await expect(pane(page).getByRole('heading', { level: 1 })).toHaveText('Deploy a release to production')
  await expect(page.locator('.kg-selection')).toHaveCount(0)
  await canvas(page).press('Escape'); await expect(page).not.toHaveURL(/entry=/)
  await expect(pane(page)).toHaveCount(0); await expect(page).toHaveURL(/mode=graph/)
  await canvas(page).press('ArrowRight'); await expect(page).toHaveURL(/entry=runbook\/rotate-host-keys/)
  await expect(pane(page).getByRole('heading', { level: 1 })).toHaveText('Rotate the fleet host keys')
  await canvas(page).press('Enter'); await expect(page).toHaveURL(/knowledge\/runbook\/rotate-host-keys/)
  await page.goBack(); await ready(page)
  await expect(pane(page)).toBeVisible()
  await page.getByRole('button', { name: 'Show linked tickets' }).click()
  await expect.poll(() => calls.some(q => q.get('include') === 'tickets')).toBe(true)
  // Entries keeps the pane (and so the selection) open beside the list.
  await toggle(page, 'Entries').click(); await expect(page).not.toHaveURL(/mode=graph/)
  await expect(canvas(page)).toHaveCount(0)
  await expect(page).toHaveURL(/entry=runbook\/rotate-host-keys/)
  await expect(pane(page).getByRole('heading', { level: 1 })).toHaveText('Rotate the fleet host keys')
})

test('the pane closes back to the graph, which keeps the display and filters', async ({ page }) => {
  await setup(page)
  await page.goto('/p/PHAROS/knowledge?mode=graph&type=runbook&entry=runbook/deploy-release'); await ready(page)
  await dockedHeading(page, 'Deploy a release to production')
  await expect(page.getByRole('group', { name: 'Knowledge display', exact: true })).toBeVisible()
  await expect(toggle(page, 'Graph')).toHaveAttribute('aria-pressed', 'true')
  await pane(page).getByRole('button', { name: 'Close the preview' }).click()
  await expect(page).toHaveURL(/\/knowledge\?(?=.*mode=graph)(?=.*type=runbook)(?!.*entry=)/)
  await expect(canvas(page)).toBeFocused()
  // Expanding keeps the way back to the graph.
  // The pane's controls are used once its entry has loaded.
  await canvas(page).press('ArrowRight')
  await dockedHeading(page, 'Rotate the fleet host keys')
  await pane(page).getByRole('button', { name: 'Open as full page' }).click()
  await expect(page).toHaveURL(/knowledge\/runbook\/[^?]+\?(?=.*mode=graph)/)
  await page.keyboard.press('Escape')
  await expect(page).toHaveURL(/\/knowledge\?(?=.*mode=graph)/); await ready(page)
})

test('below the docking width the graph keeps its own selection card', async ({ page }) => {
  await page.setViewportSize({ width: 1100, height: 800 })
  await setup(page)
  await page.goto('/p/PHAROS/knowledge?mode=graph&entry=runbook/deploy-release'); await ready(page)
  await expect(page).toHaveURL(/\/knowledge\?(?=.*mode=graph)(?=.*entry=runbook\/deploy-release)/)
  await expect(page.locator('.kg-selection')).toContainText('Deploy a release to production')
  await expect(pane(page)).toHaveCount(0)
  // Back to Entries drops the selection here, rather than opening the entry's page.
  await toggle(page, 'Entries').click()
  await expect(page).toHaveURL(/\/knowledge$/)
  await expect(page.locator('.k-row')).toHaveCount(8)
})

test('a competing graph selection cannot cancel the switch to Entries on a narrow screen', async ({ page }) => {
  await page.setViewportSize({ width: 1100, height: 800 })
  await setup(page)
  await page.goto('/p/PHAROS/knowledge?mode=graph'); await ready(page)
  await recordCancelledNavigations(page)
  const check = await holdNextSessionCheck(page)
  await page.evaluate(() => document.querySelector<HTMLButtonElement>('[aria-label="Knowledge display"] [aria-label="Entries"]')!.click())
  await check.requested
  await page.evaluate(() => document.querySelector<HTMLElement>('.kg-canvas')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true })))
  check.release()
  await expect(page).toHaveURL('/p/PHAROS/knowledge')
  await expect(page.locator('.k-row')).toHaveCount(8)
  await expect(canvas(page)).toHaveCount(0)
  expect(await page.evaluate(() => (window as unknown as { navigationFailures: number[] }).navigationFailures)).toContain(8)
})

test('a cancelled filter write follows a graph selection without losing the filter', async ({ page }) => {
  await page.setViewportSize({ width: 1100, height: 800 })
  await setup(page)
  await page.goto('/p/PHAROS/knowledge?mode=graph'); await ready(page)
  await recordCancelledNavigations(page)
  const check = await holdNextSessionCheck(page)
  await page.getByRole('searchbox', { name: 'Search knowledge in Pharos' }).fill('Deploy')
  await check.requested
  await page.evaluate(() => document.querySelector<HTMLElement>('.kg-canvas')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true })))
  check.release()
  await expect(page).toHaveURL(/\/knowledge\?(?=.*mode=graph)(?=.*q=Deploy)(?=.*entry=)/)
  await expect(page.locator('.kg-results')).toContainText('1 match')
  expect(await page.evaluate(() => (window as unknown as { navigationFailures: number[] }).navigationFailures)).toContain(8)
})

test('selection deep links survive reload, retheme, and dimension changes', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await setup(page)
  await page.goto('/p/PHAROS/knowledge?mode=graph&entry=runbook/deploy-release'); await ready(page)
  await expect(pane(page).getByRole('heading', { level: 1 })).toHaveText('Deploy a release to production')
  await expect(canvas(page)).toHaveAttribute('data-dimension', '3d')
  await page.evaluate(() => { document.documentElement.dataset.theme = 'dark' })
  await page.getByRole('button', { name: '2D', exact: true }).click(); await ready(page)
  await expect(page).toHaveURL(/entry=runbook\/deploy-release/)
  await page.getByRole('button', { name: '3D', exact: true }).click(); await ready(page)
  await page.reload(); await ready(page)
  await expect(pane(page).getByRole('heading', { level: 1 })).toHaveText('Deploy a release to production')
})

test('no WebGL uses the canvas fallback with the same selection controls', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.addInitScript(() => {
    const original = HTMLCanvasElement.prototype.getContext
    HTMLCanvasElement.prototype.getContext = function (kind: string, ...args: unknown[]) {
      if (kind.startsWith('webgl') || kind === 'experimental-webgl') return null
      return original.apply(this, [kind, ...args] as Parameters<typeof original>)
    } as typeof original
  })
  await setup(page); await page.goto('/p/PHAROS/knowledge?mode=graph'); await ready(page)
  await expect(canvas(page)).toHaveAttribute('data-dimension', '2d')
  await expect(page.getByText('3D is unavailable here.', { exact: false })).toBeVisible()
  await canvas(page).press('ArrowRight'); await expect(page).toHaveURL(/entry=/)
})

test('20 mode switches dispose every WebGL context and removed canvas', async ({ page }) => {
  test.setTimeout(120_000)
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.addInitScript(() => {
    const state = { created: 0, lost: 0 }; Object.assign(window, { graphContexts: state })
    const seen = new WeakSet<object>(), original = HTMLCanvasElement.prototype.getContext
    HTMLCanvasElement.prototype.getContext = function (...args: Parameters<typeof original>) {
      const result = original.apply(this, args)
      if (result && String(args[0]).startsWith('webgl') && !seen.has(result)) {
        seen.add(result); state.created++
        this.addEventListener('webglcontextlost', () => state.lost++, { once: true })
      }
      return result
    } as typeof original
  })
  await setup(page); await page.goto('/p/PHAROS/knowledge')
  for (let i = 0; i < 20; i++) {
    await toggle(page, 'Graph').click(); await ready(page)
    await expect(canvas(page)).toHaveAttribute('data-dimension', '3d')
    await expect(page.locator('.kg-canvas canvas')).toHaveCount(1)
    await toggle(page, 'Entries').click(); await expect(page.locator('.kg-canvas canvas')).toHaveCount(0)
    await expect.poll(() => page.evaluate(() => { const s = (window as unknown as { graphContexts: { created: number; lost: number } }).graphContexts; return s.created - s.lost })).toBe(0)
  }
  expect(await page.evaluate(() => (window as unknown as { graphContexts: { created: number } }).graphContexts.created)).toBeGreaterThanOrEqual(20)
})

test('a bubble hover, click and double-click work on the canvas itself', async ({ page }) => {
  const { nodes } = await setup(page)
  await page.route('**/api/knowledge/graph?*', route => route.fulfill({ json: { nodes: [nodes[0]], edges: [], truncated: false } }))
  await page.goto('/p/PHAROS/knowledge?mode=graph'); await ready(page); await frames(page)
  const box = (await canvas(page).boundingBox())!
  const point = { x: box.x + box.width / 2, y: box.y + box.height / 2 }
  await page.mouse.move(point.x, point.y)
  await expect(page.getByRole('tooltip')).toContainText(nodes[0].title)
  await page.mouse.click(point.x, point.y)
  await expect(page).toHaveURL(new RegExp(`entry=${nodes[0].type}/${nodes[0].slug}`))
  await expect(pane(page).getByRole('heading', { level: 1 })).toHaveText(nodes[0].title)
  // The stage narrows for the pane and keeps the selection in its centre.
  const narrowed = await settledBox(page)
  expect(narrowed.width).toBeLessThan(box.width - 200)
  const centre = { x: narrowed.x + narrowed.width / 2, y: narrowed.y + narrowed.height / 2 }
  await frames(page); await page.mouse.move(centre.x, centre.y); await frames(page)
  // Two clicks after selection also open the full entry page.
  await page.mouse.dblclick(centre.x, centre.y)
  await expect(page).toHaveURL(new RegExp(`/knowledge/${nodes[0].type}/${nodes[0].slug}`))
})
