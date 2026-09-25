// SPDX-License-Identifier: AGPL-3.0-only
// can(permission, projectId?): the one question the web app asks about access
// (ADR-003). It is answered from GET /api/me/permissions: the workspace set at
// once, a project's set when that project is asked about. Both are cached,
// refreshed when the window comes back into focus and after any access change.
// No component reads role names; they ask can().
import { shallowRef } from 'vue'
import { api } from './api.ts'

export interface RoleRef { id: string; key: string; name: string }
interface Held { role: RoleRef | null; permissions: Set<string> }
interface Wire { workspace: { role: RoleRef | null; permissions: string[] }; project: { id: string; role: RoleRef | null; permissions: string[] } | null }

const workspace = shallowRef<Held | null>(null)
const projects = shallowRef(new Map<string, Held>())
const loads = new Map<string, Promise<void>>()
// False when the server does not answer /api/me/permissions (an older build):
// nothing is granted, and screens that need it say so.
export const permissionsAvailable = shallowRef(true)
export const permissionsLoaded = shallowRef(false)

const held = (role: RoleRef | null, permissions: string[]): Held => ({ role, permissions: new Set(permissions) })

async function fetchPermissions(projectId?: string): Promise<Wire | null> {
  const response = await api(`/me/permissions${projectId ? `?project_id=${encodeURIComponent(projectId)}` : ''}`)
  if (response.status === 404 || response.status === 405 || response.status === 501) { permissionsAvailable.value = false; return null }
  if (!response.ok) throw new Error(`Permissions unavailable (${response.status})`)
  permissionsAvailable.value = true
  return response.json() as Promise<Wire>
}

// Loads the workspace set, or a project's; one request per scope at a time.
export function loadPermissions(projectId?: string): Promise<void> {
  const key = projectId ?? ''
  const running = loads.get(key)
  if (running) return running
  const request = (async () => {
    try {
      const wire = await fetchPermissions(projectId)
      const ws = wire ? held(wire.workspace.role, wire.workspace.permissions) : held(null, [])
      workspace.value = ws
      if (projectId) projects.value = new Map(projects.value).set(projectId, wire?.project ? held(wire.project.role, wire.project.permissions) : held(null, []))
      permissionsLoaded.value = true
    } catch {
      // Unknown means not granted; the next focus or change asks again.
      if (!workspace.value) workspace.value = held(null, [])
      permissionsLoaded.value = true
    } finally { loads.delete(key) }
  })()
  loads.set(key, request)
  return request
}

// Asks again for the workspace and every project already asked about.
export async function refreshPermissions(): Promise<void> {
  const known = [...projects.value.keys()]
  await Promise.all([loadPermissions(), ...known.map(id => loadPermissions(id))])
}

// Whether I may do `permission`, in the workspace or on a project (the
// workspace set plus that project's). Unknown projects are asked for once.
export function can(permission: string, projectId?: string): boolean {
  if (!workspace.value && !loads.has('')) void loadPermissions()
  const ws = workspace.value?.permissions.has(permission) ?? false
  if (!projectId) return ws
  const project = projects.value.get(projectId)
  if (!project && !loads.has(projectId)) void loadPermissions(projectId)
  return ws || (project?.permissions.has(permission) ?? false)
}

// My workspace role, for a line such as "Your role: Admin".
export function myWorkspaceRole(): RoleRef | null { return workspace.value?.role ?? null }
export function myProjectRole(projectId: string): RoleRef | null { return projects.value.get(projectId)?.role ?? null }
// Everything I hold in the workspace (to compose roles without escalation).
export function myPermissions(): Set<string> { return workspace.value?.permissions ?? new Set() }

// Refresh on focus, at most every few seconds.
let lastFocus = 0
if (typeof window !== 'undefined') {
  window.addEventListener('focus', () => {
    if (Date.now() - lastFocus < 5000 || !workspace.value) return
    lastFocus = Date.now()
    void refreshPermissions()
  })
}

// Tests reset the cache between cases.
export function resetPermissions() { workspace.value = null; projects.value = new Map(); loads.clear(); permissionsLoaded.value = false; permissionsAvailable.value = true }
