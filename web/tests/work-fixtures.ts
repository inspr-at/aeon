// SPDX-License-Identifier: AGPL-3.0-only
// A small in-memory list API for the Projects and project list pages. It applies
// the B1 query semantics (within, kind, state, priority, assignee, q, hide_closed,
// sort, facets, cursor paging) so specs can assert on real behaviour.
import type { Page } from '@playwright/test'

export const me = { id: '11111111-1111-4111-8111-111111111111', name: 'Markus Barta' }
const mira = { id: '22222222-2222-4222-8222-222222222222', name: 'Mira Holm' }
const now = Date.parse('2026-09-23T12:00:00Z')
const ago = (hours: number) => new Date(now - hours * 3_600_000).toISOString()

export interface MockNode {
  id: string; key: string; kind_slug: string; title: string; body: string; state: string
  fields: Record<string, unknown>; parent_id: string | null; project: string
  created_at: string; updated_at: string
}
export interface MockOptions {
  conflictAlways?: string
  readOnly?: boolean
  failList?: boolean
  failProjects?: boolean
  delayList?: number
  failPatch?: boolean
  conflictOn?: string
  bigProject?: number
}

const CLOSED = ['done', 'cancelled', 'archived', 'delivered', 'accepted']
const STATE_ORDER = ['new', 'backlog', 'in_progress', 'active', 'qa', 'accepted', 'done', 'cancelled', 'archived']
const PRIORITY_ORDER = ['high', 'medium', 'low', 'none']

