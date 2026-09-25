// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test, type Page } from '@playwright/test'
import { fixtures, mockWork } from './work-fixtures'

async function throttle(page: Page) {
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('Network.emulateNetworkConditions', {
    offline: false, latency: 150, downloadThroughput: 200_000, uploadThroughput: 93_750,
  })
  await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 })
  await page.addInitScript(() => {
    let lcp = 0
    Object.defineProperty(window, '__perfLCP', { get: () => lcp })
    new PerformanceObserver(list => {
      for (const entry of list.getEntries()) lcp = entry.startTime
    }).observe({ type: 'largest-contentful-paint', buffered: true })
  })
}

async function paint(page: Page) {
  await page.waitForTimeout(350)
  return page.evaluate(() => ({
    fcp: Math.round(performance.getEntriesByName('first-contentful-paint')[0]?.startTime ?? 0),
    lcp: Math.round((window as typeof window & { __perfLCP?: number }).__perfLCP ?? 0),
  }))
}

test('cold Projects home has bounded API requests', async ({ page }) => {
  test.setTimeout(60_000)
  await throttle(page)
  const calls = await mockWork(page, fixtures({ bigProject: 200 }))
  await page.goto('/')
  await expect(page.getByRole('list', { name: 'Projects' }).getByRole('link')).toHaveCount(3, { timeout: 30_000 })
  const timings = await paint(page)
  const paths = calls.map(call => call.path)
  console.log(`PERF projects ${JSON.stringify({ ...timings, requests: paths })}`)
  expect(paths.filter(path => path === '/api/projects')).toHaveLength(1)
  expect(paths.filter(path => path === '/api/nodes')).toHaveLength(1)
  expect(paths.filter(path => /^\/api\/nodes\/[^/]+$/.test(path))).toHaveLength(0)
})

test('cold ticket page has bounded API requests', async ({ page }) => {
  test.setTimeout(60_000)
  await throttle(page)
  const calls = await mockWork(page, fixtures({ bigProject: 200 }))
  await page.goto('/p/PHAROS/PHAROS-11')
  await expect(page.getByRole('heading', { name: /Hetzner Cloud for managed provisioning/ }).first()).toBeVisible({ timeout: 30_000 })
  const timings = await paint(page)
  const paths = calls.map(call => call.path)
  console.log(`PERF ticket ${JSON.stringify({ ...timings, requests: paths, nodeQueries: calls.filter(call => call.path === '/api/nodes').map(call => call.query.toString()) })}`)
  expect(paths.filter(path => path === '/api/nodes/lookup')).toHaveLength(1)
  expect(paths.filter(path => /^\/api\/nodes\/[^/]+$/.test(path) && path !== '/api/nodes/lookup')).toHaveLength(1)
  expect(paths.filter(path => path === '/api/nodes').length).toBeLessThanOrEqual(4)
})
