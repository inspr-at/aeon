// SPDX-License-Identifier: AGPL-3.0-only
import { ref } from 'vue'
import { api, sessionEnded } from './api.ts'

interface Grant { role: { id: string; key: string; name: string } | null; permissions: string[] }
interface Effective { workspace: Grant; project: (Grant & { id: string }) | null }

const cache = new Map<string, Effective | null>()
const requests = new Map<string, Promise<void>>()
const revision = ref(0)
let epoch = 0
const keyOf = (projectId?: string) => projectId || ''

// A missing or stale answer grants nothing. The revision makes Vue computed
// callers update as soon as the server's effective set arrives.
export function can(permission: string, projectId?: string): boolean {
  revision.value
  if (revoked) return false
  const key = keyOf(projectId)
  const current = cache.get(key)
  if (current === undefined) { void refreshPermissions(projectId); return false }
  if (current === null) return false
  return current.workspace.permissions.includes(permission) || current.project?.permissions.includes(permission) === true
}

export async function refreshPermissions(projectId?: string): Promise<void> {
  // After a 401 nothing asks by itself (no refetch loop); accessChanged may.
  if (revoked) return
  const key = keyOf(projectId)
  if (requests.has(key)) return requests.get(key)
  return ask(projectId)
}

// One request for one scope. Only the newest request per scope may answer, so a
// slow older answer never overwrites a newer one.
const asked = new Map<string, number>()
function ask(projectId?: string): Promise<void> {
  const key = keyOf(projectId)
  const started = epoch
  const turn = (asked.get(key) ?? 0) + 1
  asked.set(key, turn)
  const current = () => started === epoch && asked.get(key) === turn
  const request = (async () => {
    try {
      const response = await api(`/me/permissions${projectId ? `?project_id=${encodeURIComponent(projectId)}` : ''}`)
      if (response.status === 401) { if (current()) sessionGone(); return }
      if (!response.ok) throw new Error('permissions unavailable')
      const body: Effective = await response.json()
      if (!Array.isArray(body.workspace?.permissions) || (projectId && body.project?.id !== projectId)) throw new Error('invalid permissions')
      if (current()) { cache.set(key, body); revoked = false }
    } catch { if (current()) cache.set(key, null) }
    finally { if (current()) requests.delete(key); revision.value++ }
  })()
  requests.set(key, request)
  return request
}

export function clearPermissions(): void { epoch++; revoked = false; cache.clear(); requests.clear(); asked.clear(); revision.value++ }
// The session ended (a 401): nothing is granted any more and nothing is asked
// until the session is refreshed, so the page keeps its drafts without a request
// loop. Every answer in flight is dropped.
let revoked = false
export function revokePermissions(): void { epoch++; revoked = true; requests.clear(); for (const key of cache.keys()) cache.set(key, null); revision.value++ }
export function permissionsRevoked(): boolean { revision.value; return revoked }
// Every Access, Settings and permission request that meets a 401 ends up here:
// grants go at once, and the shell offers sign-in (App.vue).
export function sessionGone(): void { revokePermissions(); sessionEnded.handler?.() }
// My access may have changed (a role change, window focus): every scope already
// asked about is asked again. The answers on screen stay until the new ones
// arrive, so gated tabs and open sheets never flicker away; a failed answer
// still grants nothing. Sign-out uses clearPermissions instead.
export async function accessChanged(): Promise<void> {
  // Scopes still in flight are asked again too: their older answers must not land.
  // A revoked state stays until one of these answers with a 200.
  const keys = new Set(['', ...cache.keys(), ...requests.keys()])
  await Promise.all([...keys].map(key => ask(key || undefined)))
}

if (typeof window !== 'undefined') {
  window.addEventListener('focus', () => { void accessChanged() })
}

// ---------- Reading the answer (Settings > Access) ----------
// Views over the same cached answer, never a second source: my workspace role
// and permissions (to hide what I may not give away), and whether the server
// has answered at all (a server without the endpoint grants nothing).
export function permissionsKnown(projectId?: string): boolean {
  revision.value
  if (revoked) return true
  if (!cache.has(keyOf(projectId))) { void refreshPermissions(projectId); return false }
  return true
}
export function permissionsAvailable(): boolean { revision.value; return !!cache.get('') }
export function myWorkspaceRole(): Grant['role'] { revision.value; return cache.get('')?.workspace.role ?? null }
export function myPermissions(): Set<string> { revision.value; return new Set(cache.get('')?.workspace.permissions ?? []) }
