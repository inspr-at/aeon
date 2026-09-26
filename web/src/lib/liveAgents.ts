// SPDX-License-Identifier: AGPL-3.0-only
// Who is working where right now (AEON-184): the rules behind the live agents on
// the Projects page. GET /api/harness-sessions/live answers every visible
// project in one read; these helpers group it, keep only what is still fresh on
// the server's clock, order a project's agents, and say it in words for labels,
// screen readers and the live region. Free of Vue so it can be unit tested.
import { duration, harnessLabel } from './agentState.ts'
import type { Harness, NodeSummary } from './agents.ts'

export interface LiveAgent {
  project_id: string
  // Present only when the caller may open the session / know the agent (AEON-171).
  session_id?: string; principal_id?: string; name?: string
  harness: Harness; management_mode: 'managed' | 'unmanaged'; role: 'worker' | 'coordinator'
  phase: 'starting' | 'working' | 'stopping'; activity: 'busy' | 'unknown'
  ticket: NodeSummary | null; since: string; heartbeat_at: string
}
export interface LivePage { items: LiveAgent[]; at: string; fresh_seconds: number }

// Polling cadence: heartbeats arrive every minute, so 20 seconds keeps a card
// honest without asking often.
export const LIVE_POLL_MS = 20_000
export const LIVE_FRESH_MS = 120_000

// The server's clock is the one that counts: skew is how far this browser is ahead.
export const skewOf = (page: Pick<LivePage, 'at'>, receivedAt: number) => {
  const at = Date.parse(page.at)
  return Number.isNaN(at) ? 0 : receivedAt - at
}

// Agents that lead a project's line: one on a ticket before one without, workers
// before coordinators, then whoever started first.
export function byLead(a: LiveAgent, b: LiveAgent) {
  return Number(!a.ticket) - Number(!b.ticket)
    || Number(a.role === 'coordinator') - Number(b.role === 'coordinator')
    || Date.parse(a.since) - Date.parse(b.since)
    || (a.session_id ?? a.since).localeCompare(b.session_id ?? b.since)
}

// Project id -> its working agents, lead first; heartbeats older than the
// freshness window (on the server's clock) no longer count.
export function groupLive(items: LiveAgent[], serverNow: number, freshMs = LIVE_FRESH_MS) {
  const out = new Map<string, LiveAgent[]>()
  for (const item of items) {
    const beat = Date.parse(item.heartbeat_at)
    if (Number.isNaN(beat) || serverNow - beat > freshMs) continue
    const list = out.get(item.project_id)
    if (list) list.push(item); else out.set(item.project_id, [item])
  }
  for (const list of out.values()) list.sort(byLead)
  return out
}

// Two readings that show the same thing (heartbeats aside), so a poll that
// changes nothing re-renders nothing.
const shown = (a: LiveAgent) => [a.project_id, a.session_id, a.principal_id, a.name, a.harness, a.role, a.phase, a.activity, a.ticket?.id, a.ticket?.key, a.ticket?.title, a.since].join('\u0000')
export function sameLive(a: Map<string, LiveAgent[]>, b: Map<string, LiveAgent[]>) {
  if (a.size !== b.size) return false
  for (const [id, list] of a) {
    const other = b.get(id)
    if (!other || other.length !== list.length || list.some((agent, i) => shown(agent) !== shown(other[i]!))) return false
  }
  return true
}

// Who: the agent's name, else its harness ("Claude agent") when the caller may
// not know which agent it is.
export const who = (agent: LiveAgent) => agent.name || `${harnessLabel(agent.harness)} agent`
export const phaseLabel = (agent: Pick<LiveAgent, 'phase'>) => agent.phase === 'starting' ? 'Starting' : agent.phase === 'stopping' ? 'Stopping' : 'Working'
export const elapsedFor = (agent: Pick<LiveAgent, 'since'>, serverNow: number) => duration(serverNow - Date.parse(agent.since))

// One agent as a phrase: "hausv on HAUSV-887", "hausv, starting".
export function phrase(agent: LiveAgent) {
  const name = who(agent)
  if (agent.phase !== 'working') return `${name}, ${phaseLabel(agent).toLowerCase()}${agent.ticket ? ` on ${agent.ticket.key}` : ''}`
  return agent.ticket ? `${name} on ${agent.ticket.key}` : name
}
// "1 agent working: hausv on HAUSV-887", "2 agents working: hausv on HAUSV-887, camy".
export function liveSummary(agents: LiveAgent[]) {
  if (!agents.length) return ''
  return `${agents.length} ${agents.length === 1 ? 'agent' : 'agents'} working: ${agents.map(phrase).join(', ')}`
}

// The visible chip: the lead's name and ticket key, and how many more there are.
export function chipText(agents: LiveAgent[]) {
  const lead = agents[0]
  if (!lead) return { name: '', key: '', more: 0 }
  return { name: who(lead), key: lead.ticket?.key ?? '', more: agents.length - 1 }
}

// What the live region says when projects start or stop having agents at work:
// nothing on the first read, one sentence per change, a count when many change.
export function liveChanges(before: Map<string, LiveAgent[]> | null, after: Map<string, LiveAgent[]>, title: (projectId: string) => string | undefined) {
  if (!before) return ''
  const lines: string[] = []
  for (const [id, agents] of after) {
    if (before.get(id)?.length) continue
    const name = title(id)
    if (!name) continue
    lines.push(agents.length === 1 ? `${who(agents[0]!)} started working on ${name}.` : `${agents.length} agents started working on ${name}.`)
  }
  for (const [id, agents] of before) {
    if (!agents.length || after.get(id)?.length) continue
    const name = title(id)
    if (name) lines.push(`No agent is working on ${name} any more.`)
  }
  if (lines.length > 3) return `Agents changed in ${lines.length} projects.`
  return lines.join(' ')
}
