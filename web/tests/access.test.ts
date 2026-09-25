// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  AccessError, auditSentence, beyond, categoryOf, defaultProjectRole, defaultWorkspaceRole, diff, effectLine, groupPermissions, inviteRoles,
  isLastOwner, permissionLabel, projectRolesOf, projectSummary, validEmail, type Permission, type Role,
} from '../src/lib/access.ts'
import { can, loadPermissions, myWorkspaceRole, permissionsAvailable, refreshPermissions, resetPermissions } from '../src/lib/authz.ts'

const P = (key: string, group: string, risk: Permission['risk'], project = true): Permission => ({ key, group, description: key, risk, grantable_at: project ? ['workspace', 'project'] : ['workspace'] })
const registry = [P('nodes.read', 'Work', 'low'), P('nodes.delete', 'Work', 'high'), P('members.manage', 'People', 'high'), P('audit.read', 'People', 'medium', false), P('portal.quotes', 'Portal', 'low', false)]
const role = (id: string, key: string, permissions: string[], builtin = true): Role => ({ id, key, name: key[0]!.toUpperCase() + key.slice(1), description: '', builtin, permissions, based_on: null, member_count: 0 })

test('permissions read as words and group in registry order', () => {
  assert.equal(permissionLabel('members.manage'), 'Manage members')
  assert.equal(permissionLabel('nodes.read'), 'See work')
  assert.equal(permissionLabel('widgets.frob'), 'Frob widgets')
  assert.deepEqual(groupPermissions(registry).map(g => [g.group, g.items.length]), [['Work', 2], ['People', 2], ['Portal', 1]])
})

test('a role’s effect puts its riskiest permissions first; diffs and escalation', () => {
  assert.equal(effectLine(['nodes.read', 'members.manage', 'nodes.delete'], registry, 2), 'Can delete work, manage members, and 1 more.')
  assert.equal(effectLine([], registry), 'No access of its own.')
  assert.deepEqual(diff(['a', 'b'], ['b', 'c']), { added: ['c'], removed: ['a'] })
  assert.deepEqual(beyond(['nodes.read', 'members.manage'], new Set(['nodes.read'])), ['members.manage'])
})

test('the last active owner is recognised; a second owner or a deactivated one lifts it', () => {
  const owner = { id: 'r1', key: 'owner', name: 'Owner' }, admin = { id: 'r2', key: 'admin', name: 'Admin' }
  const a = { principal_id: 'a', status: 'active' as const, workspace_role: owner }
  const b = { principal_id: 'b', status: 'active' as const, workspace_role: admin }
  assert.equal(isLastOwner(a, [a, b]), true)
  assert.equal(isLastOwner(b, [a, b]), false)
  assert.equal(isLastOwner(a, [a, { ...b, workspace_role: owner }]), false)
  assert.equal(isLastOwner(a, [a, { ...b, workspace_role: owner, status: 'deactivated' }]), true)
})

test('defaults, project roles, invites and summaries', () => {
  const roles = [role('r-owner', 'owner', ['nodes.read', 'members.manage']), role('r-member', 'member', ['nodes.read']), role('r-guest', 'guest', ['nodes.read']), role('r-customer', 'customer', ['portal.quotes'])]
  assert.equal(defaultWorkspaceRole(roles), 'r-member')
  assert.equal(defaultProjectRole(roles), 'r-guest')
  assert.deepEqual(projectRolesOf(roles, registry).map(r => r.key), ['owner', 'member', 'guest'])
  assert.equal(inviteRoles({ workspace_role: { id: 'r', key: 'member', name: 'Member' }, project_roles: [{ project_id: 'p', project_key: 'P', project_title: 'Pharos', role: { id: 'g', key: 'guest', name: 'Guest' } }] }), 'Member in the workspace · Guest on Pharos')
  assert.equal(inviteRoles({ workspace_role: null, project_roles: [] }), 'No access yet')
  assert.equal(projectSummary([]), '')
  const pr = (t: string) => ({ project_id: t, project_key: t, project_title: t, role: { id: 'g', key: 'guest', name: 'Guest' } })
  assert.equal(projectSummary([pr('A'), pr('B')]), 'A, B')
  assert.equal(projectSummary([pr('A'), pr('B'), pr('C')]), '3 projects')
  assert.equal(validEmail(' anna@studio.at '), true)
  assert.equal(validEmail('anna@'), false)
})

