// SPDX-License-Identifier: AGPL-3.0-only
export interface Identity {
  principal: { id: string; name: string; email?: string; kind?: 'person' | 'agent'; roles?: string[] }
  tenant: { id: string; name: string }
  // The signed-in person's external identity; absent for agent keys.
  identity?: { email?: string; display_name?: string } | null
}

export function accountName(identity: Identity) {
  return identity.identity?.display_name?.trim() || identity.principal.name
}

export function accountEmail(identity: Identity) {
  return identity.identity?.email?.trim() || identity.principal.email || ''
}

export interface Version { version: string; scheme: string; brand?: import('./brand').Brand }

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
export interface SearchHit { node: WorkNode; score: number }
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
async function json<T>(path: string, method = 'GET', body?: unknown, headers: Record<string, string> = {}, signal?: AbortSignal): Promise<T> {
  const response = await api(path, {
    method,
    // A caller's signal (stale palette requests) combines with the usual timeout.
    ...(signal ? { signal: AbortSignal.any([signal, AbortSignal.timeout(10_000)]) } : {}),
    ...(body === undefined ? { headers } : { headers: { 'Content-Type': 'application/json', ...headers }, body: JSON.stringify(body) }),
  })
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    // A session that ended mid-work: the page keeps what was typed, and the shell
    // offers to sign in again beside it (AEON-140), instead of the bare word "unauthorized".
    if (response.status === 401) { sessionEnded.handler?.(); throw new APIError(401, 'your session has ended', data && typeof data === 'object' ? data : {}) }
    throw new APIError(response.status, typeof data?.error === 'string' ? data.error : `Request failed (${response.status})`, data && typeof data === 'object' ? data : {})
  }
  return response.status === 204 ? undefined as T : response.json()
}
// The shell registers what happens when a request finds the session ended.
export const sessionEnded: { handler: (() => void) | null } = { handler: null }
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
export const getNode = (id: string) => json<WorkNode>(`/nodes/${idPath(id)}`)
export const createNode = (body: NodeCreate) => json<WorkNode>('/nodes', 'POST', body)
// ifUnmodifiedSince is the node's updated_at as read; a newer server copy answers 412.
export const updateNode = (id: string, body: NodePatch, options: { ifUnmodifiedSince?: string } = {}) =>
  json<WorkNode>(`/nodes/${idPath(id)}`, 'PATCH', body, options.ifUnmodifiedSince ? { 'If-Unmodified-Since': options.ifUnmodifiedSince } : {})
// ifUnmodifiedSince: the node's updated_at as read; the move answers 412 with the current node when it changed.
export const moveNode = (id: string, parent_id: string | null, before_id?: string | null, options: { ifUnmodifiedSince?: string } = {}) =>
  json<WorkNode>(`/nodes/${idPath(id)}/move`, 'POST', { parent_id, before_id }, options.ifUnmodifiedSince ? { 'If-Unmodified-Since': options.ifUnmodifiedSince } : {})
export const deleteNode = (id: string) => json<void>(`/nodes/${idPath(id)}`, 'DELETE')
export const searchNodes = (q: string, params: { kind_id?: string; state?: string; cursor?: string; limit?: number } = {}, options: { signal?: AbortSignal } = {}) =>
  json<Page<SearchHit>>(`/search${query({ q, ...params })}`, 'GET', undefined, {}, options.signal)
