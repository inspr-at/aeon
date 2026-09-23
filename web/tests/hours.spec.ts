// SPDX-License-Identifier: AGPL-3.0-only
// Coordinator: npx playwright test -c playwright.ui.config.ts hours.spec.ts
// Mounts the owned view through Vite; production router is coordinator-owned.
import { test, expect, type Page } from '@playwright/test'

test.use({ timezoneId: 'Europe/Vienna' })

const person = '10000000-0000-4000-8000-000000000001'
const periodID = '20000000-0000-4000-8000-000000000001'
const root = '30000000-0000-4000-8000-000000000001'
const cost = '40000000-0000-4000-8000-000000000001'
const digest = 'a'.repeat(64)
const stamp = '2026-09-23T12:00:00Z'
async function setup(page: Page, options: { approved?: boolean; member?: boolean; disabled?: boolean; empty?: boolean } = {}) {
  const period = { id: periodID, principal_id: person, starts_at: '2026-09-01T00:00:00Z', ends_at: '2026-10-01T00:00:00Z', state: options.approved ? 'approved' : 'open', revision: 4, approval: options.approved ? { approved_by_principal_id: person, total_seconds: 3600 } : null }
  const state = { period, conflict: false, disabled: options.disabled || false, entries: options.empty ? [] : [{ id: 'entry', principal_id: person, node_id: root, source: 'manual', started_at: stamp, duration_seconds: 3600, rate_amount: '99999999999999.9999', amount: '99999999999999.9999', currency: 'EUR', note: 'Recorded work' }] }
  const calls: { path: string; method: string; body: any }[] = []
  await page.addInitScript(() => { class MockEventSource { addEventListener() {} close() {} }; Object.assign(window, { EventSource: MockEventSource }) })
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url()), path = url.pathname, method = route.request().method(), body = route.request().postDataJSON()
    calls.push({ path: path + url.search, method, body })
    if (path === '/api/me') return route.fulfill({ json: { principal: { id: person, kind: 'person', name: 'Markus Barta', roles: [options.member ? 'member' : 'admin'] }, tenant: { id: 'tenant', name: 'INSPR' } } })
    if (path === '/api/version') return route.fulfill({ json: { version: '260923180522.0.0', scheme: 'inspr-calendar-v2' } })
    if (state.disabled && (path.includes('time-') || path.includes('time-totals'))) return route.fulfill({ status: 403, json: { error: 'Required business plugin is disabled' } })
    if (path === '/api/time-periods') {
      if (method === 'POST') Object.assign(state.period, body, { state: 'open', revision: 1 })
      return route.fulfill({ status: method === 'POST' ? 201 : 200, json: method === 'POST' ? state.period : [state.period] })
    }
    if (path.endsWith('/approve')) {
      if (state.conflict) return route.fulfill({ status: 409, json: { error: 'Period changed; reload before approving' } })
      Object.assign(state.period, { state: 'approved', approval: { approved_by_principal_id: person, total_seconds: 3600 } })
      return route.fulfill({ json: state.period })
    }
    if (path === `/api/time-periods/${periodID}`) return route.fulfill({ json: state.period, headers: { 'X-Entries-SHA256': digest } })
    if (path === '/api/time-entries') {
      if (method === 'POST') { state.period.revision++; return route.fulfill({ status: 201, json: { id: 'new-entry', ...body } }) }
      // Deliberately return JSON number tokens beyond Number's exact range.
      const raw = JSON.stringify(state.entries).replace(/"(99999999999999\.9999)"/g, '$1')
      return route.fulfill({ contentType: 'application/json', body: raw })
    }
    if (path.endsWith('/time-totals')) return route.fulfill({ contentType: 'application/json', body: '{"duration_seconds":3600,"amounts":[{"currency":"EUR","amount":99999999999999.9999},{"currency":"USD","amount":0.0001}]}' })
    if (path === '/api/kinds') return route.fulfill({ json: { items: [{ id: 'cost-kind', slug: 'cost_unit', label: 'Cost unit' }] } })
    if (path === '/api/nodes') return route.fulfill({ json: { items: [{ id: root, key: 'PRJ-1', title: 'Delivery', kind_id: 'project' }, { id: cost, key: 'CU-1', title: 'Hourly work', kind_id: 'cost-kind' }], next_cursor: null } })
    if (path === '/api/views' || path === '/api/nodes/tree') return route.fulfill({ json: { items: [], next_cursor: null } })
    return route.fulfill({ status: 404, json: { error: 'Unmocked route' } })
  })
  return { state, calls }
}
async function mount(page: Page) {
  await page.goto('/')
  await page.evaluate(async () => {
    const routerPath = '/src/router.ts', viewPath = '/src/views/business/HoursView.vue'
    const { router } = await import(routerPath), { default: component } = await import(viewPath)
    router.addRoute({ path: '/business/hours', component })
    await router.push('/business/hours')
  })
  await expect(page.getByRole('heading', { name: 'Hours', exact: true })).toBeVisible()
}

