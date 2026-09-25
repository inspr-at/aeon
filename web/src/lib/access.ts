// SPDX-License-Identifier: AGPL-3.0-only
// Access: people, invites, roles, project access, agents and the access audit
// (ADR-003, the authz contract). Wire types mirror the contract exactly; the
// helpers below are free of Vue so they can be unit-tested.
import { api } from './api.ts'
import { learnPictures } from './avatar.ts'
import type { RoleRef } from './authz.ts'

export type { RoleRef }
export type Risk = 'low' | 'medium' | 'high'
export type Scope = 'workspace' | 'project'
export interface Permission { key: string; group: string; description: string; risk: Risk; grantable_at: Scope[] }
export interface Role { id: string; key: string; name: string; description: string; builtin: boolean; permissions: string[]; based_on: string | null; member_count: number }
export interface ProjectRole { project_id: string; project_key: string; project_title: string; role: RoleRef }
export interface Alias { principal_id: string; name: string; source: 'classic' }
export type Status = 'active' | 'deactivated'
export interface Person {
  // has_avatar (U27) is additive; the contract's avatar_url says the same when it is absent.
  principal_id: string; name: string; avatar_url: string | null; has_avatar?: boolean; email: string | null; status: Status; identity: 'inspr_id' | null
  workspace_role: RoleRef | null; project_roles: ProjectRole[]; aliases: Alias[]; classic_role: string | null; last_active_at: string | null
}
export interface Agent { principal_id: string; name: string; workspace_role: RoleRef | null; key_count: number; last_seen_at: string | null; service: boolean }
export type InviteStatus = 'pending' | 'expired' | 'revoked' | 'accepted'
export interface Invite {
  id: string; email: string; workspace_role: RoleRef | null; project_roles: ProjectRole[]; status: InviteStatus
  created_by: string | { principal_id: string; name: string }; created_at: string; expires_at: string
}
export interface Imported { principal_id: string; name: string; classic_role: string | null }
export interface Members { people: Person[]; agents: Agent[]; invites: Invite[]; imported: Imported[] }
export interface ProjectMember { principal_id: string; name: string; avatar_url: string | null; has_avatar?: boolean; kind: 'person' | 'agent'; via: 'workspace' | 'project'; role: RoleRef }
export interface AuditEvent { id: number; actor_principal_id: string; node_id?: string | null; type: string; before: unknown; after: unknown; at: string }

// ---------- Errors: {error, code, field?, reason} ----------
export class AccessError extends Error {
  readonly status: number
  readonly code: string
  readonly field: string | null
  constructor(status: number, body: { error?: string; code?: string; field?: string; reason?: string; message?: string }) {
    super(body.reason || body.error || body.message || `Request failed (${status})`)
    this.status = status
    this.code = body.code ?? ''
    this.field = body.field ?? null
  }
}
async function call<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const response = await api(path, { method, ...(body === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }) })
  if (!response.ok) throw new AccessError(response.status, await response.json().catch(() => ({})))
  return response.status === 204 ? undefined as T : response.json() as Promise<T>
}
const id = (value: string) => encodeURIComponent(value)

export const getRegistry = () => call<Permission[]>('/authz/permissions')
export const getRoles = () => call<Role[]>('/roles')
export const createRole = (body: { name: string; description: string; permissions: string[]; based_on?: string | null }) => call<Role>('/roles', 'POST', body)
export const updateRole = (roleId: string, body: { name?: string; description?: string; permissions?: string[] }) => call<Role>(`/roles/${id(roleId)}`, 'PATCH', body)
export const deleteRole = (roleId: string, reassignTo?: string) => call<void>(`/roles/${id(roleId)}${reassignTo ? `?reassign_to=${id(reassignTo)}` : ''}`, 'DELETE')
// Avatars ask for a picture only for people a payload says have one (lib/avatar).
const pictureHints = (people: { principal_id: string; avatar_url: string | null; has_avatar?: boolean; kind?: string }[]) =>
  learnPictures(people.filter(p => p.kind !== 'agent').map(p => ({ id: p.principal_id, has_avatar: p.has_avatar ?? !!p.avatar_url })))