export function fixtures(options: MockOptions = {}) {
  const projects = [
    { id: 'p-pharos', key: 'PRJ-17', title: 'Pharos', state: 'active', classic: 'PHAROS', description: 'Fleet management and host access for the **INSPR** family.', last: ago(2) },
    { id: 'p-aeon', key: 'PRJ-35', title: 'Aeon', state: 'active', classic: 'AEON', description: '# Aeon\n\nThe successor of Paimos.', last: ago(30) },
    { id: 'p-glint', key: 'PRJ-28', title: 'Glint', state: 'archived', classic: 'GLINT', description: 'Glanceable ticket context.', last: ago(24 * 40) },
    { id: 'p-frozen', key: 'PRJ-26', title: 'Studio infrastructure', state: 'frozen', classic: '', description: '', last: ago(24 * 9) },
  ]
  const nodes: MockNode[] = []
  const add = (node: Partial<MockNode> & Pick<MockNode, 'id' | 'key' | 'kind_slug' | 'title' | 'state' | 'project'>) => {
    const full: MockNode = { body: '', fields: {}, parent_id: node.project, created_at: ago(24 * 20), updated_at: ago(5), ...node }
    nodes.push(full)
    return full
  }
  const epic = add({ id: 'n-epic', key: 'PHAROS-10', kind_slug: 'epic', title: 'Guarded multi-cloud provisioning', state: 'backlog', project: 'p-pharos', fields: { priority: 'high' }, updated_at: ago(40) })
  add({ id: 'n-1', key: 'PHAROS-11', kind_slug: 'ticket', title: 'Connect Hetzner Cloud for managed provisioning', state: 'in-progress', project: 'p-pharos', parent_id: epic.id, fields: { priority: 'high', assignee: me.id }, updated_at: ago(1), body: '## Acceptance\n\n- [x] Token stored in the vault\n- [ ] Cleanup runs => nothing left behind\n\n`a => b`' })
  const parentTicket = add({ id: 'n-2', key: 'PHAROS-12', kind_slug: 'ticket', title: 'Add an Oracle Cloud connector', state: 'backlog', project: 'p-pharos', parent_id: epic.id, fields: { priority: 'medium' }, updated_at: ago(3) })
  add({ id: 'n-3', key: 'PHAROS-13', kind_slug: 'task', title: 'Run the disposable Hetzner end-to-end check', state: 'qa', project: 'p-pharos', parent_id: parentTicket.id, fields: { priority: 'low', assignee: mira.id }, updated_at: ago(6) })
  add({ id: 'n-4', key: 'PHAROS-14', kind_slug: 'ticket', title: 'Visual acceptance of the version pill', state: 'new', project: 'p-pharos', fields: {}, updated_at: ago(12) })
  add({ id: 'n-5', key: 'PHAROS-15', kind_slug: 'ticket', title: 'Beacon health probes', state: 'done', project: 'p-pharos', fields: { priority: 'medium' }, updated_at: ago(48) })
  add({ id: 'n-6', key: 'PHAROS-16', kind_slug: 'ticket', title: 'Retire the old dashboard', state: 'cancelled', project: 'p-pharos', fields: { priority: 'low' }, updated_at: ago(72) })
  add({ id: 'n-a1', key: 'AEON-1', kind_slug: 'ticket', title: 'Aeon foundation', state: 'backlog', project: 'p-aeon', fields: { priority: 'high' } })
  for (let index = 0; index < (options.bigProject ?? 0); index++) {
    add({ id: `n-big-${index}`, key: `AEON-${100 + index}`, kind_slug: 'ticket', title: `Generated ticket ${index + 1}`, state: 'backlog', project: 'p-aeon', fields: { priority: 'medium' }, updated_at: ago(index + 1) })
  }
  const activity: Record<string, { id: string; at: string; type: 'comment' | 'change' | 'created'; author: { id: string | null; name: string }; body_markdown?: string; changes?: { field: string; from: string | null; to: string | null }[] }[]> = {
    // Newest first, like the API.
    'n-1': [
      { id: '9', at: ago(0.5), type: 'comment', author: { id: mira.id, name: mira.name }, body_markdown: 'Token rotation is **done**; cleanup next.' },
      { id: '8', at: ago(0.9), type: 'comment', author: { id: me.id, name: me.name }, body_markdown: 'Picked this up. `a => b` stays literal.' },
      { id: '7', at: ago(26), type: 'change', author: { id: me.id, name: me.name }, changes: [{ field: 'status', from: 'backlog', to: 'in-progress' }] },
      { id: '6', at: ago(26.02), type: 'change', author: { id: me.id, name: me.name }, changes: [{ field: 'status', from: 'new', to: 'backlog' }, { field: 'priority', from: 'medium', to: 'high' }] },
      { id: '5', at: ago(24 * 20), type: 'created', author: { id: me.id, name: me.name } },
    ],
  }
  const relations = [
    { id: 'r-1', source_node_id: 'n-4', target_node_id: 'n-1', type: 'blocks', created_at: ago(40) },
    { id: 'r-2', source_node_id: 'n-1', target_node_id: 'n-5', type: 'relates', created_at: ago(40) },
  ]
  return { projects, nodes, people: [me, mira], activity, relations, counter: { next: 100 } }
}

export type Fixtures = ReturnType<typeof fixtures>
export interface Call { path: string; method: string; query: URLSearchParams; body: unknown; headers: Record<string, string> }

