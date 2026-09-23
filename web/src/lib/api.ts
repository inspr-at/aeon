// SPDX-License-Identifier: AGPL-3.0-only
export interface Identity {
  principal: { id: string; name: string; email?: string; kind?: 'person' | 'agent'; roles?: string[] }
  tenant: { id: string; name: string }
}

export interface Version { version: string; scheme: string }

export async function api(path: string, init: RequestInit = {}) {
  return fetch(`/api${path}`, {
    credentials: 'same-origin',
    cache: 'no-store',
    signal: AbortSignal.timeout(10_000),
    ...init,
    headers: { Accept: 'application/json', ...init.headers },
  })
}

// Keep the P0.3 auth wire contract here, separate from view components.
export async function getSession(): Promise<{ identity: Identity | null; devMode: boolean }> {
  const response = await api('/me')
  if (!response.ok && response.status !== 401) throw new Error('Session unavailable')
  // A 401 may have no JSON body; it still means sign-in is required.
  const body = await response.json().catch(() => {
    if (response.status === 401) return {}
    throw new Error('Invalid session response')
  })
  const devMode = body.dev_mode === true
  if (response.status === 401) return { identity: null, devMode }
  if (typeof body.principal?.id !== 'string' || typeof body.principal?.name !== 'string'
    || typeof body.tenant?.id !== 'string' || typeof body.tenant?.name !== 'string') {
    throw new Error('Invalid session response')
  }
  return { identity: body as Identity, devMode }
}

// R1 wire types mirror api/openapi.yaml. All workspace HTTP calls stay here.
export interface Kind {
  id: string; slug: string; label: string; short_prefix: string; icon: string
  allowed_child_kinds: string[] | null; field_schema: Record<string, unknown>
}
export interface WorkNode {
  id: string; key: string; kind_id: string; title: string; body: string
  fields: Record<string, unknown>; state: string; parent_id: string | null
  position: string; created_at: string; updated_at: string; deleted_at?: string | null
}
export interface Page<T> { items: T[]; next_cursor: string | null }
export interface TreeEntry { node: WorkNode; depth: number }
export interface SearchHit { node: WorkNode; score: number }
export type SortField = 'position' | 'updated_at' | 'created_at' | 'key' | 'title'
export interface NodeFilters {
  kind_id?: string; state?: string; parent_id?: string; include_descendants?: boolean
}
export interface NodeQuery extends NodeFilters {
  sort?: SortField; direction?: 'asc' | 'desc'; cursor?: string; limit?: number
}
export interface ViewWrite {
  name: string; filters: NodeFilters; sort: { field: SortField; direction: 'asc' | 'desc' }
  columns: string[]; shared?: boolean
}
export interface SavedView extends ViewWrite {
  id: string; owner_principal_id: string; shared: boolean; created_at: string; updated_at: string
}
export interface NodeCreate {
  kind_id: string; title: string; body?: string; fields?: Record<string, unknown>
  state?: string; parent_id?: string | null; before_id?: string | null; key_prefix?: string
}
export type NodePatch = Partial<Pick<WorkNode, 'title' | 'body' | 'fields' | 'state'>>

export class APIError extends Error {
  readonly status: number
  // The parsed error body, when the server sent one (a 412 on move carries the current node).
  readonly body: Record<string, unknown>
  constructor(status: number, message: string, body: Record<string, unknown> = {}) { super(message); this.status = status; this.body = body }
}
async function json<T>(path: string, method = 'GET', body?: unknown, headers: Record<string, string> = {}): Promise<T> {
  const response = await api(path, {
    method,
    ...(body === undefined ? { headers } : { headers: { 'Content-Type': 'application/json', ...headers }, body: JSON.stringify(body) }),
  })
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    throw new APIError(response.status, typeof data?.error === 'string' ? data.error : `Request failed (${response.status})`, data && typeof data === 'object' ? data : {})
  }
  return response.status === 204 ? undefined as T : response.json()
}
function query(values: object): string {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(values)) {
    if (value !== undefined && value !== '') params.set(key, String(value))
  }
  const encoded = params.toString()
  return encoded ? `?${encoded}` : ''
}
const idPath = (id: string) => encodeURIComponent(id)
export const getKinds = () => json<{ items: Kind[] }>('/kinds')
export const getNodes = (params: NodeQuery = {}) => json<Page<WorkNode>>(`/nodes${query(params)}`)
export const getTree = (params: { root_id?: string; cursor?: string; limit?: number } = {}) => json<Page<TreeEntry>>(`/nodes/tree${query(params)}`)
export const getNode = (id: string) => json<WorkNode>(`/nodes/${idPath(id)}`)
export const createNode = (body: NodeCreate) => json<WorkNode>('/nodes', 'POST', body)
// ifUnmodifiedSince is the node's updated_at as read; a newer server copy answers 412.
export const updateNode = (id: string, body: NodePatch, options: { ifUnmodifiedSince?: string } = {}) =>
  json<WorkNode>(`/nodes/${idPath(id)}`, 'PATCH', body, options.ifUnmodifiedSince ? { 'If-Unmodified-Since': options.ifUnmodifiedSince } : {})
