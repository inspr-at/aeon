// SPDX-License-Identifier: AGPL-3.0-only
// R3 face contract. Coordinator: mount JourneyView at /projects/:projectId and
// /projects/:projectId/journey/:stage. The view redirects the bare project to the
// server's default stage. No server module or cmd/aeon wiring belongs to P3.6.
import { api, APIError, getKinds, getNodes, type WorkNode } from './api.ts'

export const stages = ['inspire', 'shape', 'requirements', 'plan', 'build', 'deploy', 'access', 'live'] as const
export type Stage = typeof stages[number]
export type Profile = 'personal' | 'professional' | 'enterprise'
export const profiles: Record<Profile, { label: string; line: string }> = {
  personal: { label: 'Personal', line: 'One person, own time, learning along the way.' },
  professional: { label: 'Professional', line: 'A small business shipping a real job on a budget.' },
  enterprise: { label: 'Enterprise', line: 'A company with contracts, reviews and compliance.' },
}
export const stageLabel = (stage: Stage) => stage[0].toUpperCase() + stage.slice(1)
export const journeyPath = (project: string, stage: Stage) => `/projects/${encodeURIComponent(project)}/journey/${stage}`
export interface JourneyStage { key: Stage; state: 'done' | 'current' | 'later' | 'skipped' | 'blocked'; gate_approval_id?: string | null; handoff_id?: string | null }
export type Action = 'confirm_brief' | 'go' | 'reduce_scope' | 'park' | 'drop' | 'reopen' | 'start_build' | 'approve_candidate' | 'reject_candidate' | 'approve_deploy' | 'retry_deploy' | 'approve_permit' | 'plan_next_release'
export interface Journey {
  project_node_id: string; profile: Profile; revision: number; stage: Stage; stages: JourneyStage[]
  requirements_revision: number; current_release_id?: string | null
  next_action: { key: Exclude<Action, 'go' | 'reduce_scope' | 'park' | 'drop' | 'reject_candidate'> | 'continue_intake' | 'decide' | 'approve_requirements' | 'wait_for_build'; label: string; stage: Stage; available: boolean; reason?: string; approval_request_id?: string | null }
}
export interface Requirement { node_id: string; project_node_id: string; kind: 'functional' | 'nonfunctional'; revision: number; status: 'draft' | 'agreed' | 'superseded'; title: string; feature_node_id?: string | null; generated_ticket_ids?: string[] }
export interface WalkerFeature { feature_node_id: string; epic_key: string; title: string; selection: 'empty' | 'none' | 'some' | 'all'; included_count?: number; open_count?: number }
export interface WalkerTicket { ticket_node_id: string; key: string; title: string; feature_node_id?: string | null; included: boolean; position: number; estimated_hours?: number | null; screen_node_ids?: string[] }
export interface ReleaseWalker { release_node_id: string; project_node_id: string; state: 'planning' | 'building' | 'candidate' | 'deploying' | 'refused' | 'access' | 'released' | 'superseded'; revision: number; features: WalkerFeature[]; tickets: WalkerTicket[] }
export interface PlanWrite { expected_revision: number; ordered_ticket_ids: string[]; included_ticket_ids: string[] }
export interface IntakeSource { id: string; project_node_id: string; kind: 'url' | 'file' | 'note' | 'conversation'; label: string; locator?: string; file_id?: string; content_sha256: string; created_at: string }
export interface IntakeTurn { id: string; source_id: string; ordinal: number; speaker: 'person' | 'agent'; speaker_principal_id?: string; body: string; created_at: string }
export interface IntakeDraft { id: string; kind: 'brief' | 'requirement'; requirement_kind?: Requirement['kind']; title: string; body: string; base_event_id: number; status: 'proposed' | 'accepted' | 'rejected'; target_node_id?: string; proposed_at: string; citations: { source_id: string; turn_id?: string; locator: string }[] }
export interface Intake { sources: IntakeSource[]; turns: IntakeTurn[]; drafts: IntakeDraft[] }
export interface Handoff { id: string; project_node_id: string; release_node_id: string; stage: 'deploy' | 'access'; operation: 'prepare' | 'apply' | 'deploy' | 'verify'; plugin_id: string; attempt: number; authority_epoch: number; state: 'requested' | 'active' | 'blocked' | 'succeeded' | 'failed' | 'revoked'; expires_at: string; result?: { outcome: 'succeeded' | 'failed'; blocker_code?: string; completed_at: string } }
async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const response = await api(path, { method, ...(body === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }) })
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    throw new APIError(response.status, typeof data?.error === 'string' ? data.error : `Request failed (${response.status})`)
  }
  return response.json()
}
const root = (project: string) => `/projects/${encodeURIComponent(project)}`
const releaseRoot = (project: string, release: string) => `${root(project)}/releases/${encodeURIComponent(release)}`
export const getJourney = (project: string) => request<Journey>(`${root(project)}/journey`)
export const putProfile = (project: string, profile: Profile, expected_revision: number) => request<Journey>(`${root(project)}/journey/profile`, 'PUT', { profile, expected_revision })
export const postAction = (project: string, body: { action: Action; expected_revision: number; idempotency_key: string; approval_request_id?: string; release_id?: string; reason?: string }) => request<Journey>(`${root(project)}/journey/actions`, 'POST', body)
export const getRequirements = (project: string) => request<Requirement[]>(`${root(project)}/requirements`)
export const addRequirement = (project: string, body: { kind: Requirement['kind']; title: string; body: string; expected_revision: number; idempotency_key: string }) => request<Requirement>(`${root(project)}/requirements`, 'POST', body)
export const agreeRequirements = (project: string, body: { expected_revision: number; approval_request_id: string; idempotency_key: string }) => request<Requirement[]>(`${root(project)}/requirements/agree`, 'POST', body)
export const getWalker = (project: string, release: string) => request<ReleaseWalker>(`${releaseRoot(project, release)}/walker`)
export const putPlan = (project: string, release: string, body: PlanWrite) => request<ReleaseWalker>(`${releaseRoot(project, release)}/plan`, 'PUT', body)
export const getIntake = (project: string) => request<Intake>(`${root(project)}/intake`)
export const acceptDraft = (project: string, draft: IntakeDraft) => request<IntakeDraft>(`${root(project)}/intake/drafts/${encodeURIComponent(draft.id)}/accept`, 'POST', { expected_base_event_id: draft.base_event_id })
export const getHandoff = (id: string) => request<Handoff>(`/stage-handoffs/${encodeURIComponent(id)}`)
export const orderedTickets = (walker: ReleaseWalker) => [...walker.tickets].sort((a, b) => a.position - b.position)
export function featureState(tickets: WalkerTicket[]) {
  const included = tickets.filter(t => t.included).length
  return !tickets.length ? 'empty' : included === 0 ? 'none' : included === tickets.length ? 'all' : 'some'
}
export function featurePicks(tickets: WalkerTicket[], remembered?: string[]): string[] {
  const state = featureState(tickets)
  return state === 'all' ? [] : state === 'none' && remembered ? tickets.filter(t => remembered.includes(t.ticket_node_id)).map(t => t.ticket_node_id) : tickets.map(t => t.ticket_node_id)
}
export function ticketGroups(walker: ReleaseWalker) {
  const tickets = orderedTickets(walker)
  const groups: { id: string; feature?: WalkerFeature; tickets: WalkerTicket[] }[] = walker.features.map(feature => ({ id: feature.feature_node_id, feature, tickets: tickets.filter(t => t.feature_node_id === feature.feature_node_id) }))
  const ungrouped = tickets.filter(t => !walker.features.some(f => f.feature_node_id === t.feature_node_id))
  if (ungrouped.length) groups.push({ id: '', feature: undefined, tickets: ungrouped })
  return groups
}
// R1 nodes remain the authoritative screen content. No generated image or
// external URL is loaded implicitly. Comparison uses two explicitly linked nodes.
export type ScreenNode = WorkNode

// History stays in R1 release nodes, independently of the current R3 walker.
export async function getReleaseHistory(project: string): Promise<WorkNode[]> {
  const { items: kinds } = await getKinds()
  const kind = kinds.find(k => k.slug === 'release')
  if (!kind) return []
  const items: WorkNode[] = []
  let cursor: string | undefined
  do {
    const page = await getNodes({ parent_id: project, include_descendants: true, kind_id: kind.id, sort: 'created_at', direction: 'desc', cursor, limit: 200 })
    items.push(...page.items)
    cursor = page.next_cursor ?? undefined
  } while (cursor)
  return items
}
