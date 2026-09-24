// SPDX-License-Identifier: AGPL-3.0-only
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { APIError, getNode } from '../lib/api'
import {
  decideApproval, getControl, getRun, listAccounts, listApprovals, listMessages, listModels, listSessions, listTargets, message,
  requestControl, revokeApproval, sendMessage, setAccountState,
  type AgentAccount, type AgentRun, type Approval, type HarnessSession, type ModelProfile, type ProjectMessage, type SessionControl,
} from '../lib/agents'
import { agentName, groupSessions, harnessLabel, heldRequests, needsYou, pendingApprovals, runModel, sessionStatus, type SessionStatus } from '../lib/agentState'
import { toast } from '../lib/toast'
import { useProjects } from './projects'

// forbidden: not for this person; error: the read failed (any other status, including 404).
export type Availability = 'idle' | 'ready' | 'forbidden' | 'error'
export interface NodeRef { id: string; key: string; title: string }
export interface SessionView {
  session: HarnessSession; status: SessionStatus; name: string; harness: string; account: string; model: string
  run?: AgentRun; projectKey: string; projectTitle: string; ticket: (NodeRef & { href: string }) | null
}

const FAN_OUT = 6
const MESSAGE_PAGES = 20
// Per-project reads: the first project doubles as a probe, so a server without the
// surface costs one request, and one failing project never hides the others.
async function perProject<T>(ids: string[], read: (id: string) => Promise<T>): Promise<{ state: Availability; results: Map<string, T>; error: string }> {
  const results = new Map<string, T>()
  if (!ids.length) return { state: 'ready', results, error: '' }
  try { results.set(ids[0], await read(ids[0])) } catch (e) {
    if (e instanceof APIError && e.status === 403) return { state: 'forbidden', results, error: '' }
    return { state: 'error', results, error: e instanceof APIError ? `The server answered “${e.message}” (${e.status}).` : message(e) }
  }
  let next = 1
  const worker = async () => { while (next < ids.length) { const id = ids[next++]; try { results.set(id, await read(id)) } catch { /* shown as missing */ } } }
  await Promise.all(Array.from({ length: Math.min(FAN_OUT, ids.length - 1) }, worker))
  return { state: 'ready', results, error: '' }
}
async function all<T>(items: T[], work: (item: T) => Promise<void>) {
  let next = 0
  const worker = async () => { while (next < items.length) { const item = items[next++]; try { await work(item) } catch { /* optional detail */ } } }
  await Promise.all(Array.from({ length: Math.min(FAN_OUT, items.length) }, worker))
}
async function allMessages(projectId: string) {
  const out: ProjectMessage[] = []
  let after = 0
  for (let page = 0; page < MESSAGE_PAGES; page++) {
    const result = await listMessages(projectId, after)
    out.push(...result.items)
    if (result.items.length < 10 || result.next_after === after) break
    after = result.next_after
  }
  return out
}
const terminal = (run?: AgentRun) => !!run && ['completed', 'failed', 'cancelled', 'ownership_lost'].includes(run.status)

