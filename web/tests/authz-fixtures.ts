// SPDX-License-Identifier: AGPL-3.0-only
// Effective permissions for UI mocks. Access decisions are driven by this
// endpoint, independently of deprecated principal.roles in /api/me.
export type MockRole = 'admin' | 'member' | 'viewer' | 'customer'

const common = ['authz.read', 'nodes.read', 'kinds.read', 'knowledge.read', 'journey.read', 'views.read', 'search.read', 'profile.read']
const member = [...common, 'nodes.write', 'nodes.move', 'nodes.delete', 'relations.read', 'relations.write', 'relations.delete', 'attachments.read', 'attachments.write', 'attachments.delete', 'knowledge.write', 'knowledge.delete', 'comments.write', 'comments.delete', 'journey.act', 'releases.write', 'requirements.write', 'harness.read', 'harness.control', 'run.claim', 'work_orders.write', 'approvals.decide', 'inbox.read', 'inbox.send', 'crm.read', 'crm.write', 'hours.read', 'hours.write', 'quotes.read', 'quotes.write', 'profile.write']
const admin = [...member, 'approvals.decide_high', 'intake.decide', 'inbox.manage', 'settings.manage', 'plugins.manage', 'project_groups.write', 'keys.read', 'keys.manage', 'members.read', 'members.manage', 'roles.read', 'roles.manage', 'quotes.manage', 'quotes.issue', 'hours.approve']

export function mockEffectivePermissions(role: MockRole, projectId?: string) {
  const permissions = role === 'admin' ? admin : role === 'member' ? member : role === 'customer' ? ['authz.read', 'profile.read', 'profile.write', 'quotes.portal_read', 'quotes.portal_accept'] : common
  const ref = { id: `role-${role}`, key: role, name: role[0].toUpperCase() + role.slice(1) }
  return { workspace: { role: ref, permissions }, project: projectId ? { id: projectId, role: null, permissions } : null }
}