export const getMembers = () => call<Members>('/members').then(members => { pictureHints(members.people); return members })
export const setWorkspaceRole = (principalId: string, roleId: string | null) => call<Person | Agent>(`/members/${id(principalId)}/workspace-role`, 'PUT', { role_id: roleId })
export const getProjectMembers = (projectId: string) => call<ProjectMember[]>(`/projects/${id(projectId)}/members`).then(members => { pictureHints(members); return members })
export const setProjectRole = (projectId: string, principalId: string, roleId: string) => call<unknown>(`/projects/${id(projectId)}/members/${id(principalId)}`, 'PUT', { role_id: roleId })
export const removeProjectMember = (projectId: string, principalId: string) => call<void>(`/projects/${id(projectId)}/members/${id(principalId)}`, 'DELETE')
export const createInvite = (body: { email: string; workspace_role_id?: string; project_roles?: { project_id: string; role_id: string }[]; expires_in_days?: number }) =>
  call<{ invite: Invite; join_url: string }>('/members/invites', 'POST', body)
export const revokeInvite = (inviteId: string) => call<void>(`/members/invites/${id(inviteId)}`, 'DELETE')
export const deactivate = (principalId: string) => call<Person>(`/members/${id(principalId)}/deactivate`, 'POST')
export const reactivate = (principalId: string) => call<Person>(`/members/${id(principalId)}/reactivate`, 'POST')
export const linkAlias = (principalId: string, fromPrincipalId: string) => call<Person>(`/members/${id(principalId)}/aliases`, 'POST', { from_principal_id: fromPrincipalId })
export const unlinkAlias = (principalId: string, fromPrincipalId: string) => call<void>(`/members/${id(principalId)}/aliases/${id(fromPrincipalId)}`, 'DELETE')
export async function getAudit(after?: number): Promise<{ items: AuditEvent[]; next_after: number | null }> {
  const body = await call<AuditEvent[] | { items: AuditEvent[]; next_after?: number | null }>(`/audit?category=access${after ? `&after=${after}` : ''}`)
  return Array.isArray(body) ? { items: body, next_after: null } : { items: body.items, next_after: body.next_after ?? null }
}
// Agent keys (existing endpoints): a new key's secret is shown only once.
export interface AgentKeyCreated { id: string; token: string; prefix: string; name: string; expires_at: string | null }
export const createAgentKey = (name: string, expiresAt: string | null, scopes: string[]) => call<AgentKeyCreated>('/agent-keys', 'POST', { name, scopes, ...(expiresAt ? { expires_at: expiresAt } : {}) })
export const revokeAgentKey = (keyId: string) => call<void>(`/agent-keys/${id(keyId)}`, 'DELETE')

