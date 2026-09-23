// SPDX-License-Identifier: AGPL-3.0-only
// Requires the coordinator route { path: '/business', component: BusinessHome, meta: { title: 'Business' } }.
import { test, expect, type Page } from '@playwright/test'

const build = 'ab'.repeat(32)
const pinned = 'cd'.repeat(32)
const identity = {
  principal: { id: 'person-1', name: 'Markus Barta', kind: 'person', roles: ['admin'] },
  tenant: { id: 'tenant-1', name: 'INSPR Studio' },
}

function plugin(id: string, permissions: string[], installation: { enabled: boolean; permissions: string[]; digest?: string; updated_at?: string }) {
  return {
    id,
    version: '1',
    digest_sha256: build,
    owner: 'aeon',
    permissions,
    node_kinds: id === 'business_costs' ? [{ slug: 'cost_unit', field_schema: {} }] : [],
    views: [{ id, panels: ['main'] }],
    workflow_steps: [],
    agent_tools: [],
    integrations: [],
    background_jobs: [],
    installation: {
      manifest_digest_sha256: installation.digest ?? build,
      enabled: installation.enabled,
      permissions: installation.permissions,
      plugin_id: id,
      version: '1',
      updated_at: installation.updated_at ?? '2026-09-23T18:00:00Z',
    },
  }
}

function catalog() {
  return [
    plugin('business_costs', ['nodes.contribute', 'views.provide', 'steps.apply'], { enabled: true, permissions: ['nodes.contribute', 'views.provide', 'steps.apply'] }),
    plugin('business_crm', ['nodes.contribute', 'views.provide'], { enabled: false, permissions: [], updated_at: '0001-01-01T00:00:00Z' }),
    plugin('business_quotes', ['views.provide', 'steps.apply'], { enabled: true, permissions: ['views.provide', 'steps.apply'] }),
    plugin('business_hours', ['views.provide', 'steps.apply'], { enabled: true, permissions: ['steps.apply'] }),
  ]
}

async function mockAPI(page: Page, options: { admin?: boolean; items?: ReturnType<typeof catalog> } = {}) {
  let items = options.items ?? catalog()
  const calls: { path: string; method: string; body: string | null }[] = []
  const principal = {
    ...identity.principal,
    roles: options.admin === false ? ['customer'] : identity.principal.roles,
  }
  await page.route('**/api/**', async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    const body = request.postData()
    calls.push({ path, method: request.method(), body })
    if (path === '/api/version') return route.fulfill({ json: { version: '260923120000.0.0', scheme: 'inspr-calendar-v2' } })
    if (path === '/api/me') return route.fulfill({ json: { principal, tenant: identity.tenant, dev_mode: false } })
    if (path === '/api/plugins' && request.method() === 'GET') return route.fulfill({ json: items })
    const install = path.match(/^\/api\/plugins\/([a-z][a-z0-9_]*)\/installation$/)
    if (install && request.method() === 'PUT') {
      const written = JSON.parse(body ?? '{}') as { manifest_digest_sha256: string; enabled: boolean; permissions: string[] }
      items = items.map(item => item.id === install[1] ? {
        ...item,
        installation: {
          ...item.installation,
          manifest_digest_sha256: written.manifest_digest_sha256,
          enabled: written.enabled,
          permissions: written.permissions,
          updated_at: '2026-09-23T18:05:00Z',
        },
      } : item)
      return route.fulfill({ json: items.find(item => item.id === install[1])?.installation })
    }
    return route.fulfill({ status: 404, json: {} })
  })
  return { calls, read: () => items }
}

async function noOverflow(page: Page) {
  expect(await page.evaluate(() => {
    const main = document.querySelector('main')!
    return {
      documentX: document.documentElement.scrollWidth > innerWidth,
      documentY: document.documentElement.scrollHeight > innerHeight,
      mainY: main.scrollHeight > main.clientHeight + 1,
      mainX: main.scrollWidth > main.clientWidth + 1,
    }
  })).toEqual({ documentX: false, documentY: false, mainY: false, mainX: false })
}