test('audit events read as sentences with their subject', () => {
  const names = { principal: (id?: string | null) => ({ me: 'Markus', mira: 'Mira', jw: 'jw (classic)' } as Record<string, string>)[id ?? ''] ?? 'someone', project: (id?: string | null) => id === 'p1' ? 'Pharos' : 'a project' }
  const e = (type: string, before: unknown, after: unknown) => ({ id: 1, actor_principal_id: 'me', type, before, after, at: '2026-09-23T10:00:00Z' })
  assert.deepEqual(auditSentence(e('binding.set', { principal_id: 'mira', scope: 'project', project_id: 'p1', role: { name: 'Member' } }, { principal_id: 'mira', scope: 'project', project_id: 'p1', role: { name: 'Guest' } }), names),
    { actor: 'Markus', subject: 'Mira', text: 'changed Mira’s role on Pharos from Member to Guest' })
  assert.equal(auditSentence(e('binding.set', null, { principal_id: 'mira', scope: 'workspace', role: { name: 'Admin' } }), names).text, 'gave Mira the role Admin in the workspace')
  assert.equal(auditSentence(e('role.updated', { name: 'Lead', permissions: ['a'] }, { name: 'Lead', permissions: ['a', 'b', 'c'] }), names).text, 'changed the role Lead (2 added)')
  assert.equal(auditSentence(e('principal.alias_linked', null, { principal_id: 'mira', from_principal_id: 'jw' }), names).text, 'linked jw (classic) to Mira')
  assert.equal(auditSentence(e('agent_key.revoked', null, { principal_id: 'x', name: 'deployer', prefix: 'ph4r' }), names).text, 'revoked the key deployer (aeon_ph4r_…)')
  assert.equal(auditSentence(e('something.else', null, {}), names).text, 'something else')
  assert.deepEqual(['role.created', 'binding.removed', 'invite.revoked', 'principal.deactivated', 'agent_key.created', 'node.updated'].map(categoryOf), ['roles', 'bindings', 'invites', 'lifecycle', 'keys', null])
})

test('errors carry the server’s reason and field', () => {
  const error = new AccessError(409, { error: 'conflict', code: 'last_owner', reason: 'The last owner stays.', field: 'role_id' })
  assert.equal(error.message, 'The last owner stays.')
  assert.equal(error.field, 'role_id')
  assert.equal(error.code, 'last_owner')
})

test('can() answers from /api/me/permissions: workspace, then per project', async () => {
  const asked: string[] = []
  globalThis.fetch = (async (input: string) => {
    asked.push(input)
    const project = new URL(input, 'http://x').searchParams.get('project_id')
    return new Response(JSON.stringify({ workspace: { role: { id: 'r', key: 'member', name: 'Member' }, permissions: ['nodes.read'] }, project: project ? { id: project, role: { id: 'g', key: 'lead', name: 'Lead' }, permissions: ['members.manage'] } : null }), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }) as typeof fetch
  resetPermissions()
  assert.equal(can('nodes.read'), false) // unknown until asked; asks once
  await loadPermissions()
  assert.equal(can('nodes.read'), true)
  assert.equal(can('members.manage'), false)
  assert.equal(myWorkspaceRole()?.name, 'Member')
  await loadPermissions('p1')
  assert.equal(can('members.manage', 'p1'), true)
  assert.equal(can('nodes.read', 'p1'), true)
  assert.equal(can('members.manage', 'p2'), false)
  await refreshPermissions()
  assert.ok(asked.some(url => url.endsWith('/api/me/permissions?project_id=p1')))
})

test('an older server without /api/me/permissions grants nothing', async () => {
  globalThis.fetch = (async () => new Response('{}', { status: 404 })) as unknown as typeof fetch
  resetPermissions()
  await loadPermissions()
  assert.equal(permissionsAvailable.value, false)
  assert.equal(can('members.read'), false)
})
