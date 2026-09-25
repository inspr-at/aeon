// SPDX-License-Identifier: AGPL-3.0-only
import { ref } from 'vue'
import { api } from './api'

interface Grant { role: { id: string; key: string; name: string } | null; permissions: string[] }
interface Effective { workspace: Grant; project: (Grant & { id: string }) | null }

const cache = new Map<string, Effective | null>()
const requests = new Map<string, Promise<void>>()
const revision = ref(0)
const keyOf = (projectId?: string) => projectId || ''

// A missing or stale answer grants nothing. The revision makes Vue computed
// callers update as soon as the server's effective set arrives.
export function can(permission: string, projectId?: string): boolean {
  revision.value
  const key = keyOf(projectId)
  const current = cache.get(key)
  if (current === undefined) { void refreshPermissions(projectId); return false }
  if (current === null) return false
  return current.workspace.permissions.includes(permission) || current.project?.permissions.includes(permission) === true
}

export async function refreshPermissions(projectId?: string): Promise<void> {
  const key = keyOf(projectId)
  if (requests.has(key)) return requests.get(key)
  const request = (async () => {
    try {
      const response = await api(`/me/permissions${projectId ? `?project_id=${encodeURIComponent(projectId)}` : ''}`)
      if (!response.ok) throw new Error('permissions unavailable')
      const body: Effective = await response.json()
      if (!Array.isArray(body.workspace?.permissions) || (projectId && body.project?.id !== projectId)) throw new Error('invalid permissions')
      cache.set(key, body)
    } catch { cache.set(key, null) }
    finally { requests.delete(key); revision.value++ }
  })()
  requests.set(key, request)
  return request
}

export function clearPermissions(): void { cache.clear(); revision.value++ }
export async function accessChanged(): Promise<void> {
  const projects = [...cache.keys()].filter(Boolean)
  clearPermissions()
  await Promise.all([refreshPermissions(), ...projects.map(id => refreshPermissions(id))])
}

if (typeof window !== 'undefined') {
  window.addEventListener('focus', () => { void accessChanged() })
}