// B1 list and project-summary wire types (api/openapi.yaml NodeListItem, listProjects).
export interface ListPerson { id: string; name: string }
export interface ListParent { id: string; key: string; title: string; kind_slug: string }
export interface ListProject { id: string; key: string; title: string }
export interface ListItem extends WorkNode {
  kind_slug: string; kind_label: string; priority: string | null; assignee: ListPerson | null
  parent: ListParent | null; children_count: number; project: ListProject | null
  // The nearest epic above the item (a task's is its ticket's epic); absent on older servers.
  epic?: ListProject | null
}
export type Facets = Record<string, Record<string, number>>
export interface ListPage extends Page<ListItem> { facets?: Facets }
export interface ListQuery {
  within?: string; kind?: string[]; state?: string[]; priority?: string[]; assignee?: string[]
  tag?: string[]; epic?: string[]; cost_unit?: string[]; release?: string[]
  date_field?: string; date_from?: string; date_to?: string
  q?: string; hide_closed?: boolean; facets?: string[]; sort?: string; cursor?: string; limit?: number; parent_id?: string
}
// Work (ticket, task, epic) counts: open = new and backlog, in_progress = in progress and QA,
// done = done, delivered and accepted; cancelled is separate; total counts every work state.
export interface ProjectSummary {
  id: string; key: string; title: string; state: string
  open: number; in_progress: number; done: number; cancelled?: number; total: number; last_activity: string
  // The people (and agents) most recently active in the project, newest first; absent on older servers.
  people?: ProjectPerson[]
}
export interface ProjectPerson { id: string; name: string; kind: 'person' | 'agent' }
function listQuery(params: ListQuery): string {
  const values: Record<string, string | number | boolean | undefined> = {}
  for (const [key, value] of Object.entries(params)) {
    if (Array.isArray(value)) { if (value.length) values[key] = value.join(',') }
    else if (value !== undefined && value !== '' && value !== false) values[key] = value
  }
  return query(values)
}
export const listNodes = (params: ListQuery, options: { signal?: AbortSignal } = {}) => json<ListPage>(`/nodes${listQuery(params)}`, 'GET', undefined, {}, options.signal)
// U22 saved views: a project's list state with a name, own or shared (api/openapi.yaml SavedView).
export interface SavedView {
  id: string; owner_principal_id: string; project_id: string | null; name: string
  filters: Record<string, unknown>; sort_keys: string[]; group_by: string; columns: string[]; shared: boolean
  created_at: string; updated_at: string; deleted_at: string | null
}
export interface ViewWrite { name: string; project_id?: string | null; filters: Record<string, string>; sort_keys: string[]; group_by: string; columns: string[]; shared: boolean }
export const listViews = (projectId: string) => json<{ items: SavedView[] }>(`/views${query({ project_id: projectId })}`)
export const createView = (body: ViewWrite) => json<SavedView>('/views', 'POST', body)
export const updateView = (id: string, body: Partial<Omit<ViewWrite, 'project_id'>>) => json<SavedView>(`/views/${idPath(id)}`, 'PATCH', body)
export const deleteView = (id: string) => json<void>(`/views/${idPath(id)}`, 'DELETE')
export const restoreView = (id: string) => json<SavedView>(`/views/${idPath(id)}/restore`, 'POST')
// U22 bulk change: one change for many nodes, undone as one through its event.
export interface BulkChange {
  ids: string[]; state?: string; priority?: string | null; assignee?: string | null
  tags_add?: (string | { name: string; color?: string })[]; tags_remove?: string[]; parent_id?: string
}
export interface BulkResult { event_id: number | null; items: WorkNode[]; unchanged: string[]; skipped: { id: string; key?: string; reason: string }[] }
export const bulkChange = (body: BulkChange) => json<BulkResult>('/nodes/bulk', 'POST', body)
export const undoEvent = (eventId: number) => json<unknown>(`/events/${eventId}/undo`, 'POST')
export const getProjects = (includeArchived = false) => json<{ items: ProjectSummary[] }>(`/projects${includeArchived ? '?include_archived=true' : ''}`)
// Shared project groups (AEON-136): everyone reads them; admins write, and every
// write answers the event that POST /events/{id}/undo reverses.
export interface SharedProjectGroup { id: string; name: string; position: number; project_ids: string[]; created_by: string; created_at: string; updated_at: string }
export interface ProjectGroupWrite { group?: SharedProjectGroup; assignments?: { project_id: string; group_id: string | null }[]; event_id: number | null }
export const getProjectGroups = () => json<{ items: SharedProjectGroup[] }>('/project-groups')
export const createProjectGroup = (body: { name: string; project_ids?: string[]; position?: number }) => json<ProjectGroupWrite>('/project-groups', 'POST', body)
export const updateProjectGroup = (id: string, body: { name?: string; position?: number }) => json<ProjectGroupWrite>(`/project-groups/${idPath(id)}`, 'PATCH', body)
export const deleteProjectGroup = (id: string) => json<ProjectGroupWrite>(`/project-groups/${idPath(id)}`, 'DELETE')
export const assignProjectGroup = (groupId: string | null, projectIds: string[]) => json<ProjectGroupWrite>('/project-groups/assign', 'POST', { group_id: groupId, project_ids: projectIds })
export const undoGroupEvent = (eventId: number) => json<unknown>(`/events/${eventId}/undo`, 'POST')
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
export interface NodePreview { id: string; key: string; title: string; state: string }
export const lookupNodes = (ids: string[]) => json<{ items: NodePreview[] }>(`/nodes/lookup${query({ ids })}`)

