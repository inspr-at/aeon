// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect, type Page } from '@playwright/test'

const agentId = '10000000-0000-4000-8000-000000000001'
const runId = '20000000-0000-4000-8000-000000000001'
const accountId = '30000000-0000-4000-8000-000000000001'
const approvalId = '40000000-0000-4000-8000-000000000001'
const stamp = '2026-09-23T10:00:00Z'
const account = { id: accountId, account_key: 'opaque-account', harness: 'codex', daemon_id: 'daemon-workstation', label: 'Build account', registered_by_principal_id: agentId, state: 'available', max_parallel_runs: 2, last_probe_at: stamp, last_probe_ok: true, created_at: stamp }
const run = { id: runId, work_order_id: '50000000-0000-4000-8000-000000000001', agent_principal_id: agentId, status: 'running', account_id: accountId, requested_model: 'requested-model', effective_model: 'effective-model', model_evidence: 'vendor_reported', input_tokens: 4200, output_tokens: 800, cost_micros: 123456, created_at: stamp, started_at: stamp }
const approval = { id: approvalId, agent_principal_id: agentId, scope: 'run.claim', resource_kind: 'run', resource_id: runId, run_id: runId, rationale: 'Claim the assigned work on this daemon.', expires_at: '2099-01-01T00:00:00Z', proposed_at: stamp, decision: null as string | null }

async function setup(page: Page, options: { kind?: string; admin?: boolean; empty?: boolean } = {}) {
  const state = { accounts: options.empty ? [] : [{ ...account }], run: { ...run }, approvals: options.empty ? [] : [{ ...approval }], failRead: false, failWrite: false, delayRun: false }
  const calls: { path: string; method: string; body: any }[] = []
  await page.addInitScript(() => {
    const streams: any[] = []
    class MockEventSource {
      listeners: Record<string, (() => void)[]> = {}; onopen?: () => void; onerror?: () => void; onmessage?: () => void; closed = false
      constructor() { streams.push(this); setTimeout(() => this.onopen?.(), 10) }
      addEventListener(name: string, fn: () => void) { (this.listeners[name] ??= []).push(fn) }
      close() { this.closed = true }
    }
    Object.assign(window, { EventSource: MockEventSource, agentStreams: streams, emitAgentEvent: (name: string) => streams.filter(s => !s.closed).forEach(s => {
      if (name === 'open') s.onopen?.()
      else if (name === 'error') s.onerror?.()
      else s.listeners[name]?.forEach((fn: () => void) => fn())
    }) })
  })
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname
    const method = route.request().method()
    const body = route.request().postDataJSON()
    calls.push({ path, method, body })
    if (path === '/api/me') return route.fulfill({ json: { principal: { id: 'person-1', name: 'Markus Barta', kind: options.kind ?? 'person', roles: options.admin === false ? [] : ['admin'] }, tenant: { id: 'tenant-1', name: 'INSPR Studio' } } })
    if (path === '/api/version') return route.fulfill({ json: { version: '260923143005.0.0', scheme: 'inspr-calendar-v2' } })
    if (state.failRead && method === 'GET') return route.fulfill({ status: 503, json: { error: 'Service unavailable' } })
    if (state.failWrite && method !== 'GET') return route.fulfill({ status: 409, json: { error: 'Concurrent change; refresh and retry' } })
    if (path === '/api/agent-accounts') return route.fulfill({ json: state.accounts })
    if (path === `/api/agent-accounts/${accountId}` && method === 'PATCH') { Object.assign(state.accounts[0]!, body); return route.fulfill({ json: state.accounts[0] }) }
    if (path === `/api/agent-accounts/${accountId}/windows` && method === 'POST') return route.fulfill({ status: 201, json: { ...body, id: 'window-1', account_id: accountId, used: 0, reserved: 0 } })
    if (path === `/api/runs/${runId}`) {
      const snapshot = { ...state.run }
      if (state.delayRun) await new Promise(resolve => setTimeout(resolve, 500))
      return route.fulfill({ json: snapshot })
    }
    if (path.startsWith('/api/runs/')) return route.fulfill({ status: 404, json: { error: 'Run not found' } })
    if (path === '/api/approvals') return route.fulfill({ json: state.approvals })
    if (path === `/api/approvals/${approvalId}/decision`) { state.approvals[0]!.decision = body.decision; return route.fulfill({ json: state.approvals[0] }) }
    if (path === `/api/approvals/${approvalId}/revoke`) return route.fulfill({ json: state.approvals[0] })
    if (path === '/api/kinds' || path === '/api/views') return route.fulfill({ json: { items: [] } })
    if (path === '/api/nodes/tree') return route.fulfill({ json: { items: [], next_cursor: null } })
    return route.fulfill({ status: 404, json: { error: 'Uncontracted route' } })
  })
  return { state, calls }
}
async function emit(page: Page, type: string) { await page.evaluate(type => (window as any).emitAgentEvent(type), type) }

