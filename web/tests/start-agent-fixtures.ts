// SPDX-License-Identifier: AGPL-3.0-only
import type { Page } from '@playwright/test'
import { fixtures, me, mockWork } from './work-fixtures'
import { mockEffectivePermissions } from './authz-fixtures'
import type { AgentAccount, AgentRun, HarnessSession, ModelProfile } from '../src/lib/agents'
import type { WorkOrder } from '../src/lib/startAgent'

export async function mockStartAgent(page: Page, options: { offline?: boolean; unavailable?: boolean; forbidden?: boolean; failQueue?: boolean; readOnly?: boolean } = {}) {
  const work = fixtures()
  await mockWork(page, work, { admin: true })
  const agentId = 'a0000000-0000-4000-8000-000000000001'
  const profile: ModelProfile = { id: 'model-enabled', slug: 'Build · deliberate', harness: 'codex', family: 'openai', model: 'workspace-build', effort: 'high', tier: 'standard', enabled: true }
  const models = [profile, { ...profile, id: 'model-disabled', slug: 'Retired model', enabled: false }]
  const account: AgentAccount = {
    id: 'account-1', account_key: 'work', harness: 'codex', daemon_id: 'workstation', label: 'Workspace account', registered_by_principal_id: agentId,
    state: options.unavailable ? 'draining' : 'available', max_parallel_runs: 2, last_probe_at: new Date(Date.now() - (options.offline ? 180_000 : 1000)).toISOString(), last_probe_ok: true, created_at: new Date().toISOString(),
    windows: [{ id: 'window-1', account_id: 'account-1', starts_at: new Date(Date.now() - 60_000).toISOString(), ends_at: new Date(Date.now() + 3600_000).toISOString(), unit: 'requests', allowance: 100, used: 1, reserved: 0, pace_model: 'unrestricted', burst_ratio: 0 }],
  }
  const state = { orders: [] as WorkOrder[], runs: [] as AgentRun[], sessions: [] as HarnessSession[], failQueue: !!options.failQueue }
  const calls: { method: string; path: string; body: Record<string, unknown> | null }[] = []
  await page.route('**/api/**', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname, method = req.method()
    const json = (value: unknown, status = 200) => route.fulfill({ status, json: value })
    const body = req.postData() ? req.postDataJSON() as Record<string, unknown> : null
    calls.push({ method, path, body })
    if (path === '/api/me/permissions') {
      const permissions = mockEffectivePermissions(options.readOnly ? 'viewer' : 'admin', url.searchParams.get('project_id') ?? undefined)
      if (!options.readOnly) permissions.workspace.permissions.push('run.create', 'run.read', 'models.read', 'account.read', 'work_orders.read')
      return json(permissions)
    }
    if (path === '/api/business/principals') return options.forbidden ? json({ error: 'Permission denied' }, 403) : json([{ id: agentId, name: 'Studio builder', kind: 'agent', roles: [] }, { ...me, kind: 'person', roles: [] }])
    if (path === '/api/models') return json(models)
    if (path === '/api/agent-accounts') return json([account])
    if (path === '/api/harness-sessions') return json({ items: state.sessions, next_cursor: null })
    if (path === '/api/approvals') return json([])
    if (path.endsWith('/message-targets')) return json([])
    if (path.endsWith('/messages')) return json({ items: [], next_after: 0 })
    if (path === '/api/runs') return json({ items: state.runs.filter(r => !url.searchParams.get('work_order') || r.work_order_id === url.searchParams.get('work_order')), next_cursor: null })
    if (path.startsWith('/api/runs/')) return json(state.runs.find(r => path.endsWith(r.id)))
    if (path === '/api/nodes' && url.searchParams.get('kind') === 'work_order') return json({ items: state.orders.map(o => ({ id: o.node_id })), next_cursor: null })
    if (path === '/api/nodes/order-1') return json({ id: 'order-1', title: 'PHAROS-11: Connect Hetzner Cloud for managed provisioning' })
    if (path === '/api/work-orders' && method === 'POST') {
      const order: WorkOrder = { node_id: 'order-1', status: 'draft', revision: 1, assignee_principal_id: String(body!.assignee_principal_id), criteria: (body!.criteria as string[]).map((description, i) => ({ id: `criterion-${i}`, description, checked_at: null })) }
      state.orders.push(order)
      return json(order, 201)
    }
    if (path === '/api/work-orders/order-1') {
      if (method === 'PATCH') {
        if (body!.expected_revision !== state.orders[0].revision) return json({ error: 'revision conflict' }, 409)
        Object.assign(state.orders[0], body, { revision: state.orders[0].revision + 1 })
      }
      return json(state.orders[0])
    }
    if (path === '/api/work-orders/order-1/runs' && method === 'POST') {
      if (state.failQueue) return json({ error: 'Temporary queue failure' }, 503)
      const run: AgentRun = { id: 'run-1', work_order_id: 'order-1', agent_principal_id: String(body!.agent_principal_id), model_profile_id: String(body!.model_profile_id), status: 'queued', model_evidence: 'unverified', requested_model: profile.model, input_tokens: 0, output_tokens: 0, cost_micros: 0, created_at: new Date().toISOString() }
      if (body!.requested_account_id) run.requested_account_id = String(body!.requested_account_id)
      state.runs.push(run)
      return json(run, 201)
    }
    return route.fallback()
  })
  function claim(register = false) {
    state.runs[0].status = 'starting'
    if (register) state.sessions.push({ id: 'managed-1', project_id: 'p-pharos', agent_principal_id: agentId, run_id: 'run-1', ticket_node_id: 'n-1', work_order_id: 'order-1', parent_harness_session_id: null, harness: 'codex', host: 'workstation', management_mode: 'managed', role: 'worker', work_shape: 'ship', advertised_capabilities: ['interrupt', 'stop'], phase: 'starting', activity: 'unknown', activity_sequence: 1, revision: 1, heartbeat_at: new Date().toISOString(), stopped_at: null, stop_reason: null, created_at: new Date().toISOString(), project: { id: 'p-pharos', key: 'PHAROS', title: 'Pharos' }, ticket: { id: 'n-1', key: 'PHAROS-11', title: 'Connect Hetzner Cloud for managed provisioning' }, agent: { id: agentId, name: 'Studio builder' } })
  }
  return { state, calls, claim, account, profile, agentId }
}