test('approval pins reviewed revision/digest and seals the period', async ({ page }) => {
  const { calls } = await setup(page); await mount(page)
  await expect(page.getByRole('cell', { name: 'EUR 99999999999999.9999', exact: true })).toHaveCount(2)
  const approve = page.getByRole('button', { name: 'Approve and close period' })
  await expect(approve).toBeDisabled()
  await page.getByRole('checkbox', { name: 'I have reviewed these entries' }).check()
  await approve.click()
  await expect(page.getByRole('heading', { name: 'Approved period', exact: true })).toBeVisible()
  const call = calls.find(c => c.path.endsWith('/approve'))!
  expect(call.body).toEqual({ expected_revision: 4, expected_entries_sha256: digest })
  await expect(page.getByText('Record time', { exact: true })).toHaveCount(0)
})
test('stale approval preserves the open period and requires renewed review', async ({ page }) => {
  const { state } = await setup(page); state.conflict = true; await mount(page)
  await page.getByRole('checkbox', { name: 'I have reviewed these entries' }).check()
  await page.getByRole('button', { name: 'Approve and close period' }).click()
  await expect(page.getByRole('alert')).toContainText('Period changed')
  await expect(page.getByRole('heading', { name: 'Open period', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Approve and close period' })).toBeDisabled()
})
test('terminal run conversion sends no client time or principal facts', async ({ page }) => {
  const { calls } = await setup(page); await mount(page)
  await page.locator('summary').filter({ hasText: /^Record time$/ }).click()
  await page.getByLabel('Source', { exact: true }).selectOption('agent_run')
  await page.getByLabel('Agent run ID', { exact: true }).fill('50000000-0000-4000-8000-000000000001')
  await page.getByLabel('Cost unit', { exact: true }).selectOption(cost)
  await page.getByRole('button', { name: 'Record time', exact: true }).click()
  await expect(page.getByRole('status', { name: 'Hours status', exact: true })).toHaveText('Time recorded.')
  const call = calls.find(c => c.path === '/api/time-entries' && c.method === 'POST')!
  expect(call.body).toEqual({ period_id: periodID, source: 'agent_run', cost_unit_node_id: cost, currency: 'EUR', agent_run_id: '50000000-0000-4000-8000-000000000001', note: '' })
})
test('subtree totals keep currencies separate and exact', async ({ page }) => {
  const { calls } = await setup(page, { approved: true }); await mount(page)
  await page.getByLabel('Root work item', { exact: true }).selectOption(root)
  await page.getByRole('checkbox', { name: 'Approved periods only' }).check()
  await page.getByRole('button', { name: 'Calculate totals' }).click()
  const totals = page.getByRole('status', { name: 'Subtree totals', exact: true })
  await expect(totals.locator('p')).toHaveText(['EUR 99999999999999.9999', 'USD 0.0001'])
  await expect(totals.getByText('USD 0.0001', { exact: true })).toBeVisible()
  expect(calls.some(c => c.path.endsWith('/time-totals?approved_only=true'))).toBeTruthy()
  await expect(page.getByRole('button', { name: 'Approve and close period' })).toHaveCount(0)
})
test('disabled installations hide forms and a member has no approval controls', async ({ page }) => {
  const { state } = await setup(page, { disabled: true, member: true, empty: true }); await mount(page)
  await expect(page.getByRole('alert')).toContainText('plugin is disabled')
  await expect(page.getByLabel('Period', { exact: true })).toHaveCount(0)
  state.disabled = false
  await page.getByRole('button', { name: 'Reload', exact: true }).click()
  await expect(page.getByText('No time entries in this period.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Approve and close period' })).toHaveCount(0)
})
test('manual entry submits the selected principal and UTC interval', async ({ page }) => {
  const { calls } = await setup(page); await mount(page)
  await page.locator('summary').filter({ hasText: /^Record time$/ }).click()
  await page.getByLabel('Work item', { exact: true }).selectOption(root)
  await page.getByLabel('Started', { exact: true }).fill('2026-09-23T12:00:00')
  await page.getByLabel('Ended', { exact: true }).fill('2026-09-23T13:00:00')
  await page.getByLabel('Cost unit', { exact: true }).selectOption(cost)
  await page.getByRole('button', { name: 'Record time', exact: true }).click()
  await expect(page.getByRole('status', { name: 'Hours status', exact: true })).toHaveText('Time recorded.')
  const writes = calls.filter(c => c.path === '/api/time-entries' && c.method === 'POST')
  expect(writes).toHaveLength(1)
  expect(writes[0]!.body).toEqual({ period_id: periodID, principal_id: person, node_id: root, source: 'manual', cost_unit_node_id: cost, currency: 'EUR', note: '', started_at: '2026-09-23T10:00:00.000Z', ended_at: '2026-09-23T11:00:00.000Z' })
  expect(writes[0]!.body.started_at).toMatch(/Z$/)
  expect(Date.parse(writes[0]!.body.ended_at) - Date.parse(writes[0]!.body.started_at)).toBe(3600000)
})