// ---------- Agent key scopes ----------
// What a key may call. Since SEC4 the server's key ceiling denies by default and
// an empty list grants nothing (internal/auth/doc.go); a key also never does more
// than its agent's role allows. The server has no list of scopes to read yet, so
// this mirrors its route table (stage.<op> scopes are left to the CLI).
export interface KeyScope { key: string; label: string }
export const KEY_SCOPE_GROUPS: { label: string; scopes: KeyScope[] }[] = [
  { label: 'Work', scopes: [{ key: 'nodes.read', label: 'Read projects and tickets' }, { key: 'nodes.write', label: 'Create and change tickets' }, { key: 'nodes.configure', label: 'Set up ticket types and tags' }] },
  { label: 'Links and history', scopes: [{ key: 'relations.read', label: 'Read links between tickets' }, { key: 'relations.write', label: 'Link tickets' }, { key: 'events.read', label: 'Read history' }, { key: 'events.undo', label: 'Undo changes' }] },
  { label: 'Search and views', scopes: [{ key: 'search.read', label: 'Search' }, { key: 'views.read', label: 'Read saved views' }, { key: 'views.write', label: 'Save views' }] },
  { label: 'Knowledge', scopes: [{ key: 'knowledge.read', label: 'Read knowledge' }, { key: 'knowledge.write', label: 'Write knowledge' }, { key: 'journey.read', label: 'Read project journeys' }] },
  { label: 'Messages and approvals', scopes: [{ key: 'inbox.read', label: 'Read its messages' }, { key: 'inbox.send', label: 'Send messages' }, { key: 'approvals.read', label: 'Read approvals' }, { key: 'approvals.request', label: 'Ask for approval' }] },
  { label: 'Work orders and runs', scopes: [{ key: 'work_orders.read', label: 'Read work orders' }, { key: 'work_orders.write', label: 'Write work orders' }, { key: 'run.create', label: 'Start runs' }, { key: 'run.read', label: 'Read runs' }, { key: 'run.claim', label: 'Take on runs' }, { key: 'run.telemetry', label: 'Report run progress' }] },
  { label: 'Harness sessions', scopes: [{ key: 'harness.read', label: 'Read sessions' }, { key: 'harness.write', label: 'Register and update sessions' }, { key: 'harness.worker', label: 'Work as a session' }, { key: 'harness.control', label: 'Control sessions' }] },
  { label: 'Intake and hours', scopes: [{ key: 'intake.read', label: 'Read intake' }, { key: 'intake.write', label: 'Add to intake' }, { key: 'hours.read', label: 'Read hours' }, { key: 'hours.write', label: 'Book hours' }] },
  { label: 'Itself', scopes: [{ key: 'account.manage', label: 'Read its own identity and accounts' }, { key: 'models.read', label: 'Read models' }, { key: 'plugins.read', label: 'Read plugins' }] },
]
const ALL_SCOPES = KEY_SCOPE_GROUPS.flatMap(g => g.scopes)
export const KEY_SCOPE_COUNT = ALL_SCOPES.length
export const MAX_KEY_SCOPES = 32
export const KEY_PRESETS: { id: string; label: string; scopes: string[] }[] = [
  { id: 'read', label: 'Read only', scopes: [...ALL_SCOPES.filter(s => s.key.endsWith('.read')).map(s => s.key), 'account.manage'] },
  // What the coordinating agent uses end to end (internal/cli compat test).
  { id: 'coordinator', label: 'Coordinator', scopes: ['account.manage', 'inbox.read', 'inbox.send', 'models.read', 'nodes.read', 'nodes.write', 'nodes.configure', 'relations.read', 'relations.write', 'events.read', 'events.undo', 'search.read', 'views.read', 'views.write'] },
]
export const scopeLabel = (key: string) => ALL_SCOPES.find(s => s.key === key)?.label ?? key

// ---------- Permissions in words ----------
const RESOURCE: Record<string, string> = {
  nodes: 'work', comments: 'comments', knowledge: 'knowledge', kinds: 'ticket types', settings: 'workspace settings', members: 'members',
  roles: 'roles', audit: 'the access log', keys: 'agent keys', approvals: 'agent approvals', agents: 'agents', quotes: 'quotes',
  hours: 'hours', customers: 'customers', portal: 'the customer portal', workspace: 'the workspace', releases: 'releases', journey: 'the journey',
}
const ACTION: Record<string, string> = { read: 'See', write: 'Edit', manage: 'Manage', delete: 'Delete', issue: 'Issue', approve: 'Approve', decide: 'Decide on', log: 'Log', run: 'Run', create: 'Create' }
// "members.manage" -> "Manage members"; "nodes.read" -> "See work".
export function permissionLabel(key: string): string {
  const [resource, ...rest] = key.split('.')
  const action = rest.join('.')
  const noun = RESOURCE[resource!] ?? resource!.replace(/_/g, ' ')
  const verb = ACTION[action] ?? action.replace(/[._]/g, ' ')
  return `${verb.charAt(0).toUpperCase()}${verb.slice(1)} ${noun}`
}
export const RISK_LABEL: Record<Risk, string> = { low: 'Low risk', medium: 'Medium risk', high: 'High risk' }
const RISK_ORDER: Record<Risk, number> = { high: 0, medium: 1, low: 2 }