test('registered accounts, administrator drain and live probe refresh', async ({ page }) => {
  const { state, calls } = await setup(page)
  await page.goto('/agents')
  await expect(page.getByRole('heading', { name: 'Build account' })).toBeVisible()
  await page.getByRole('button', { name: 'Drain', exact: true }).click()
  await expect(page.getByText('draining', { exact: true })).toBeVisible()
  expect(calls.find(c => c.method === 'PATCH')?.body).toEqual({ state: 'draining' })
  state.accounts[0]!.last_probe_ok = false
  await emit(page, 'agent_account.probed')
  await expect(page.getByText('Unavailable', { exact: true })).toBeVisible()
  await emit(page, 'error')
  await expect(page.getByText('Reconnecting · refresh every 30s')).toBeVisible()
  state.accounts[0]!.label = 'Reconnected account'
  await emit(page, 'open')
  await expect(page.getByRole('heading', { name: 'Reconnected account' })).toBeVisible()
})

test('run lookup, telemetry refresh, model evidence and stream cleanup', async ({ page }) => {
  const { state, calls } = await setup(page)
  await page.goto('/runs')
  await page.getByLabel('Run ID').fill(runId)
  await page.getByRole('button', { name: 'Open run', exact: true }).click()
  await expect(page.getByLabel('Run telemetry')).toContainText('effective-model (Vendor reported)')
  await expect(page.getByLabel('Run telemetry')).toContainText('4,200')
  state.run.output_tokens = 1600; state.run.status = 'ownership_lost'
  await emit(page, 'run.telemetry')
  await expect(page.getByLabel('Run telemetry')).toContainText('1,600')
  await expect(page.getByText('Daemon ownership was lost.', { exact: false })).toBeVisible()
  await page.getByRole('link', { name: 'Agents', exact: true }).click()
  await expect.poll(() => page.evaluate(() => (window as any).agentStreams.filter((s: any) => s.closed).length)).toBe(1)
  expect(calls.some(c => c.path === '/api/runs/queued' || c.path.endsWith('/telemetry'))).toBe(false)
})

