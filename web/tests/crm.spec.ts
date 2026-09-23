// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect, type Page } from '@playwright/test'

const orgId = '10000000-0000-4000-8000-000000000001'
const contactId = '10000000-0000-4000-8000-000000000002'
const projectId = '10000000-0000-4000-8000-000000000003'
const quoteId = '10000000-0000-4000-8000-000000000004'
const personId = '20000000-0000-4000-8000-000000000009'
const stamp = '2026-09-23T10:00:00Z'
const kinds = [
  { id: 'k-org', slug: 'organisation', label: 'Organisation', short_prefix: 'ORG', icon: 'organisation', allowed_child_kinds: [], field_schema: {} },
  { id: 'k-contact', slug: 'contact', label: 'Contact', short_prefix: 'CON', icon: 'contact', allowed_child_kinds: [], field_schema: {} },
  { id: 'k-project', slug: 'project', label: 'Project', short_prefix: 'PRJ', icon: 'project', allowed_child_kinds: null, field_schema: {} },
  { id: 'k-quote', slug: 'quote', label: 'Quote', short_prefix: 'QUO', icon: 'quote', allowed_child_kinds: [], field_schema: {} },
]
function node(id: string, kind: string, key: string, title: string) {
  return { id, key, kind_id: kind, title, body: '', fields: {}, state: 'open', parent_id: null, position: '1', created_at: stamp, updated_at: stamp }
}

async function setup(page: Page, options: { admin?: boolean; empty?: boolean } = {}) {
  const state = {
    nodes: options.empty ? [] : [
      node(orgId, 'k-org', 'ORG-1', 'Acme'),
      node(contactId, 'k-contact', 'CON-1', 'Ada'),
      node(projectId, 'k-project', 'PRJ-1', 'Harbour'),
      node(quoteId, 'k-quote', 'QUO-1', 'Spring offer'),
    ],
    relations: [] as { id: string; source_node_id: string; target_node_id: string; type: string; created_at: string }[],
    failBind: false,
    failLink: false,
  }
  const calls: { path: string; method: string; body: unknown }[] = []
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url())
    const path = url.pathname
    const method = route.request().method()
    const body = method === 'GET' ? null : route.request().postDataJSON()
    calls.push({ path, method, body })
    if (path === '/api/me') return route.fulfill({ json: { principal: { id: 'person-1', name: 'Markus Barta', kind: 'person', roles: options.admin === false ? ['member'] : ['admin'] }, tenant: { id: 'tenant-1', name: 'INSPR Studio' } } })
    if (path === '/api/version') return route.fulfill({ json: { version: '260923143005.0.0', scheme: 'inspr-calendar-v2' } })
    if (path === '/api/kinds') return route.fulfill({ json: { items: options.empty ? [] : kinds } })
    if (path === '/api/nodes' && method === 'GET') {
      const kind = url.searchParams.get('kind_id')
      return route.fulfill({ json: { items: state.nodes.filter(item => item.kind_id === kind), next_cursor: null } })
    }
    if (path === '/api/nodes' && method === 'POST') {
      const created = node(`node-${state.nodes.length + 1}`, String(body.kind_id), 'ORG-9', String(body.title))
      state.nodes.push(created)
      return route.fulfill({ status: 201, json: created })
    }
    if (path === '/api/relations' && method === 'GET') {
      const id = url.searchParams.get('node_id')
      const items = state.relations.filter(rel => rel.source_node_id === id || rel.target_node_id === id)
      return route.fulfill({ json: { items, next_cursor: null } })
    }
    if (path === '/api/relations' && method === 'POST') {
      if (state.failLink) return route.fulfill({ status: 409, json: { code: 'conflict', message: 'relation kind or direction is not allowed' } })
      const created = { id: `rel-${state.relations.length + 1}`, source_node_id: body.source_node_id, target_node_id: body.target_node_id, type: body.type, created_at: stamp }
      state.relations.push(created)
      return route.fulfill({ status: 201, json: created })
    }
    if (path === `/api/crm/contacts/${contactId}/principals` && method === 'POST') {
      if (state.failBind) return route.fulfill({ status: 409, json: { code: 'conflict', message: 'business_crm is not enabled for this operation' } })
      return route.fulfill({ status: 201, json: { principal_id: body.principal_id, contact_node_id: contactId, bound_by_principal_id: 'person-1', bound_at: stamp } })
    }
    return route.fulfill({ status: 404, json: { error: 'Uncontracted route' } })
  })
  return { state, calls }
}