// Groups in the registry's own order, each with its permissions.
export function groupPermissions(registry: Permission[]): { group: string; items: Permission[] }[] {
  const out: { group: string; items: Permission[] }[] = []
  for (const permission of registry) {
    let bucket = out.find(g => g.group === permission.group)
    if (!bucket) { bucket = { group: permission.group, items: [] }; out.push(bucket) }
    bucket.items.push(permission)
  }
  return out
}
export function diff(base: string[], next: string[]): { added: string[]; removed: string[] } {
  const a = new Set(base), b = new Set(next)
  return { added: next.filter(k => !a.has(k)), removed: base.filter(k => !b.has(k)) }
}
// A role's effect in one line: its riskiest permissions first, in words.
export function effectLine(permissions: string[], registry: Permission[], max = 4): string {
  if (!permissions.length) return 'No access of its own.'
  const byKey = new Map(registry.map(p => [p.key, p]))
  const sorted = [...permissions].sort((x, y) => RISK_ORDER[byKey.get(x)?.risk ?? 'low'] - RISK_ORDER[byKey.get(y)?.risk ?? 'low'] || registry.findIndex(p => p.key === x) - registry.findIndex(p => p.key === y))
  const words = sorted.slice(0, max).map(key => { const label = permissionLabel(key); return label.charAt(0).toLowerCase() + label.slice(1) })
  const rest = sorted.length - words.length
  return `Can ${words.join(', ')}${rest > 0 ? `, and ${rest} more` : ''}.`
}
// Permissions in `wanted` that `mine` lacks: a role holding them cannot be granted by me.
export const beyond = (wanted: string[], mine: Set<string>) => wanted.filter(key => !mine.has(key))

// ---------- Defaults ----------
// What a new invite starts with: Member in the workspace; Guest on a project.
export const defaultWorkspaceRole = (roles: Role[]) => roles.find(r => r.builtin && r.key === 'member')?.id ?? ''
export const defaultProjectRole = (roles: Role[]) => roles.find(r => r.builtin && r.key === 'guest')?.id ?? roles[0]?.id ?? ''
// Roles that mean something on a project: at least one permission grantable there.
export const projectRolesOf = (roles: Role[], registry: Permission[]) => roles.filter(role => role.permissions.some(key => registry.find(p => p.key === key)?.grantable_at.includes('project')))

// ---------- People ----------
export const OWNER = 'owner'
// The last active owner can never be demoted, deactivated or removed.
export function isLastOwner(person: Pick<Person, 'principal_id' | 'status' | 'workspace_role'>, people: Pick<Person, 'principal_id' | 'status' | 'workspace_role'>[]): boolean {
  if (person.workspace_role?.key !== OWNER || person.status !== 'active') return false
  return people.filter(p => p.status === 'active' && p.workspace_role?.key === OWNER).length === 1
}
export const LAST_OWNER_REASON = 'The last active owner keeps Owner, so the workspace always has someone who can manage it. Make another person an owner first.'
export function projectSummary(roles: ProjectRole[]): string {
  if (!roles.length) return ''
  if (roles.length <= 2) return roles.map(r => r.project_title).join(', ')
  return `${roles.length} projects`
}
export function identityLine(person: Pick<Person, 'email' | 'identity'>): string {
  return person.email ?? (person.identity === 'inspr_id' ? 'INSPR ID' : '')
}
export function matchesPerson(person: Person, needle: string): boolean {
  const text = `${person.name} ${person.email ?? ''} ${person.aliases.map(a => a.name).join(' ')} ${person.workspace_role?.name ?? ''}`.toLowerCase()
  return !needle || text.includes(needle.trim().toLowerCase())
}

// ---------- Invites ----------
export const INVITE_LABEL: Record<InviteStatus, string> = { pending: 'Pending', expired: 'Expired', revoked: 'Revoked', accepted: 'Accepted' }
export const EXPIRY_DAYS = [7, 14, 30, 60, 90] as const
export const validEmail = (value: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim())
export function inviteRoles(invite: Pick<Invite, 'workspace_role' | 'project_roles'>): string {
  const parts = [invite.workspace_role ? `${invite.workspace_role.name} in the workspace` : '', ...invite.project_roles.map(r => `${r.role.name} on ${r.project_title}`)].filter(Boolean)
  return parts.join(' · ') || 'No access yet'
}
export const creatorName = (invite: Invite, names: Map<string, string>) =>
  typeof invite.created_by === 'string' ? names.get(invite.created_by) ?? 'Someone' : invite.created_by.name

