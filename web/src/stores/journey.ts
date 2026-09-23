// SPDX-License-Identifier: AGPL-3.0-only
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { APIError, getNode, type WorkNode } from '../lib/api'
import { listApprovals, decideApproval, message, type Approval } from '../lib/agents'
import * as wire from '../lib/journey'

export const useJourney = defineStore('journey', () => {
  const projectId = ref(''), project = ref<WorkNode>(), journey = ref<wire.Journey>()
  const requirements = ref<wire.Requirement[]>([]), intake = ref<wire.Intake>({ sources: [], turns: [], drafts: [] })
  const walker = ref<wire.ReleaseWalker>(), release = ref<WorkNode>(), handoffs = ref<wire.Handoff[]>([]), approvals = ref<Approval[]>([])
  const loading = ref(false), busy = ref(false), error = ref(''), stale = ref(false), notice = ref('')
  const partial = new Map<string, string[]>()
  let generation = 0
  const groups = computed(() => walker.value ? wire.ticketGroups(walker.value) : [])
  const tickets = computed(() => walker.value ? wire.orderedTickets(walker.value) : [])
  const selected = computed(() => tickets.value.filter(t => t.included))
  const planning = computed(() => walker.value?.state === 'planning' && !busy.value && !loading.value && !stale.value)
  // A failed/ambiguous mutation must be refreshed before another command. Never
  // retry an approval or advance automatically, and never optimistically advance.
  async function load(id = projectId.value) {
    if (!id) return
    const changed = projectId.value !== id
    const request = ++generation
    if (changed) {
      projectId.value = id; project.value = undefined; journey.value = undefined; walker.value = undefined; release.value = undefined
      requirements.value = []; intake.value = { sources: [], turns: [], drafts: [] }; handoffs.value = []; approvals.value = []; partial.clear(); notice.value = ''
    }
    loading.value = true
    try {
      const j = await wire.getJourney(id)
      const [p, req, sources, gates, w, r, h] = await Promise.all([
        getNode(id), wire.getRequirements(id), wire.getIntake(id), listApprovals(),
        j.current_release_id ? wire.getWalker(id, j.current_release_id) : undefined,
        j.current_release_id ? getNode(j.current_release_id) : undefined,
        Promise.all([...new Set(j.stages.flatMap(s => s.handoff_id ? [s.handoff_id] : []))].map(wire.getHandoff)),
      ])
      if (request !== generation) return
      if (walker.value?.release_node_id !== w?.release_node_id) partial.clear()
      project.value = p; journey.value = j; requirements.value = req; intake.value = sources; approvals.value = gates; walker.value = w; release.value = r; handoffs.value = h
      error.value = ''; stale.value = false
    } catch (e) { if (request === generation) { error.value = message(e); stale.value = true } }
    finally { if (request === generation) loading.value = false }
  }
  async function mutate(operation: () => Promise<unknown>, success: string) {
    if (busy.value || loading.value || stale.value) return false
    const id = projectId.value
    busy.value = true; error.value = ''; notice.value = ''
    try {
      await operation()
      if (id !== projectId.value) return false
      await load(id); notice.value = success
      // The write succeeded even if the follow-up read failed. Clear submitted
      // drafts so a refresh cannot accidentally submit the same intent twice.
      return true
    } catch (e) {
      if (id === projectId.value) { stale.value = true; error.value = e instanceof APIError && e.status === 409 ? 'This project changed. Refresh before trying again; the change was not applied.' : message(e) }
      return false
    } finally { busy.value = false }
  }
  function profile(value: wire.Profile) {
    const j = journey.value
    return j ? mutate(() => wire.putProfile(projectId.value, value, j.revision), 'Profile saved. Future gates re-evaluated.') : Promise.resolve(false)
  }
  function action(action: wire.Action, reason?: string) {
    const j = journey.value
    if (!j) return Promise.resolve(false)
    const body = { action, expected_revision: j.revision, idempotency_key: crypto.randomUUID(), approval_request_id: j.next_action.approval_request_id ?? undefined, release_id: j.current_release_id ?? undefined, reason }
    return mutate(() => wire.postAction(projectId.value, body), 'Action recorded.')
  }
  function agree() {
    const j = journey.value, approval = j?.next_action.approval_request_id
    if (!j || !approval) return Promise.resolve(false)
    return mutate(() => wire.agreeRequirements(projectId.value, { expected_revision: j.revision, approval_request_id: approval, idempotency_key: crypto.randomUUID() }), 'Requirements agreed.')
  }
  function createRequirement(kind: wire.Requirement['kind'], title: string, body: string) {
    const j = journey.value
    if (!j) return Promise.resolve(false)
    return mutate(() => wire.addRequirement(projectId.value, { kind, title, body, expected_revision: j.revision, idempotency_key: crypto.randomUUID() }), 'Draft requirement added.')
  }
  function accept(draft: wire.IntakeDraft) { return mutate(() => wire.acceptDraft(projectId.value, draft), 'Cited draft accepted. Existing edits are preserved.') }
  function decide(id: string, decision: 'approved' | 'denied', reason: string) { return mutate(() => decideApproval(id, decision, reason), 'Gate decision recorded. Review the next action.') }
  function savePlan(included: string[], ordered = tickets.value.map(t => t.ticket_node_id)) {
    const w = walker.value
    if (!w || !planning.value) return Promise.resolve(false)
    return mutate(() => wire.putPlan(projectId.value, w.release_node_id, { expected_revision: w.revision, ordered_ticket_ids: ordered, included_ticket_ids: included }), 'Release plan saved.')
  }
  async function toggleTicket(id: string) {
    const ticket = tickets.value.find(t => t.ticket_node_id === id)
    if (!ticket) return
    const next = tickets.value.map(t => t.ticket_node_id === id ? { ...t, included: !t.included } : t)
    const group = next.filter(t => (t.feature_node_id ?? '') === (ticket.feature_node_id ?? ''))
    const remembered = group.filter(t => t.included).map(t => t.ticket_node_id)
    if (await savePlan(next.filter(t => t.included).map(t => t.ticket_node_id)) && wire.featureState(group) === 'some') partial.set(ticket.feature_node_id ?? '', remembered)
  }
  async function toggleFeature(id: string) {
    const group = groups.value.find(g => g.id === id)
    if (!group) return
    if (wire.featureState(group.tickets) === 'some') partial.set(id, group.tickets.filter(t => t.included).map(t => t.ticket_node_id))
    const picks = wire.featurePicks(group.tickets, partial.get(id))
    const others = selected.value.filter(t => !group.tickets.some(g => g.ticket_node_id === t.ticket_node_id)).map(t => t.ticket_node_id)
    await savePlan([...others, ...picks])
  }
  function dispose() { generation++; projectId.value = ''; journey.value = undefined; loading.value = false; partial.clear() }
  return { projectId, project, journey, requirements, intake, walker, release, handoffs, approvals, loading, busy, error, stale, notice, groups, tickets, selected, planning, load, profile, action, agree, createRequirement, accept, decide, savePlan, toggleTicket, toggleFeature, dispose }
})
