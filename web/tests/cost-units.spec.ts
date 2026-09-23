// SPDX-License-Identifier: AGPL-3.0-only
// Coordinator route: /business/cost-units -> views/business/CostUnitsView.vue
import { test, expect, type Page } from '@playwright/test'

const kindID = '10000000-0000-4000-8000-0000000000c1'
const nodeID = '20000000-0000-4000-8000-0000000000c1'
const rateID = '30000000-0000-4000-8000-0000000000c1'
const stamp = '2026-09-23T10:00:00Z'
const costUnit = {
  id: nodeID, key: 'PAI-101', kind_id: kindID, title: 'Studio delivery', body: '',
  fields: { classic: { source_id: 'ppm', id: 101, issue_key: 'PAI-101', type: 'cost_unit' }, notes: 'imported' },
  state: 'open', parent_id: null, position: '1', created_at: stamp, updated_at: stamp,
}
const rate = {
  id: rateID, cost_unit_node_id: nodeID, unit: 'hour', currency: 'EUR',
  internal_amount: 125.5, bill_amount: 180, effective_from: '2026-01-01', effective_until: null,
  created_by_principal_id: 'person-1', created_at: stamp,
}

async function setup(page: Page, options: { admin?: boolean; empty?: boolean; conflict?: boolean } = {}) {
  const calls: { path: string; method: string; raw: string | null }[] = []
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname
    const method = route.request().method()
    const raw = route.request().postData()
    calls.push({ path, method, raw })
    if (path === '/api/me') return route.fulfill({ json: { principal: { id: 'person-1', name: 'Markus Barta', kind: 'person', roles: options.admin === false ? [] : ['admin'] }, tenant: { id: 'tenant-1', name: 'INSPR Studio' } } })
    if (path === '/api/version') return route.fulfill({ json: { version: '260923180522.0.0', scheme: 'inspr-calendar-v2' } })
    if (path === '/api/kinds') return route.fulfill({ json: { items: options.empty ? [] : [{ id: kindID, slug: 'cost_unit', label: 'Cost unit', short_prefix: 'CU', icon: 'cost_unit', allowed_child_kinds: [], field_schema: {} }] } })
    if (path === '/api/nodes') return route.fulfill({ json: { items: options.empty ? [] : [costUnit], next_cursor: null } })
    if (path === `/api/cost-units/${nodeID}/rates` && method === 'GET') return route.fulfill({ json: options.empty ? [] : [rate] })
    if (path === `/api/cost-units/${nodeID}/rates` && method === 'POST') {
      if (options.conflict) return route.fulfill({ status: 409, json: { code: 'conflict', message: 'rate intervals overlap' } })
      return route.fulfill({ status: 201, json: { ...rate, id: '40000000-0000-4000-8000-0000000000c1', internal_amount: 125.25, bill_amount: 200, effective_from: '2026-06-01' } })
    }
    if (path === '/api/views') return route.fulfill({ json: { items: [] } })
    return route.fulfill({ status: 404, json: { code: 'not_found', message: 'Uncontracted route' } })
  })
  return calls
}

test('imported cost unit keeps its key and shows exact rate amounts', async ({ page }) => {
  await setup(page)
  await page.goto('/business/cost-units')
  await expect(page.getByRole('heading', { name: 'Cost units' })).toBeVisible()
  await expect(page.getByRole('button', { name: /PAI-101/ })).toBeVisible()
  await expect(page.getByText('Imported', { exact: true })).toBeVisible()
  await expect(page.getByText('101', { exact: true })).toBeVisible()
  await expect(page.getByRole('cell', { name: '125.5', exact: true })).toBeVisible()
  await expect(page.getByRole('cell', { name: '180', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'EUR' })).toBeVisible()
})

test('admin records an exact decimal rate', async ({ page }) => {
  const calls = await setup(page)
  await page.goto('/business/cost-units')
  await page.getByLabel('Internal amount').fill('125.25')
  await page.getByLabel('Bill amount').fill('200')
  await page.getByLabel('Effective from').fill('2026-06-01')
  await page.getByRole('button', { name: 'Record rate' }).click()
  await expect(page.getByRole('status')).toHaveText('Rate recorded.')
  const posted = calls.find(call => call.method === 'POST' && call.path.endsWith('/rates'))
  expect(posted?.raw).toContain('"internal_amount":125.25')
  expect(posted?.raw).toContain('"bill_amount":200')
  expect(posted?.raw).not.toContain('"internal_amount":"')
})

test('a member does not get the rate form', async ({ page }) => {
  const calls = await setup(page, { admin: false })
  await page.goto('/business/cost-units')
  await expect(page.getByText('PAI-101')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Record rate' })).toHaveCount(0)
  expect(calls.some(call => call.method === 'POST')).toBe(false)
})

test('a rejected rate keeps the typed amount', async ({ page }) => {
  await setup(page, { conflict: true })
  await page.goto('/business/cost-units')
  await page.getByLabel('Internal amount').fill('10.25')
  await page.getByLabel('Bill amount').fill('12')
  await page.getByLabel('Effective from').fill('2026-03-01')
  await page.getByRole('button', { name: 'Record rate' }).click()
  await expect(page.getByRole('alert')).toContainText('rate intervals overlap')
  await expect(page.getByLabel('Internal amount')).toHaveValue('10.25')
})

test('empty tenant explains that imported keys are kept', async ({ page }) => {
  await setup(page, { empty: true })
  await page.goto('/business/cost-units')
  await expect(page.getByText(/original keys/)).toBeVisible()
})

test('the cost unit screen fits a phone width', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await setup(page)
  await page.goto('/business/cost-units')
  await expect(page.getByRole('heading', { name: 'Studio delivery' })).toBeVisible()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth + 1)
  expect(overflow).toBe(false)
})
