// SPDX-License-Identifier: AGPL-3.0-only
// Shared project groups (AEON-136) over the work fixtures: /api/project-groups
// with admin-only writes, their events and undo, and project archive/restore
// through PATCH /api/nodes/{projectId}. Register after mockWork: later routes win.
import type { Page } from '@playwright/test'
import type { Fixtures } from './work-fixtures'

export interface SharedGroupRow { id: string; name: string; position: number; project_ids: string[]; created_by: string; created_at: string; updated_at: string }
export interface GroupsWorld {
  groups: SharedGroupRow[]
  events: { id: number; type: string; before: unknown; after: unknown; undone: boolean }[]
  calls: { method: string; path: string; body: unknown }[]
  available: boolean
}
export const CLIENTS = 'c0000000-0000-4000-8000-00000000000c'
const at = '2026-09-20T09:00:00.000Z'

export function groupsWorld(options: { clients?: string[]; available?: boolean } = {}): GroupsWorld {
  return {
    groups: options.clients ? [{ id: CLIENTS, name: 'Clients', position: 0, project_ids: options.clients, created_by: '11111111-1111-4111-8111-111111111111', created_at: at, updated_at: at }] : [],
    events: [], calls: [], available: options.available ?? true,
  }
}

export async function mockProjectGroups(page: Page, data: Fixtures, world: GroupsWorld, options: { admin?: boolean } = {}) {
  let next = 5000
  const record = (type: string, before: unknown, after: unknown) => { const id = next++; world.events.push({ id, type, before, after, undone: false }); return id }
  const clone = <T>(value: T): T => structuredClone(value)
  const deny = () => ({ status: 403, json: { error: 'only workspace admins can change shared groups' } })
  await page.route('**/api/project-groups**', async route => {
    const request = route.request(), url = new URL(request.url()), method = request.method(), path = url.pathname
    let body: Record<string, unknown> = {}
    try { body = request.postDataJSON() ?? {} } catch { body = {} }
    world.calls.push({ method, path, body })
    if (!world.available) return route.fulfill({ status: 404, json: { error: 'not found' } })
    if (method === 'GET' && path === '/api/project-groups') return route.fulfill({ json: { items: [...world.groups].sort((a, b) => a.position - b.position) } })
    if (!options.admin) return route.fulfill(deny())
    if (method === 'POST' && path === '/api/project-groups') {
      const name = String(body.name ?? '').trim()
      if (world.groups.some(g => g.name.toLowerCase() === name.toLowerCase())) return route.fulfill({ status: 409, json: { error: 'a shared group with this name already exists' } })
      const ids = (body.project_ids as string[] | undefined) ?? []
      for (const g of world.groups) g.project_ids = g.project_ids.filter(id => !ids.includes(id))
      const group: SharedGroupRow = { id: `d0000000-0000-4000-8000-${String(next).padStart(12, '0')}`, name, position: Number(body.position ?? world.groups.length), project_ids: [...ids].sort(), created_by: '11111111-1111-4111-8111-111111111111', created_at: at, updated_at: at }
      world.groups.push(group)
      return route.fulfill({ status: 201, json: { group, event_id: record('project_group.created', null, clone(group)) } })
    }
    if (method === 'POST' && path === '/api/project-groups/assign') {
      const target = body.group_id as string | null, ids = body.project_ids as string[]
      const before = ids.map(id => ({ project_id: id, group_id: world.groups.find(g => g.project_ids.includes(id))?.id ?? null }))
      for (const g of world.groups) g.project_ids = g.project_ids.filter(id => !ids.includes(id))
      if (target) world.groups.find(g => g.id === target)!.project_ids.push(...ids)
      const after = ids.map(id => ({ project_id: id, group_id: target }))
      return route.fulfill({ json: { assignments: after, event_id: record('project_group.assigned', before, after) } })
    }
    const one = /^\/api\/project-groups\/([^/]+)$/.exec(path)
    const group = one ? world.groups.find(g => g.id === one[1]) : undefined
    if (!group) return route.fulfill({ status: 404, json: { error: 'group not found' } })
    if (method === 'PATCH') {
      const before = clone(group)
      if (typeof body.name === 'string') group.name = body.name.trim()
      return route.fulfill({ json: { group, event_id: record('project_group.updated', before, clone(group)) } })
    }
    if (method === 'DELETE') {
      world.groups.splice(world.groups.indexOf(group), 1)
      return route.fulfill({ json: { event_id: record('project_group.deleted', clone(group), null) } })
    }
    return route.fulfill({ status: 405, json: { error: 'method not allowed' } })
  })
  await page.route('**/api/events/*/undo', async route => {
    const id = Number(/\/api\/events\/(\d+)\/undo$/.exec(new URL(route.request().url()).pathname)?.[1])
    const event = world.events.find(e => e.id === id)
    if (!event) return route.fallback()
    world.calls.push({ method: 'POST', path: `/api/events/${id}/undo`, body: null })
    if (event.undone) return route.fulfill({ status: 409, json: { code: 'conflict', message: 'resource changed or event is not reversible' } })
    event.undone = true
    if (event.type === 'project_group.created') world.groups = world.groups.filter(g => g.id !== (event.after as SharedGroupRow).id)
    if (event.type === 'project_group.deleted') world.groups.push(clone(event.before as SharedGroupRow))
    if (event.type === 'project_group.updated') Object.assign(world.groups.find(g => g.id === (event.before as SharedGroupRow).id)!, { name: (event.before as SharedGroupRow).name })
    if (event.type === 'project_group.assigned') {
      for (const a of event.before as { project_id: string; group_id: string | null }[]) {
        for (const g of world.groups) g.project_ids = g.project_ids.filter(p => p !== a.project_id)
        if (a.group_id) world.groups.find(g => g.id === a.group_id)?.project_ids.push(a.project_id)
      }
    }
    return route.fulfill({ status: 201, json: { id: next++, type: event.type, undo_of: id } })
  })
  // Archiving and restoring a project is a PATCH of its node's state.
  await page.route('**/api/nodes/p-*', async route => {
    const request = route.request()
    if (request.method() !== 'PATCH') return route.fallback()
    const id = decodeURIComponent(new URL(request.url()).pathname.split('/')[3]!)
    const project = data.projects.find(p => p.id === id)
    if (!project) return route.fallback()
    const body = request.postDataJSON() as { state?: string }
    world.calls.push({ method: 'PATCH', path: `/api/nodes/${id}`, body })
    if (body.state) project.state = body.state
    return route.fulfill({ json: { id, key: project.key, kind_id: 'k-project', title: project.title, body: project.description, fields: {}, state: project.state, parent_id: null, position: '0', created_at: at, updated_at: new Date(Date.parse('2026-09-23T12:00:00Z') + 60_000 * world.calls.length).toISOString(), deleted_at: null } })
  })
}