test('admin enables an unpinned plugin with the compiled digest, then quotes open', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const api = await mockAPI(page)
  await page.goto('/business')
  await expect(page.getByRole('heading', { name: 'Business' }), 'coordinator registers /business to BusinessHome').toBeVisible()
  await expect(page.getByRole('article', { name: 'Quotes' })).toContainText('Waiting on Organisations.')
  await expect(page.getByRole('link', { name: 'Open Quotes' })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Open Cost units' })).toHaveAttribute('href', '/business/costs')
  await page.getByRole('button', { name: 'Enable Organisations' }).click()
  await expect(page.getByRole('link', { name: 'Open Quotes' })).toHaveAttribute('href', '/business/quotes')
  const put = api.calls.find(call => call.method === 'PUT')
  assertBody(put?.body, {
    manifest_digest_sha256: build,
    enabled: true,
    permissions: ['nodes.contribute', 'views.provide'],
  })
  expect(errors).toEqual([])
})

test('digest mismatch is relabelled with the compiled digest, not the pin', async ({ page }) => {
  const items = catalog()
  items[3] = plugin('business_hours', ['views.provide', 'steps.apply'], { enabled: true, permissions: ['views.provide', 'steps.apply'], digest: pinned })
  const api = await mockAPI(page, { items })
  await page.goto('/business')
  const hours = page.getByRole('article', { name: 'Hours' })
  await expect(hours).toContainText('Digest mismatch')
  await expect(hours).toContainText('does not match this build')
  await page.getByRole('button', { name: 'Enable Hours' }).click()
  const put = api.calls.find(call => call.path === '/api/plugins/business_hours/installation')
  assertBody(put?.body, {
    manifest_digest_sha256: build,
    enabled: true,
    permissions: ['views.provide', 'steps.apply'],
  })
})

test('a customer sees open sections and no installation controls', async ({ page }) => {
  await mockAPI(page, { admin: false })
  await page.goto('/business')
  await expect(page.getByRole('link', { name: 'Open Cost units' })).toBeVisible()
  await expect(page.getByRole('button', { name: /Enable|Disable|Grant/ })).toHaveCount(0)
})

test('a failed plugin list stays closed and retries', async ({ page }) => {
  let fail = true
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/version') return route.fulfill({ json: { version: '260923120000.0.0', scheme: 'inspr-calendar-v2' } })
    if (path === '/api/me') return route.fulfill({ json: { ...identity, dev_mode: false } })
    if (path === '/api/plugins') {
      if (fail) return route.fulfill({ status: 503, json: { message: 'Plugin list unavailable' } })
      return route.fulfill({ json: catalog() })
    }
    return route.fulfill({ status: 404, json: {} })
  })
  await page.goto('/business')
  await expect(page.getByRole('alert')).toContainText('Plugin list unavailable')
  await expect(page.getByRole('link', { name: 'Open Cost units' })).toHaveCount(0)
  fail = false
  await page.getByRole('button', { name: 'Refresh' }).click()
  await expect(page.getByRole('link', { name: 'Open Cost units' })).toBeVisible()
})

for (const viewport of [{ width: 1280, height: 720 }, { width: 390, height: 844 }]) {
  for (const colorScheme of ['light', 'dark'] as const) {
    test(`business home ${viewport.width} ${colorScheme}`, async ({ page }) => {
      const errors: string[] = []
      page.on('pageerror', error => errors.push(error.message))
      await page.setViewportSize(viewport)
      await page.emulateMedia({ colorScheme })
      await mockAPI(page)
      await page.goto('/business')
      await expect(page.getByRole('heading', { name: 'Business' })).toBeVisible()
      await page.evaluate(() => document.fonts.ready)
      await noOverflow(page)
      for (const control of await page.locator('main').locator('button:visible, a:visible, [role="button"]:visible').all()) {
        const bounds = await control.boundingBox()
        expect(bounds!.height).toBeGreaterThanOrEqual(44)
        expect(bounds!.width).toBeGreaterThanOrEqual(44)
      }
      const centered = await page.locator('.business-nav .business-link, .business-mark, .business-heading .button, .business-actions .button').evaluateAll(nodes => nodes.map(node => {
        const icon = node.querySelector('svg')
        if (!icon) return 0
        const host = node.getBoundingClientRect()
        const mark = icon.getBoundingClientRect()
        return Math.abs((host.y + host.height / 2) - (mark.y + mark.height / 2))
      }))
      for (const delta of centered) expect(delta).toBeLessThanOrEqual(1)
      expect(errors).toEqual([])
    })
  }
}

function assertBody(body: string | null | undefined, expected: unknown) {
  expect(JSON.parse(body ?? 'null')).toEqual(expected)
}