// ---------- Audit ----------
export type AuditCategory = 'roles' | 'bindings' | 'invites' | 'lifecycle' | 'keys'
export const AUDIT_CATEGORIES: { id: AuditCategory; label: string }[] = [
  { id: 'bindings', label: 'Access' }, { id: 'roles', label: 'Roles' }, { id: 'invites', label: 'Invites' }, { id: 'lifecycle', label: 'People' }, { id: 'keys', label: 'Keys' },
]
export function categoryOf(type: string): AuditCategory | null {
  if (type.startsWith('role.')) return 'roles'
  if (type.startsWith('binding.')) return 'bindings'
  if (type.startsWith('invite.')) return 'invites'
  if (type.startsWith('principal.')) return 'lifecycle'
  if (type.startsWith('agent_key.')) return 'keys'
  return null
}
export interface Names { principal: (id: string | null | undefined) => string; project: (id: string | null | undefined) => string }
type Snapshot = Record<string, unknown>
const obj = (value: unknown): Snapshot => value && typeof value === 'object' && !Array.isArray(value) ? value as Snapshot : {}
const str = (value: unknown) => typeof value === 'string' ? value : ''
const roleName = (value: unknown) => str(obj(value).name) || str(obj(obj(value).role).name)
// One event as a sentence: who did what to whom, in plain words. The subject
// (the person or thing changed) is returned apart, so a filter can match it.
export function auditSentence(event: AuditEvent, names: Names): { actor: string; text: string; subject: string } {
  const before = obj(event.before), after = obj(event.after), either = Object.keys(after).length ? after : before
  const actor = names.principal(event.actor_principal_id)
  const who = names.principal(str(either.principal_id))
  const scope = str(either.scope) === 'project' || str(either.project_id) ? ` on ${names.project(str(either.project_id))}` : ' in the workspace'
  switch (event.type) {
    case 'role.created': return { actor, subject: str(after.name), text: `created the role ${str(after.name)}` }
    case 'role.updated': {
      const d = diff((before.permissions as string[] | undefined) ?? [], (after.permissions as string[] | undefined) ?? [])
      const renamed = str(before.name) && str(before.name) !== str(after.name) ? `renamed the role ${str(before.name)} to ${str(after.name)}` : `changed the role ${str(after.name)}`
      const changes = [d.added.length ? `${d.added.length} added` : '', d.removed.length ? `${d.removed.length} removed` : ''].filter(Boolean).join(', ')
      return { actor, subject: str(after.name), text: changes ? `${renamed} (${changes})` : renamed }
    }
    case 'role.deleted': return { actor, subject: str(before.name), text: `deleted the role ${str(before.name)}${after.reassigned_to ? `; its people now have ${roleName(after.reassigned_to) || 'another role'}` : ''}` }
    case 'binding.set': {
      const from = roleName(before), to = roleName(after)
      return { actor, subject: who, text: from ? `changed ${who}’s role${scope} from ${from} to ${to}` : `gave ${who} the role ${to}${scope}` }
    }
    case 'binding.removed': return { actor, subject: who, text: `removed ${who}’s ${roleName(before) || 'access'}${scope}` }
    case 'invite.created': return { actor, subject: str(after.email), text: `invited ${str(after.email)}` }
    case 'invite.revoked': return { actor, subject: str(either.email), text: `revoked the invite for ${str(either.email)}` }
    case 'invite.accepted': return { actor, subject: str(either.email), text: `accepted the invite for ${str(either.email)}` }
    case 'principal.deactivated': return { actor, subject: who, text: `deactivated ${who}; their sessions and keys were revoked` }
    case 'principal.reactivated': return { actor, subject: who, text: `reactivated ${who}` }
    case 'principal.alias_linked': return { actor, subject: who, text: `linked ${names.principal(str(either.from_principal_id))} to ${who}` }
    case 'principal.alias_unlinked': return { actor, subject: who, text: `unlinked ${names.principal(str(either.from_principal_id))} from ${who}` }
    case 'agent_key.created': return { actor, subject: names.principal(str(either.principal_id)), text: `created the key ${str(either.name) || 'for an agent'}${either.prefix ? ` (aeon_${str(either.prefix)}_…)` : ''}` }
    case 'agent_key.revoked': return { actor, subject: names.principal(str(either.principal_id)), text: `revoked the key ${str(either.name) || ''}${either.prefix ? ` (aeon_${str(either.prefix)}_…)` : ''}`.trim() }
    default: return { actor, subject: '', text: event.type.replace(/[._]/g, ' ') }
  }
}