function item(node: MockNode, data: Fixtures) {
  const kindIds: Record<string, string> = { epic: 'k-epic', ticket: 'k-ticket', task: 'k-task', project: 'k-project' }
  const parent = node.parent_id ? data.nodes.find(n => n.id === node.parent_id) : undefined
  const project = data.projects.find(p => p.id === node.project)!
  const assignee = typeof node.fields.assignee === 'string' ? data.people.find(p => p.id === node.fields.assignee) ?? null : null
  return {
    id: node.id, key: node.key, kind_id: kindIds[node.kind_slug], title: node.title, body: node.body, fields: node.fields, state: node.state,
    parent_id: node.parent_id, position: '0', created_at: node.created_at, updated_at: node.updated_at, deleted_at: null,
    kind_slug: node.kind_slug, kind_label: node.kind_slug[0].toUpperCase() + node.kind_slug.slice(1),
    priority: typeof node.fields.priority === 'string' ? node.fields.priority : null, assignee,
    parent: parent ? { id: parent.id, key: parent.key, title: parent.title, kind_slug: parent.kind_slug } : node.parent_id ? { id: project.id, key: project.key, title: project.title, kind_slug: 'project' } : null,
    children_count: data.nodes.filter(n => n.parent_id === node.id).length,
    project: { id: project.id, key: project.key, title: project.title },
  }
}
function projectItem(project: Fixtures['projects'][number]) {
  return {
    id: project.id, key: project.key, kind_id: 'k-project', title: project.title, body: project.description, state: project.state,
    fields: { classic: project.classic ? { key: project.classic, description: project.description } : {} },
    parent_id: null, position: '0', created_at: ago(24 * 90), updated_at: project.last, deleted_at: null,
    kind_slug: 'project', kind_label: 'Project', priority: null, assignee: null, parent: null, children_count: 0,
    project: { id: project.id, key: project.key, title: project.title },
  }
}
const listParam = (query: URLSearchParams, name: string) => (query.get(name) ?? '').split(',').map(v => v.trim()).filter(Boolean)
const normal = (state: string) => state.replace(/-/g, '_')