test('administrator binds a person and records a directed customer link', async ({ page }) => {
  const { calls } = await setup(page)
  await page.goto('/crm')
  await expect(page.getByRole('heading', { name: 'CRM' })).toBeVisible()
  await expect(page.getByText('A matching email or display name never does.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Ada' })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByLabel('Person principal')).toHaveAttribute('type', 'text')
  await page.getByLabel('Person principal').fill(personId)
  await page.getByRole('button', { name: 'Bind person' }).click()
  await expect(page.getByRole('status')).toContainText(personId)
  expect(calls.find(call => call.path.endsWith('/principals'))?.body).toEqual({ principal_id: personId })
  await page.getByLabel('Link type').selectOption('customer_of')
  await expect(page.getByLabel('Link source').locator('option')).toHaveText(['Acme'])
  await expect(page.getByLabel('Link target').locator('option')).toHaveText(['Harbour', 'Spring offer'])
  await page.getByLabel('Link target').selectOption({ label: 'Harbour' })
  await page.getByRole('button', { name: 'Create link' }).click()
  await expect(page.getByText('Acme customer of Harbour')).toBeVisible()
  const link = calls.find(call => call.path === '/api/relations' && call.method === 'POST')
  expect(link?.body).toEqual({ source_node_id: orgId, target_node_id: projectId, type: 'customer_of' })
})

test('contact links offer only a contact as the source', async ({ page }) => {
  await setup(page)
  await page.goto('/crm')
  await expect(page.getByLabel('Link source').locator('option')).toHaveText(['Ada'])
  await expect(page.getByLabel('Link target').locator('option')).toHaveText(['Acme', 'Harbour', 'Spring offer'])
})

test('a closed installation does not claim the person was bound', async ({ page }) => {
  const { state, calls } = await setup(page)
  state.failBind = true
  await page.goto('/crm')
  await page.getByLabel('Person principal').fill(personId)
  await page.getByRole('button', { name: 'Bind person' }).click()
  await expect(page.getByRole('alert')).toContainText('business_crm is not enabled')
  await expect(page.getByRole('status')).toHaveCount(0)
  expect(calls.filter(call => String(call.path).endsWith('/principals'))).toHaveLength(1)
  state.failBind = false
  await page.getByRole('button', { name: 'Bind person' }).click()
  await expect(page.getByRole('status')).toContainText('Bound principal')
})

test('members can read the directory and cannot bind or create', async ({ page }) => {
  const { calls } = await setup(page, { admin: false })
  await page.goto('/crm')
  await expect(page.getByRole('heading', { name: 'Acme', exact: false })).toBeHidden()
  await expect(page.getByRole('button', { name: 'Acme' })).toBeVisible()
  await expect(page.getByText('requires an administrator')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Bind person' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Create organisation' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Create link' })).toHaveCount(0)
  await expect(page.getByLabel('Person principal')).toHaveCount(0)
  expect(calls.every(call => call.method === 'GET')).toBe(true)
})

test('missing kinds explain that CRM nodes are not configured', async ({ page }) => {
  await setup(page, { empty: true })
  await page.goto('/crm')
  await expect(page.getByText('Organisation and contact kinds are not configured for this tenant.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Create organisation' })).toHaveCount(0)
})

test('creating an organisation posts a node of that kind', async ({ page }) => {
  const { calls } = await setup(page, { empty: false })
  await page.goto('/crm')
  await page.getByLabel('Organisation name').fill('Northwind')
  await page.getByRole('button', { name: 'Create organisation' }).click()
  await expect(page.getByRole('button', { name: 'Northwind' })).toBeVisible()
  expect(calls.find(call => call.path === '/api/nodes' && call.method === 'POST')?.body).toEqual({ kind_id: 'k-org', title: 'Northwind' })
})
