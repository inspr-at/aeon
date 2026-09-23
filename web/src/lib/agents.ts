// SPDX-License-Identifier: AGPL-3.0-only
// R2 HTTP surface. Daemon-only endpoints are deliberately absent from this UI.
import { api, APIError } from './api.ts'

export interface AgentAccount {
  id: string; account_key: string; harness: string; daemon_id: string; label: string
  registered_by_principal_id: string; state: 'available' | 'draining' | 'unavailable'
  max_parallel_runs?: number; last_probe_at?: string | null; last_probe_ok?: boolean | null; created_at: string
}
export interface AgentRun {
  id: string; work_order_id: string; agent_principal_id: string; model_profile_id?: string | null
  account_id?: string | null; status: 'queued' | 'starting' | 'running' | 'waiting' | 'completed' | 'failed' | 'cancelled' | 'ownership_lost'
  requested_model?: string | null; effective_model?: string | null; model_evidence: 'unverified' | 'vendor_reported'
  input_tokens: number; output_tokens: number; cost_micros: number
  started_at?: string | null; ended_at?: string | null; created_at: string
}
export interface Approval {
  id: string; agent_principal_id: string; scope: string; resource_kind: 'tenant' | 'node' | 'run'
  resource_id?: string; run_id?: string; rationale: string; expires_at: string; proposed_at: string
  decision: 'approved' | 'denied' | null; decided_by_principal_id?: string | null
}
export interface AllowanceWrite {
  starts_at: string; ends_at: string; unit: 'requests' | 'tokens' | 'cost_micros'
  allowance: number; pace_model: 'steady' | 'frontload' | 'unrestricted'; burst_ratio: number
}
export interface AllowanceWindow extends AllowanceWrite { id: string; account_id: string; used: number; reserved: number }

async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const response = await api(path, { method, ...(body === undefined ? {} : {
    headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }) })
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    throw new APIError(response.status, typeof data?.error === 'string' ? data.error : `Request failed (${response.status})`)
  }
  return response.json()
}
export const listAccounts = () => request<AgentAccount[]>('/agent-accounts')
export const setAccountState = (id: string, state: AgentAccount['state']) => request<AgentAccount>(`/agent-accounts/${encodeURIComponent(id)}`, 'PATCH', { state })
export const getRun = (id: string) => request<AgentRun>(`/runs/${encodeURIComponent(id)}`)
export const listApprovals = () => request<Approval[]>('/approvals?limit=200')
export const decideApproval = (id: string, decision: 'approved' | 'denied', reason: string) => request<Approval>(`/approvals/${encodeURIComponent(id)}/decision`, 'POST', { decision, reason })
export const revokeApproval = (id: string) => request<Approval>(`/approvals/${encodeURIComponent(id)}/revoke`, 'POST')
export const createWindow = (id: string, body: AllowanceWrite) => request<AllowanceWindow>(`/agent-accounts/${encodeURIComponent(id)}/windows`, 'POST', body)
export const message = (error: unknown) => error instanceof Error ? error.message : 'Request failed. Please retry.'
export const timestamp = (value?: string | null) => value ? new Date(value).toLocaleString() : 'Not reported'
export const uuidPattern = '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}'

// The cumulative pacing model from internal/agents/doc.go, bounded by the hard allowance.
export function paceFraction(model: AllowanceWrite['pace_model'], elapsed: number, burst: number) {
  const f = Math.min(1, Math.max(0, elapsed))
  const pace = model === 'steady' ? f : model === 'frontload' ? 1 - (1 - f) ** 2 : 1
  return Math.min(1, pace + burst)
}