export async function mockWork(page: Page, data: Fixtures, options: MockOptions = {}) {
  const calls: Call[] = []
  await page.route('**/api/**', async route => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname, method = request.method(), query = url.searchParams
    let body: unknown = null
    try { body = request.postDataJSON() } catch { body = request.postData() }
    calls.push({ path, method, query, body, headers: request.headers() })
    if (path === '/api/me') return route.fulfill({ json: { principal: { id: me.id, name: me.name, roles: options.readOnly ? ['viewer'] : ['member'] }, tenant: { id: 't1', name: 'INSPR Studio' } } })
    if (path === '/api/kinds') return route.fulfill({ json: { items: ['epic', 'ticket', 'task', 'project'].map(slug => ({ id: `k-${slug}`, slug, label: slug[0].toUpperCase() + slug.slice(1), short_prefix: slug.slice(0, 3).toUpperCase(), icon: slug, allowed_child_kinds: null, field_schema: {} })) } })
    if (path === '/api/relations') return route.fulfill({ json: { items: data.relations.filter(r => r.source_node_id === query.get('node_id') || r.target_node_id === query.get('node_id')), next_cursor: null } })
    const activityPath = /^\/api\/nodes\/([^/]+)\/(activity|comments)(?:\/(\d+))?$/.exec(path)
    if (activityPath) {
      const [, nodeId, part, commentId] = activityPath
      const list = (data.activity[nodeId] ??= [{ id: `c-${nodeId}`, at: ago(24 * 20), type: 'created', author: { id: me.id, name: me.name } }])
      if (part === 'activity') return route.fulfill({ json: { items: list, next_cursor: null } })
      const text = (body as { body_markdown?: string } | null)?.body_markdown ?? ''
      if (method === 'POST') { const item = { id: String(1000 + calls.length), at: new Date(now).toISOString(), type: 'comment' as const, author: { id: me.id, name: me.name }, body_markdown: text }; list.unshift(item); return route.fulfill({ status: 201, json: item }) }
      const index = list.findIndex(item => item.id === commentId)
      if (index === -1) return route.fulfill({ status: 404, json: { error: 'comment not found' } })
      if (method === 'PATCH') { list[index] = { ...list[index], body_markdown: text }; return route.fulfill({ json: list[index] }) }
      if (method === 'DELETE') { list.splice(index, 1); return route.fulfill({ status: 204 }) }
    }
    if (path === '/api/nodes' && method === 'POST') {
      const input = body as { kind_id: string; title: string; state?: string; fields?: Record<string, unknown>; parent_id?: string; key_prefix?: string }
      const parent = data.nodes.find(n => n.id === input.parent_id)
      const project = parent?.project ?? data.projects.find(p => p.id === input.parent_id)?.id ?? 'p-pharos'
      const node = { id: `n-new-${data.counter.next}`, key: `${input.key_prefix ?? 'TKT'}-${data.counter.next++}`, kind_slug: input.kind_id.slice(2), title: input.title, body: '', state: input.state ?? 'open', fields: input.fields ?? {}, parent_id: input.parent_id ?? null, project, created_at: new Date(now).toISOString(), updated_at: new Date(now).toISOString() }
      data.nodes.push(node)
      const { kind_slug: _k, project: _p, ...rest } = node
      return route.fulfill({ status: 201, json: { ...rest, kind_id: input.kind_id, position: '0', deleted_at: null } })
    }
    const movePath = /^\/api\/nodes\/([^/]+)\/move$/.exec(path)
    if (movePath) {
      const node = data.nodes.find(n => n.id === movePath[1])!
      node.parent_id = (body as { parent_id: string }).parent_id
      node.updated_at = new Date(now + 90_000).toISOString()
      const { kind_slug: _k, project: _p, ...rest } = node
      return route.fulfill({ json: { ...rest, kind_id: 'k-ticket', position: '0', deleted_at: null } })
    }
    if (path === '/api/version') return route.fulfill({ json: { version: '260923120000.0.0', scheme: 'inspr-calendar-v2' } })
    if (path === '/api/projects') {
      if (options.failProjects) return route.fulfill({ status: 503, json: { error: 'Projects are resting' } })
      const archived = query.get('include_archived') === 'true'
      // B3 semantics: work kinds only; open = new/backlog, in_progress = in progress/QA,
      // done = done/delivered/accepted, cancelled separate.
      return route.fulfill({ json: { items: data.projects.filter(p => archived || p.state !== 'archived').map(p => {
        const inside = data.nodes.filter(n => n.project === p.id).map(n => normal(n.state))
        const count = (states: string[]) => inside.filter(state => states.includes(state)).length
        return { id: p.id, key: p.key, title: p.title, state: p.state, open: count(['new', 'backlog']), in_progress: count(['in_progress', 'qa']), done: count(['done', 'delivered', 'accepted']), cancelled: count(['cancelled']), total: inside.length, last_activity: p.last }
      }) } })
    }
    if (path === '/api/nodes' && method === 'GET') {
      const kinds = listParam(query, 'kind')
      if (kinds.length === 1 && kinds[0] === 'project') return route.fulfill({ json: { items: data.projects.map(projectItem), next_cursor: null } })
      if (options.failList) return route.fulfill({ status: 503, json: { error: 'The list is resting' } })
      if (options.delayList) await new Promise(resolve => setTimeout(resolve, options.delayList))
      const parentFilter = query.get('parent_id')
      if (parentFilter) return route.fulfill({ json: { items: data.nodes.filter(n => n.parent_id === parentFilter).map(n => item(n, data)), next_cursor: null } })
      const within = query.get('within'), states = listParam(query, 'state'), priorities = listParam(query, 'priority'), assignees = listParam(query, 'assignee'), q = (query.get('q') ?? '').toLowerCase()
      let rows = data.nodes.filter(n => (!within || n.project === within) && (!kinds.length || kinds.includes(n.kind_slug)))
        .filter(n => !states.length || states.includes(n.state))
        .filter(n => !priorities.length || priorities.includes(typeof n.fields.priority === 'string' ? n.fields.priority : 'none'))
        .filter(n => !assignees.length || assignees.includes(typeof n.fields.assignee === 'string' ? n.fields.assignee : 'none'))
        .filter(n => !q || n.key.toLowerCase().includes(q) || n.title.toLowerCase().includes(q))
        .filter(n => query.get('hide_closed') !== 'true' || !CLOSED.includes(n.state))
      const sort = (query.get('sort') ?? 'position').split(',')
      rows = [...rows].sort((a, b) => {
        for (const raw of sort) {
          const desc = raw.startsWith('-'), field = desc ? raw.slice(1) : raw
          const value = (n: MockNode): string | number => field === 'state' ? (STATE_ORDER.indexOf(normal(n.state)) + 1 || 99)
            : field === 'priority' ? PRIORITY_ORDER.indexOf(typeof n.fields.priority === 'string' ? n.fields.priority : 'none')
            : field === 'updated_at' ? Date.parse(n.updated_at) : field === 'key' ? Number(n.key.split('-')[1]) : field === 'title' ? n.title : 0
          const x = value(a), y = value(b)
          if (x !== y) return (x < y ? -1 : 1) * (desc ? -1 : 1)
        }
        return a.id < b.id ? -1 : 1
      })
      const facets: Record<string, Record<string, number>> = {}
      for (const facet of listParam(query, 'facets')) {
        facets[facet] = {}
        for (const n of rows) {
          const value = facet === 'state' ? n.state : facet === 'kind' ? n.kind_slug : facet === 'priority' ? (typeof n.fields.priority === 'string' ? n.fields.priority : 'none') : (typeof n.fields.assignee === 'string' ? n.fields.assignee : 'none')
          facets[facet][value] = (facets[facet][value] ?? 0) + 1
        }
      }
      const limit = Number(query.get('limit') ?? 50), offset = Number((query.get('cursor') ?? 'o:0').slice(2))
      const pageRows = rows.slice(offset, offset + limit)
      return route.fulfill({ json: { items: pageRows.map(n => item(n, data)), next_cursor: offset + limit < rows.length ? `o:${offset + limit}` : null, ...(Object.keys(facets).length ? { facets } : {}) } })
    }
    if (path.startsWith('/api/nodes/')) {
      const id = decodeURIComponent(path.split('/')[3]), node = data.nodes.find(n => n.id === id)
      if (!node) return route.fulfill({ status: 404, json: { error: 'Not found' } })
      if (method === 'DELETE') {
        if (data.nodes.some(n => n.parent_id === id)) return route.fulfill({ status: 409, json: { error: 'node has children' } })
        data.nodes.splice(data.nodes.indexOf(node), 1)
        return route.fulfill({ status: 204 })
      }
      if (method === 'PATCH' && options.readOnly) return route.fulfill({ status: 403, json: { error: 'forbidden' } })
      if (method === 'PATCH' && options.conflictAlways === id) {
        node.updated_at = new Date(now + 45_000 + calls.length).toISOString(); node.title = 'Renamed by Mira'
        return route.fulfill({ status: 412, json: { error: 'node has changed' } })
      }
      if (method === 'PATCH') {
        if (options.failPatch) return route.fulfill({ status: 422, json: { error: 'State is not allowed here' } })
        // Someone else saved this node after the list was read.
        if (options.conflictOn === id && !node.title.endsWith('(edited elsewhere)')) { node.updated_at = new Date(now + 30_000).toISOString(); node.title = `${node.title} (edited elsewhere)` }
        const expected = request.headers()['if-unmodified-since']
        if (expected && expected !== node.updated_at) return route.fulfill({ status: 412, json: { error: 'node has changed' } })
        Object.assign(node, body as object, { updated_at: new Date(now + 60_000 + calls.length).toISOString() })
      }
      const { kind_slug: _kind, project: _project, ...rest } = node
      return route.fulfill({ json: { ...rest, kind_id: 'k-ticket', position: '0', deleted_at: null } })
    }
    if (path === '/api/search') {
      const q = (query.get('q') ?? '').toLowerCase()
      const hits = data.nodes.filter(n => n.title.toLowerCase().includes(q)).map(n => ({ node: { ...item(n, data) }, score: 0.9 }))
      return route.fulfill({ json: { items: hits, next_cursor: null } })
    }
    return route.fulfill({ status: 404, json: { error: 'Unmocked route' } })
  })
  return calls
}

export function watchErrors(page: Page) {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error' && !/Failed to load resource/.test(message.text())) errors.push(message.text()) })
  return errors
}