// ifUnmodifiedSince: the node's updated_at as read; the move answers 412 with the current node when it changed.
export const moveNode = (id: string, parent_id: string | null, before_id?: string | null, options: { ifUnmodifiedSince?: string } = {}) =>
  json<WorkNode>(`/nodes/${idPath(id)}/move`, 'POST', { parent_id, before_id }, options.ifUnmodifiedSince ? { 'If-Unmodified-Since': options.ifUnmodifiedSince } : {})
export const deleteNode = (id: string) => json<void>(`/nodes/${idPath(id)}`, 'DELETE')
export const searchNodes = (q: string, params: { kind_id?: string; state?: string; cursor?: string; limit?: number } = {}) => json<Page<SearchHit>>(`/search${query({ q, ...params })}`)
// B1 list and project-summary wire types (api/openapi.yaml NodeListItem, listProjects).
export interface ListPerson { id: string; name: string }
export interface ListParent { id: string; key: string; title: string; kind_slug: string }
export interface ListProject { id: string; key: string; title: string }
export interface ListItem extends WorkNode {
  kind_slug: string; kind_label: string; priority: string | null; assignee: ListPerson | null
  parent: ListParent | null; children_count: number; project: ListProject | null
}
export type Facets = Record<string, Record<string, number>>
export interface ListPage extends Page<ListItem> { facets?: Facets }
export interface ListQuery {
  within?: string; kind?: string[]; state?: string[]; priority?: string[]; assignee?: string[]
  q?: string; hide_closed?: boolean; facets?: string[]; sort?: string; cursor?: string; limit?: number; parent_id?: string
}
// Work (ticket, task, epic) counts: open = new and backlog, in_progress = in progress and QA,
// done = done, delivered and accepted; cancelled is separate; total counts every work state.
export interface ProjectSummary {
  id: string; key: string; title: string; state: string
  open: number; in_progress: number; done: number; cancelled?: number; total: number; last_activity: string
}
function listQuery(params: ListQuery): string {
  const values: Record<string, string | number | boolean | undefined> = {}
  for (const [key, value] of Object.entries(params)) {
    if (Array.isArray(value)) { if (value.length) values[key] = value.join(',') }
    else if (value !== undefined && value !== '' && value !== false) values[key] = value
  }
  return query(values)
}
export const listNodes = (params: ListQuery) => json<ListPage>(`/nodes${listQuery(params)}`)
export const getProjects = (includeArchived = false) => json<{ items: ProjectSummary[] }>(`/projects${includeArchived ? '?include_archived=true' : ''}`)
// B2 ticket activity and comments.
export type ChangeField = 'status' | 'priority' | 'assignee' | 'title' | 'parent'
export interface ActivityChange { field: ChangeField; from: string | null; to: string | null }
export interface ActivityItem {
  id: string; at: string; type: 'comment' | 'change' | 'created'
  author: { id: string | null; name: string }
  body_markdown?: string; changes?: ActivityChange[]
}
export const getActivity = (nodeId: string, cursor?: string | null) =>
  json<{ items: ActivityItem[]; next_cursor: string | null }>(`/nodes/${idPath(nodeId)}/activity${query({ limit: 50, cursor: cursor ?? undefined })}`)
export const createComment = (nodeId: string, body_markdown: string) => json<ActivityItem>(`/nodes/${idPath(nodeId)}/comments`, 'POST', { body_markdown })
export const updateComment = (nodeId: string, commentId: string, body_markdown: string) =>
  json<ActivityItem>(`/nodes/${idPath(nodeId)}/comments/${idPath(commentId)}`, 'PATCH', { body_markdown })
export const deleteComment = (nodeId: string, commentId: string) => json<void>(`/nodes/${idPath(nodeId)}/comments/${idPath(commentId)}`, 'DELETE')

export type RelationType = 'blocks' | 'relates' | 'implements' | 'cites' | 'duplicates' | 'customer_of' | 'contact_for'
export interface Relation { id: string; source_node_id: string; target_node_id: string; type: RelationType; created_at: string }
export const getRelations = (nodeId: string) => json<{ items: Relation[]; next_cursor: string | null }>(`/relations${query({ node_id: nodeId, limit: 100 })}`)

export const getViews = () => json<{ items: SavedView[] }>('/views')
export const createView = (body: ViewWrite) => json<SavedView>('/views', 'POST', body)
export const updateView = (id: string, body: Partial<ViewWrite>) => json<SavedView>(`/views/${idPath(id)}`, 'PATCH', body)
export const deleteView = (id: string) => json<void>(`/views/${idPath(id)}`, 'DELETE')

// EventSource owns Last-Event-ID and retries; close it when the workspace unmounts.
// Named events do not reach onmessage. Keep this list aligned with R1 writers.
export function subscribeWorkspace(changed: () => void, connection: (live: boolean) => void): () => void {
  const stream = new EventSource('/api/events/stream')
  stream.onopen = () => { connection(true); changed() }
  stream.onerror = () => connection(false)
  stream.onmessage = changed
  for (const resource of ['node', 'kind', 'relation', 'view']) {
    for (const action of ['created', 'updated', 'deleted', 'moved', 'restored']) {
      stream.addEventListener(`${resource}.${action}`, changed)
    }
  }
  stream.addEventListener('event.undone', changed)
  return () => stream.close()
}
