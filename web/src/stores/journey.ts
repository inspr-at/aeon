// SPDX-License-Identifier: AGPL-3.0-only
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { APIError } from '../lib/api'
import type { Approval } from '../lib/agents'
import { agreeRequirements, getJourney, postAction, putProfile, type ActionKey, type Journey, type Profile } from '../lib/journey'
import { useAgents } from './agents'

// The journey projection per project (the header's compact stage and the
// Journey view share it). Every write carries the revision it was made on; a
// stale revision reloads the projection and says so instead of retrying.
const RELEASE_ACTIONS: ActionKey[] = ['start_build', 'mark_candidate', 'approve_candidate', 'reject_candidate', 'approve_deploy', 'retry_deploy', 'approve_permit', 'plan_next_release']
export class StaleJourney extends Error {}

export const useJourney = defineStore('journey', () => {
  const agents = useAgents()
  const journeys = ref<Record<string, Journey>>({})
  const errors = ref<Record<string, string>>({})
  const loading = ref<Record<string, boolean>>({})
  const busy = ref(false)
  const inflight = new Map<string, Promise<Journey | null>>()

  async function load(projectId: string, force = false): Promise<Journey | null> {
    if (!force && journeys.value[projectId]) return journeys.value[projectId]
    const running = inflight.get(projectId)
    if (running) return running
    const task = (async () => {
      loading.value = { ...loading.value, [projectId]: true }
      try {
        const journey = await getJourney(projectId)
        journeys.value = { ...journeys.value, [projectId]: journey }
        errors.value = { ...errors.value, [projectId]: '' }
        return journey
      } catch (e) {
        errors.value = { ...errors.value, [projectId]: e instanceof APIError && e.status === 404 ? 'This project has no journey.' : e instanceof Error ? e.message : 'The journey could not be loaded.' }
        return null
      } finally {
        loading.value = { ...loading.value, [projectId]: false }
        inflight.delete(projectId)
      }
    })()
    inflight.set(projectId, task)
    return task
  }
  // A 409 on the revision means someone else moved the journey: show the new state.
  async function guard<T>(projectId: string, write: () => Promise<T>): Promise<T> {
    busy.value = true
    try { return await write() } catch (e) {
      if (e instanceof APIError && e.status === 409 && /revision/i.test(e.message)) {
        await load(projectId, true)
        throw new StaleJourney('The journey changed meanwhile. It now shows the newest state; check it and try again.')
      }
      throw e
    } finally { busy.value = false }
  }
  function set(projectId: string, journey: Journey) { journeys.value = { ...journeys.value, [projectId]: journey } }

  async function act(projectId: string, action: ActionKey, options: { approval?: Approval | null; reason?: string } = {}) {
    const journey = journeys.value[projectId]
    if (!journey) throw new Error('The journey is not loaded yet.')
    const next = await guard(projectId, () => postAction(projectId, {
      action, expected_revision: journey.revision, idempotency_key: crypto.randomUUID(),
      approval_request_id: options.approval?.id ?? null,
      ...(RELEASE_ACTIONS.includes(action) ? { release_id: journey.current_release_id } : {}),
      ...(options.reason ? { reason: options.reason } : {}),
    }))
    set(projectId, next)
    return next
  }
  async function agree(projectId: string, approval: Approval) {
    const journey = journeys.value[projectId]
    if (!journey) throw new Error('The journey is not loaded yet.')
    if (approval.decision === null) await agents.decide(approval, 'approved', '')
    const result = await guard(projectId, () => agreeRequirements(projectId, { expected_revision: journey.revision, approval_request_id: approval.id, idempotency_key: crypto.randomUUID() }))
    await load(projectId, true)
    return result
  }
  async function profile(projectId: string, value: Profile) {
    const journey = journeys.value[projectId]
    if (!journey || journey.profile === value) return journey
    const next = await guard(projectId, () => putProfile(projectId, value, journey.revision))
    set(projectId, next)
    return next
  }
  return { journeys, errors, loading, busy, load, act, agree, profile, set }
})