test('approval is an explicit scoped human decision; history supports revocation', async ({ page }) => {
  const { calls } = await setup(page)
  await page.goto('/approvals')
  await expect(page.getByText(approval.rationale)).toBeVisible()
  expect(calls.some(c => c.method === 'POST')).toBe(false)
  await page.getByRole('button', { name: 'Review request' }).click()
  await page.getByLabel('Decision reason (optional)').fill('Confirmed assigned scope')
  await page.getByRole('button', { name: 'Approve permission' }).click()
  await expect(page.getByText('No pending approvals.')).toBeVisible()
  expect(calls.find(c => c.path.endsWith('/decision'))?.body).toEqual({ decision: 'approved', reason: 'Confirmed assigned scope' })
  await page.getByLabel('Include decided and expired requests').check()
  await page.getByRole('button', { name: 'Review revocation' }).click()
  await page.getByRole('button', { name: 'Revoke grant', exact: true }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Revocation recorded.' })).toBeVisible()
  expect(calls.filter(c => c.path.endsWith('/revoke'))).toHaveLength(1)
})

test('denial conflicts preserve reason and do not claim success', async ({ page }) => {
  const { state, calls } = await setup(page)
  state.failWrite = true
  await page.goto('/approvals')
  await page.getByRole('button', { name: 'Review request' }).click()
  await page.getByLabel('Decision reason (optional)').fill('Wrong resource')
  await page.getByRole('button', { name: 'Deny request' }).click()
  await expect(page.getByRole('alert')).toContainText('Concurrent change')
  await expect(page.getByLabel('Decision reason (optional)')).toHaveValue('Wrong resource')
  expect(calls.find(c => c.path.endsWith('/decision'))?.body.decision).toBe('denied')
  state.failWrite = false
  await page.getByRole('button', { name: 'Deny request' }).click()
  await expect(page.getByText('Decision recorded: denied.')).toBeVisible()
})

test('expired approvals and agent identities cannot decide', async ({ page }) => {
  const { state, calls } = await setup(page, { kind: 'agent', admin: false })
  state.approvals[0]!.expires_at = '2000-01-01T00:00:00Z'
  await page.goto('/approvals')
  await expect(page.getByText('No pending approvals.')).toBeVisible()
  await page.getByLabel('Include decided and expired requests').check()
  await expect(page.getByText('expired', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Review request' })).toHaveCount(0)
  state.approvals[0]!.expires_at = approval.expires_at
  await emit(page, 'approval.proposed')
  await expect(page.getByText('pending', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Review request' })).toHaveCount(0)
  expect(calls.every(c => c.method === 'GET')).toBe(true)
})

test('pacing preview and window creation match the contract', async ({ page }) => {
  const { calls } = await setup(page)
  await page.goto('/pacing')
  // The wrapping label includes option text; match the select's accessible name.
  await page.getByRole('combobox', { name: 'Account', exact: true }).selectOption(accountId)
  await page.getByLabel('Starts (local time)').fill('2026-09-23T10:00')
  await page.getByLabel('Ends (local time)').fill('2026-09-23T20:00')
  await page.getByLabel('Pacing model').selectOption('frontload')
  await expect(page.getByRole('progressbar', { name: 'Allowed at 50% elapsed' })).toHaveAttribute('value', '0.85')
  await page.getByRole('button', { name: 'Create allowance window' }).click()
  await expect(page.getByText('Allowance window created', { exact: true })).toBeVisible()
  const body = calls.find(c => c.path.endsWith('/windows'))!.body
  expect(body).toMatchObject({ allowance: 1000, unit: 'tokens', pace_model: 'frontload', burst_ratio: 0.1 })
  expect(Date.parse(body.ends_at) - Date.parse(body.starts_at)).toBe(10 * 60 * 60 * 1000)
  expect(calls.filter(c => c.path.endsWith('/windows') && c.method === 'GET')).toHaveLength(0)
  await page.getByLabel('Pacing model').selectOption('unrestricted')
  await expect(page.getByRole('progressbar', { name: 'Allowed at 0% elapsed' })).toHaveAttribute('value', '1')
})

test('pacing rejects reversed dates and preserves drafts on conflict and refresh', async ({ page }) => {
  const { state, calls } = await setup(page)
  await page.goto('/pacing')
  await page.getByRole('combobox', { name: 'Account', exact: true }).selectOption(accountId)
  await page.getByLabel('Starts (local time)').fill('2026-09-24T10:00')
  await page.getByLabel('Ends (local time)').fill('2026-09-23T10:00')
  await page.getByRole('button', { name: 'Create allowance window' }).click()
  await expect(page.getByRole('alert')).toContainText('an end after the start')
  expect(calls.filter(c => c.method === 'POST')).toHaveLength(0)
  await page.getByLabel('Ends (local time)').fill('2026-09-25T10:00')
  state.failWrite = true
  await page.getByRole('button', { name: 'Create allowance window' }).click()
  await expect(page.getByRole('alert')).toContainText('Concurrent change')
  await emit(page, 'allowance.created')
  await expect(page.getByLabel('Starts (local time)')).toHaveValue('2026-09-24T10:00')
  await expect(page.getByText('Allowance window created', { exact: true })).toHaveCount(0)
})

test('read failures expose retry and disable mutations on stale accounts', async ({ page }) => {
  const { state } = await setup(page)
  await page.goto('/agents')
  await expect(page.getByRole('heading', { name: 'Build account' })).toBeVisible()
  state.failRead = true
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('Service unavailable')
  await expect(page.getByRole('button', { name: 'Drain', exact: true })).toBeDisabled()
  state.failRead = false
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Drain', exact: true })).toBeEnabled()
})

test('empty accounts and non-admin sessions have no account controls', async ({ page }) => {
  await setup(page, { empty: true, admin: false })
  await page.goto('/agents')
  await expect(page.getByText('No daemon accounts registered yet.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Drain', exact: true })).toHaveCount(0)
  await page.getByRole('link', { name: 'Pacing', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Create allowance window' })).toBeDisabled()
})

for (const theme of ['light', 'dark'] as const) {
  test(`operations navigation and mobile layout in ${theme}`, async ({ page }) => {
    await setup(page)
    await page.setViewportSize({ width: 390, height: 844 })
    await page.emulateMedia({ colorScheme: theme })
    const errors: string[] = []
    page.on('pageerror', error => errors.push(error.message))
    await page.goto('/agents')
    for (const section of ['Agents', 'Sessions & runs', 'Approvals', 'Pacing']) {
      await page.getByRole('link', { name: section, exact: true }).click()
      await expect(page.getByRole('heading', { name: section, exact: true })).toBeVisible()
      expect(await page.evaluate(() => document.querySelector('main')!.scrollWidth <= innerWidth)).toBe(true)
    }
    await page.screenshot({ path: `/tmp/aeon-p27-${theme}.png`, fullPage: true })
    expect(errors).toEqual([])
  })
}