export const useAgents = defineStore('agents', () => {
  const projects = useProjects()
  const now = ref(Date.now())
  const sessions = ref<HarnessSession[]>([])
  const sessionsState = ref<Availability>('idle')
  const sessionsError = ref('')
  const approvals = ref<Approval[]>([])
  const approvalsState = ref<Availability>('idle')
  const approvalsError = ref('')
  const accounts = ref<AgentAccount[]>([])
  const accountsState = ref<Availability>('idle')
  const messagingState = ref<Availability>('idle')
  const messages = ref<Record<string, ProjectMessage[]>>({})
  const addresses = ref<Record<string, string>>({})
  const runs = ref<Record<string, AgentRun>>({})
  const nodes = ref<Record<string, NodeRef>>({})
  const models = ref<ModelProfile[]>([])
  const controls = ref<Record<string, SessionControl>>({})
  const loading = ref(false)
  const loaded = ref(false)
  const projectLoadedAt = new Map<string, number>()
  let needsAt = 0
  let messagesAt = 0

  const activeProjectIds = () => projects.projects.filter(p => !p.archived).map(p => p.id)

  // ---------- Reads ----------
  async function refreshApprovals() {
    try { approvals.value = await listApprovals(); approvalsState.value = 'ready'; approvalsError.value = '' }
    catch (e) {
      approvalsState.value = e instanceof APIError && e.status === 403 ? 'forbidden' : 'error'
      approvalsError.value = message(e)
    }
  }
  async function refreshAccounts() {
    try { accounts.value = await listAccounts(); accountsState.value = 'ready' }
    catch (e) { accountsState.value = e instanceof APIError && e.status === 403 ? 'forbidden' : 'error' }
  }
  async function refreshModels() { if (!models.value.length) try { models.value = await listModels() } catch { /* model names fall back to the run's */ } }
  // Messages are paged per project from the oldest, so a full scan runs at most
  // every 30 seconds; a sent message refreshes its own thread at once.
  async function refreshMessages() {
    if (Date.now() - messagesAt < 30_000) return
    messagesAt = Date.now()
    const result = await perProject(activeProjectIds(), async id => {
      const [items, targets] = await Promise.all([allMessages(id), listTargets(id).catch(() => [])])
      return { items, targets }
    })
    messagingState.value = result.state
    if (result.state !== 'ready') return
    const next: Record<string, ProjectMessage[]> = {}
    const names: Record<string, string> = { ...addresses.value }
    for (const [id, { items, targets }] of result.results) {
      next[id] = items
      for (const item of items) if (item.to && !names[item.recipient_principal_id]) names[item.recipient_principal_id] = item.to
      for (const target of targets) if (target.enabled && target.role !== 'simple_fallback') names[target.principal_id] = target.address
    }
    messages.value = next
    addresses.value = names
  }
  async function refreshSessions() {
    const result = await perProject(activeProjectIds(), listSessions)
    sessionsState.value = result.state
    sessionsError.value = result.error
    if (result.state !== 'ready') return
    sessions.value = [...result.results.values()].flat()
    for (const id of result.results.keys()) projectLoadedAt.set(id, Date.now())
    await details(sessions.value)
  }
  // Runs (account, model, telemetry) and bound tickets, for live and recent sessions.
  async function details(list: HarnessSession[]) {
    const recent = Date.now() - 48 * 3_600_000
    const runIds = [...new Set(list.filter(s => s.run_id && (!s.stopped_at || Date.parse(s.stopped_at) > recent) && !terminal(runs.value[s.run_id])).map(s => s.run_id!))]
    const ticketIds = [...new Set(list.map(s => s.ticket_node_id).filter((id): id is string => !!id && !nodes.value[id]))]
    await Promise.all([
      all(runIds, async id => { const run = await getRun(id); runs.value = { ...runs.value, [id]: run } }),
      all(ticketIds, async id => { const node = await getNode(id); nodes.value = { ...nodes.value, [id]: { id, key: node.key, title: node.title } } }),
    ])
  }
  async function resourceNodes() {
    const ids = [...new Set(approvals.value.filter(a => a.resource_kind === 'node' && a.resource_id && !nodes.value[a.resource_id]).map(a => a.resource_id!))]
    await all(ids, async id => { const node = await getNode(id); nodes.value = { ...nodes.value, [id]: { id, key: node.key, title: node.title } } })
  }

  async function loadAll() {
    loading.value = true
    try {
      await projects.load()
      await Promise.all([refreshApprovals(), refreshSessions(), refreshMessages(), refreshAccounts(), refreshModels()])
      await resourceNodes()
      loaded.value = true
      needsAt = Date.now()
    } finally { loading.value = false; now.value = Date.now() }
  }
  // The header badge: approvals and held action requests, at most every 30 seconds.
  async function loadNeeds(force = false) {
    if (!force && Date.now() - needsAt < 30_000) return
    needsAt = Date.now()
    await projects.load()
    await Promise.all([refreshApprovals(), refreshMessages()])
    now.value = Date.now()
  }
  // One project's sessions, for the ticket panel; cached for 20 seconds.
  async function ensureProject(projectId: string) {
    if (Date.now() - (projectLoadedAt.get(projectId) ?? 0) < 20_000) return
    projectLoadedAt.set(projectId, Date.now())
    try {
      const list = await listSessions(projectId)
      sessions.value = [...sessions.value.filter(s => s.project_id !== projectId), ...list]
      if (sessionsState.value === 'idle') sessionsState.value = 'ready'
      await details(list)
    } catch { /* the ticket panel simply shows no sessions */ }
  }
  async function refreshThread(projectId: string) {
    try { const list = await allMessages(projectId); messages.value = { ...messages.value, [projectId]: list }; if (messagingState.value === 'idle') messagingState.value = 'ready' }
    catch (e) { if (messagingState.value !== 'ready') messagingState.value = e instanceof APIError && e.status === 403 ? 'forbidden' : 'error' }
  }
  async function runsFor(runIds: string[]) {
    await all(runIds.filter(id => !runs.value[id] || !terminal(runs.value[id])), async id => { const run = await getRun(id); runs.value = { ...runs.value, [id]: run } })
  }

  // ---------- Derived ----------
  const pending = computed(() => pendingApprovals(approvals.value, now.value))
  const held = computed(() => Object.entries(messages.value).flatMap(([projectId, list]) => heldRequests(list).map(m => ({ ...m, projectId }))))
  const needsCount = computed(() => pending.value.length + held.value.length)
  const accountById = computed(() => new Map(accounts.value.map(a => [a.id, a])))
  const modelById = computed(() => new Map(models.value.map(m => [m.id, m])))
  function viewOf(session: HarnessSession): SessionView {
    const run = session.run_id ? runs.value[session.run_id] : undefined
    const project = projects.byId(session.project_id)
    const node = session.ticket_node_id ? nodes.value[session.ticket_node_id] : undefined
    return {
      session, status: sessionStatus(session, now.value, needsYou(session, pending.value, held.value)), name: agentName(session, addresses.value), harness: harnessLabel(session.harness),
      account: run?.account_id ? accountById.value.get(run.account_id)?.label ?? '' : '',
      model: (run?.model_profile_id ? modelById.value.get(run.model_profile_id)?.slug : '') || runModel(run),
      run, projectKey: project?.routeKey ?? '', projectTitle: project?.title ?? '',
      ticket: node && project ? { ...node, href: `/p/${encodeURIComponent(project.routeKey)}/${encodeURIComponent(node.key)}` } : null,
    }
  }
  const views = computed(() => sessions.value.map(viewOf))
  const grouped = computed(() => {
    const out: Record<SessionStatus['group'], SessionView[]> = { needs: [], working: [], idle: [], stopped: [] }
    const buckets = groupSessions(sessions.value, now.value, s => needsYou(s, pending.value, held.value))
    const byId = new Map(views.value.map(v => [v.session.id, v]))
    for (const group of ['needs', 'working', 'idle', 'stopped'] as const) out[group] = buckets[group].map(entry => byId.get(entry.session.id)!).filter(Boolean)
    return out
  })
  const byAgent = (principalId: string) => views.value.filter(v => v.session.agent_principal_id === principalId)
  const forTicket = (nodeId: string) => views.value.filter(v => v.session.ticket_node_id === nodeId && v.status.group !== 'stopped')
  // Who asks: the agent's name from a session or message address, else a short id.
  function askerName(principalId: string) {
    const session = sessions.value.find(s => s.agent_principal_id === principalId)
    if (session) return { name: agentName(session, addresses.value), harness: harnessLabel(session.harness), sessionId: session.id }
    const address = addresses.value[principalId]
    if (address) { const [harness, name] = address.split(':'); return { name, harness: harnessLabel(harness), sessionId: '' } }
    return { name: `Agent ${principalId.slice(0, 8)}`, harness: '', sessionId: '' }
  }
  const thread = (session: HarnessSession) => (messages.value[session.project_id] ?? [])
    .filter(m => m.sender_principal_id === session.agent_principal_id || m.recipient_principal_id === session.agent_principal_id)
  const addressOf = (principalId: string) => addresses.value[principalId] ?? ''

  // ---------- Writes ----------
  async function decide(approval: Approval, decision: 'approved' | 'denied', reason: string) {
    const updated = await decideApproval(approval.id, decision, reason)
    approvals.value = approvals.value.map(a => a.id === approval.id ? { ...a, ...updated, decision } : a)
  }
  async function revoke(approval: Approval) {
    await revokeApproval(approval.id)
  }
  async function control(view: SessionView, kind: SessionControl['kind']) {
    const { session } = view
    const issued = await requestControl(session.project_id, session.id, kind)
    controls.value = { ...controls.value, [session.id]: issued }
    void follow(session, issued, view.name)
    return issued
  }
  // Controls are claimed and completed by the agent; follow for up to a minute.
  async function follow(session: HarnessSession, issued: SessionControl, name: string) {
    for (let i = 0; i < 30; i++) {
      await new Promise(resolve => setTimeout(resolve, 2000))
      let current: SessionControl
      try { current = await getControl(session.project_id, session.id, issued.id) } catch { return }
      controls.value = { ...controls.value, [session.id]: current }
      if (current.state === 'completed') {
        const what = current.kind === 'stop' ? 'stop' : 'interrupt'
        if (current.outcome === 'applied') toast(`${name} applied the ${what}.`)
        else toast(`${name} refused the ${what}${current.reason ? `: ${current.reason}` : '.'}`, { tone: 'error' })
        void ensureProject(session.project_id)
        return
      }
    }
  }
  async function send(session: HarnessSession, to: string, body: string, level: 'simple' | 'steer', replyTo?: string) {
    await sendMessage(session.project_id, { to, body, idempotency_key: crypto.randomUUID(), expects_reply: false, is_action_request: false, delivery_level: level, ...(replyTo ? { reply_to: replyTo } : {}) })
    await refreshThread(session.project_id)
  }
  async function setAccount(account: AgentAccount, state: AgentAccount['state']) {
    const updated = await setAccountState(account.id, state)
    accounts.value = accounts.value.map(a => a.id === account.id ? { ...a, ...updated } : a)
  }
  function tick() { now.value = Date.now() }

  return {
    now, sessions, sessionsState, sessionsError, approvals, approvalsState, approvalsError, accounts, accountsState, messagingState, messages, runs, nodes, controls,
    loading, loaded, pending, held, needsCount, views, grouped,
    loadAll, loadNeeds, ensureProject, refreshApprovals, refreshSessions, refreshThread, runsFor, tick,
    viewOf, byAgent, forTicket, askerName, thread, addressOf, decide, revoke, control, send, setAccount,
  }
})
