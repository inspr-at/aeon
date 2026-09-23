// SPDX-License-Identifier: AGPL-3.0-only
import { test, expect, type Page } from '@playwright/test'

const kind = { id: 'kind-project', slug: 'project', label: 'Project', short_prefix: 'PR', icon: 'tree', allowed_child_kinds: null, field_schema: {} }
const taskKind = { ...kind, id: 'kind-experiment', slug: 'experiment', label: 'Experiment', short_prefix: 'EXP', allowed_child_kinds: [] }
const timestamp = '2026-09-23T12:00:00Z'
const root = { id: 'root-1', key: 'PR-1', kind_id: kind.id, title: 'First project', body: '# Context\n\nA **shared** plan.\n\n- First item\n- Second item', state: 'open', fields: {}, parent_id: null, position: '1', created_at: timestamp, updated_at: timestamp }
const child = { ...root, id: 'child-1', key: 'EXP-1', kind_id: taskKind.id, title: 'Try a new idea', parent_id: root.id, body: 'An experiment.' }
const saved = { id: 'view-1', name: 'Open experiments', filters: { kind_id: taskKind.id, state: 'open', parent_id: root.id, include_descendants: true }, sort: { field: 'title', direction: 'desc' }, columns: ['key', 'state'], shared: false, owner_principal_id: 'person-1', created_at: timestamp, updated_at: timestamp }

async function setup(page: Page, options: { failTree?: boolean; failPatch?: boolean; pagination?: boolean } = {}) {
  const state = { root: { ...root }, child: { ...child }, views: [{ ...saved }], failTree: options.failTree ?? false, failPatch: options.failPatch ?? false, deleted: false }
  const calls: { path: string; method: string; query: URLSearchParams; body: any }[] = []
  await page.addInitScript(() => {
    class MockEventSource extends EventTarget {
      onopen: (() => void) | null = null
      onerror: (() => void) | null = null
      onmessage: (() => void) | null = null
      constructor(url: string) {
        super()
        Object.assign(window, {
          streamURL: url,
          streamClosed: false,
          emitWorkspaceEvent: (type: string) => this.dispatchEvent(new MessageEvent(type, { data: '{}' })),
          streamReconnect: () => this.onopen?.(),
          streamDisconnect: () => this.onerror?.(),
        })
        setTimeout(() => this.onopen?.(), 0)
      }
      close() { Object.assign(window, { streamClosed: true }) }
    }
    Object.assign(window, { EventSource: MockEventSource })
  })
  await page.route('**/api/**', async route => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname, method = request.method()
    const body = request.postDataJSON()
    calls.push({ path, method, query: url.searchParams, body })
    if (path === '/api/me') return route.fulfill({ json: { principal: { id: 'person-1', name: 'Markus Barta' }, tenant: { id: 'tenant-1', name: 'INSPR Studio' } } })
    if (path === '/api/version') return route.fulfill({ json: { version: 'dev', scheme: 'inspr-calendar-v2' } })
    if (path === '/api/kinds') return route.fulfill({ json: { items: [kind, taskKind] } })
    if (path === '/api/views' && method === 'GET') return route.fulfill({ json: { items: state.views } })
    if (path === '/api/views' && method === 'POST') {
      const view = { ...saved, ...body, id: 'view-2' }; state.views.push(view)
      return route.fulfill({ status: 201, json: view })
    }
    if (path.startsWith('/api/views/') && method === 'PATCH') {
      const view = { ...state.views.find(view => path.endsWith(view.id)), ...body }
      state.views = state.views.map(existing => existing.id === view.id ? view : existing)
      return route.fulfill({ json: view })
    }
    if (path.startsWith('/api/views/') && method === 'DELETE') { state.views = state.views.filter(view => !path.endsWith(view.id)); return route.fulfill({ status: 204 }) }
    if (path === '/api/nodes/tree') {
      if (state.failTree) return route.fulfill({ status: 503, json: { error: 'Tree temporarily unavailable' } })
      const items = [{ node: state.root, depth: 0 }, { node: state.child, depth: 1 }]
      return route.fulfill({ json: options.pagination ? { items: url.searchParams.has('cursor') ? [items[1]] : [items[0]], next_cursor: url.searchParams.has('cursor') ? null : 'tree-page-2' } : { items, next_cursor: null } })
    }
    if (path === '/api/nodes' && method === 'GET') return route.fulfill({ json: { items: [state.child], next_cursor: null } })
    if (path === '/api/nodes' && method === 'POST') return route.fulfill({ status: 201, json: { ...state.child, ...body } })
    if (path.startsWith('/api/nodes/')) {
      if (state.deleted) return route.fulfill({ status: 404, json: { error: 'Not found' } })
      if (method === 'DELETE') return route.fulfill({ status: 204 })
      if (path.endsWith('/move')) return route.fulfill({ json: { ...state.root, ...body } })
      if (method === 'PATCH') {
        if (state.failPatch) return route.fulfill({ status: 422, json: { error: 'Invalid state' } })
        Object.assign(state.root, body, { updated_at: '2026-09-23T13:00:00Z' })
      }
      return route.fulfill({ json: path.endsWith(root.id) ? state.root : state.child })
    }
    if (path === '/api/search') return route.fulfill({ json: { items: url.searchParams.get('q') === 'absent' ? [] : [{ node: state.child, score: 0.7 }], next_cursor: null } })
    return route.fulfill({ status: 404, json: { error: 'Unknown route' } })
  })
  return { state, calls }
}

async function openRoot(page: Page) { await page.getByRole('button', { name: 'Open PR-1: First project' }).click(); await expect(page.getByRole('heading', { name: 'First project' })).toBeVisible() }
async function emit(page: Page, name = 'node.updated') { await page.evaluate(name => (window as any).emitWorkspaceEvent(name), name) }

test('dynamic kinds, arbitrary-depth tree and cursor paging retain parent collapse', async ({ page }) => {
  const { calls } = await setup(page, { pagination: true })
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Open PR-1: First project' })).toBeVisible()
  await page.getByRole('button', { name: 'Load more work' }).click()
  await expect(page.getByRole('button', { name: 'Open EXP-1: Try a new idea' })).toBeVisible()
  expect(calls.some(call => call.path === '/api/nodes/tree' && call.query.get('cursor') === 'tree-page-2')).toBeTruthy()
  await emit(page)
  await expect.poll(() => calls.filter(call => call.path === '/api/nodes/tree' && call.query.has('cursor')).length).toBeGreaterThan(1)
  await expect(page.getByRole('button', { name: 'Open EXP-1: Try a new idea' })).toBeVisible()
  await page.getByRole('button', { name: 'Collapse PR-1' }).click()
  await expect(page.getByRole('button', { name: 'Open EXP-1: Try a new idea' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Expand PR-1' }).click()
  await expect(page.getByRole('list', { name: 'Work tree' }).getByText('Experiment', { exact: true })).toBeVisible()
})

test('list filters and saved views round trip filters, sorting and columns', async ({ page }) => {
  const { calls } = await setup(page)
  await page.goto('/')
  await page.getByText('Saved views', { exact: true }).click()
  await page.getByLabel('Open a view').selectOption('view-1')
  await expect(page.getByRole('button', { name: 'List', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.locator('.filters').getByLabel('Kind', { exact: true })).toHaveValue(taskKind.id)
  await expect(page.getByLabel('State', { exact: true })).toHaveValue('open')
  await expect(page.getByLabel('Include descendants')).toBeChecked()
  await expect(page.getByRole('list', { name: 'Work list' }).getByText('Experiment', { exact: true })).toHaveCount(0)
  await expect.poll(() => calls.some(call => call.path === '/api/nodes' && call.query.get('direction') === 'desc' && call.query.get('sort') === 'title' && call.query.get('include_descendants') === 'true')).toBeTruthy()
  await page.getByLabel('View name').fill('My experiments')
  await page.getByLabel('Share with workspace').check()
  await page.getByRole('button', { name: 'Save as new view' }).click()
  await expect.poll(() => calls.find(call => call.path === '/api/views' && call.method === 'POST')?.body).toEqual({ ...saved, name: 'My experiments', shared: true, id: undefined, owner_principal_id: undefined, created_at: undefined, updated_at: undefined })
  await expect(page.getByLabel('Open a view')).toHaveValue('view-2')
  await page.getByLabel('View name').fill('Renamed view')
  await page.getByRole('button', { name: 'Update view' }).click()
  await expect.poll(() => calls.some(call => call.method === 'PATCH' && call.body.name === 'Renamed view')).toBeTruthy()
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: 'Delete view' }).click()
  await expect(page.getByLabel('Open a view')).toHaveValue('')
})

test('global search opens by keyboard and selects with arrows and Enter', async ({ page }) => {
  const { calls } = await setup(page)
  await page.goto('/')
  await page.getByRole('button', { name: 'Search all work' }).focus()
  await page.keyboard.press('Control+k')
  await expect(page.getByRole('combobox', { name: 'Search all work' })).toBeFocused()
  await page.getByRole('combobox').fill('idea & plan')
  await expect(page.getByRole('listbox', { name: 'Search results' }).getByRole('option')).toContainText('Try a new idea')
  expect(calls.find(call => call.path === '/api/search')?.query.get('q')).toBe('idea & plan')
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'Try a new idea' })).toBeVisible()
  await page.getByRole('button', { name: 'Close node details' }).click()
  await page.keyboard.press('Control+k')
  await page.getByRole('combobox').fill('absent')
  await expect(page.getByText('No matching work.')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).not.toBeVisible()
})

test('Markdown renders safely; failed edits keep drafts and successful edits use PATCH', async ({ page }) => {
  const { state, calls } = await setup(page, { failPatch: true })
  state.root.body += '\n<script>window.pwned = true</script>\n\n[bad](javascript:alert(1))\n\n![remote](https://example.invalid/pixel.png)'
  await page.goto('/')
  await openRoot(page)
  await expect(page.getByRole('heading', { name: 'Context' })).toBeVisible()
  await expect(page.locator('.markdown-body strong')).toHaveText('shared')
  await expect(page.locator('.markdown-body script, .markdown-body img, .markdown-body a[href^="javascript:"]')).toHaveCount(0)
  await page.getByRole('button', { name: 'Edit node' }).click()
  await page.getByRole('complementary', { name: 'Node details' }).getByLabel('Markdown', { exact: true }).fill('# Revised\n\nA new body.')
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect(page.getByRole('alert')).toContainText('Invalid state')
  await expect(page.getByRole('complementary', { name: 'Node details' }).getByLabel('Markdown', { exact: true })).toHaveValue('# Revised\n\nA new body.')
  state.failPatch = false
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect(page.getByRole('heading', { name: 'Revised' })).toBeVisible()
  await page.screenshot({ path: '/tmp/aeon-p14b-desktop.png', fullPage: true })
  expect(calls.find(call => call.method === 'PATCH')?.body).toEqual({ title: root.title, body: '# Revised\n\nA new body.', state: 'open' })
})

test('named SSE refreshes rows, preserves drafts, detects conflicts and closes on navigation', async ({ page }) => {
  const { state } = await setup(page)
  await page.goto('/')
  await openRoot(page)
  await page.getByRole('button', { name: 'Edit node' }).click()
  await page.getByRole('complementary', { name: 'Node details' }).getByLabel('Markdown', { exact: true }).fill('My unsaved work')
  state.root.title = 'Changed by another principal'; state.root.updated_at = '2026-09-23T14:00:00Z'
  await emit(page)
  await expect(page.getByRole('button', { name: /Open PR-1: Changed by/ })).toBeVisible()
  await expect(page.getByText('Updated elsewhere.', { exact: false })).toBeVisible()
  await expect(page.getByRole('complementary', { name: 'Node details' }).getByLabel('Markdown', { exact: true })).toHaveValue('My unsaved work')
  await expect(page.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  page.once('dialog', dialog => dialog.dismiss())
  await page.getByRole('button', { name: 'Close node details' }).click()
  await expect(page.getByRole('complementary', { name: 'Node details' }).getByLabel('Markdown', { exact: true })).toBeVisible()
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: 'Cancel editing' }).click()
  await expect(page.getByRole('heading', { name: state.root.title })).toBeVisible()
  await page.evaluate(() => (window as any).streamDisconnect())
  await expect(page.getByText('Reconnecting…')).toBeVisible()
  state.root.title = 'After reconnect'
  await page.evaluate(() => (window as any).streamReconnect())
  await expect(page.getByRole('heading', { name: 'After reconnect' })).toBeVisible()
  expect(await page.evaluate(() => (window as any).streamURL)).toBe('/api/events/stream')
  await page.evaluate(() => { history.pushState({}, '', '/missing'); window.dispatchEvent(new PopStateEvent('popstate')) })
  await expect(page.getByRole('heading', { name: 'A little off the path.' })).toBeVisible()
  expect(await page.evaluate(() => (window as any).streamClosed)).toBe(true)
})

test('deleted nodes preserve a draft without permitting a save', async ({ page }) => {
  const { state } = await setup(page)
  await page.goto('/'); await openRoot(page)
  await page.getByRole('button', { name: 'Edit node' }).click()
  await page.getByRole('complementary', { name: 'Node details' }).getByLabel('Markdown', { exact: true }).fill('Keep me')
  state.deleted = true; await emit(page, 'node.deleted')
  await expect(page.getByRole('alert')).toContainText('no longer available')
  await expect(page.getByRole('complementary', { name: 'Node details' }).getByLabel('Markdown', { exact: true })).toHaveValue('Keep me')
  await expect(page.getByRole('button', { name: 'Save changes' })).toBeDisabled()
})

test('creation uses tenant kinds, selected parent and JSON fields', async ({ page }) => {
  const { calls } = await setup(page)
  await page.goto('/'); await openRoot(page)
  await page.getByRole('button', { name: 'New node', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Create a node' })
  await expect(dialog.getByLabel('Parent node ID')).toHaveValue(root.id)
  await dialog.getByLabel('Title', { exact: true }).fill('A fresh experiment')
  await dialog.getByLabel('Kind').selectOption(taskKind.id)
  await dialog.getByText('Custom fields (JSON)').click()
  await dialog.getByLabel('Fields', { exact: true }).fill('{"hypothesis":"Useful"}')
  await dialog.getByRole('button', { name: 'Create node', exact: true }).click()
  await expect(dialog).not.toBeVisible()
  expect(calls.find(call => call.path === '/api/nodes' && call.method === 'POST')?.body).toEqual({ title: 'A fresh experiment', kind_id: taskKind.id, body: '', parent_id: root.id, fields: { hypothesis: 'Useful' } })
})

test('tree failures are actionable and retry recovers', async ({ page }) => {
  const { state } = await setup(page, { failTree: true })
  await page.goto('/')
  await expect(page.getByRole('alert')).toContainText('Tree temporarily unavailable')
  state.failTree = false
  await page.getByRole('button', { name: 'Retry work' }).click()
  await expect(page.getByRole('button', { name: 'Open PR-1: First project' })).toBeVisible()
})

for (const colorScheme of ['light', 'dark'] as const) {
  test(`mobile workspace and Markdown sidebar fit the shell in ${colorScheme}`, async ({ page }) => {
    await setup(page)
    await page.setViewportSize({ width: 390, height: 844 }); await page.emulateMedia({ colorScheme })
    await page.goto('/'); await openRoot(page)
    await expect(page.getByRole('heading', { name: 'Context' })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    expect((await page.getByRole('complementary', { name: 'Node details' }).boundingBox())?.y).toBe(64)
    await expect(page.getByRole('button', { name: 'Tree', exact: true })).not.toBeVisible()
    await page.screenshot({ path: `/tmp/aeon-p14b-mobile-${colorScheme}.png`, fullPage: true })
    await page.getByRole('button', { name: 'Close node details' }).click()
    await page.screenshot({ path: `/tmp/aeon-p14b-workspace-${colorScheme}.png`, fullPage: true })
  })
}


test('move and delete use their dedicated R1 endpoints', async ({ page }) => {
  const { calls } = await setup(page)
  await page.goto('/'); await openRoot(page)
  await page.getByRole('button', { name: 'Move node', exact: true }).click()
  await page.getByLabel('New parent ID').fill('destination')
  await page.getByLabel('Before sibling ID').fill('sibling')
  await page.getByRole('button', { name: 'Confirm move' }).click()
  await expect.poll(() => calls.find(call => call.path.endsWith('/move'))?.body).toEqual({ parent_id: 'destination', before_id: 'sibling' })
  page.once('dialog', dialog => dialog.accept())
  await page.getByRole('button', { name: 'Delete node', exact: true }).click()
  await expect(page.getByRole('complementary', { name: 'Node details' })).toHaveCount(0)
  expect(calls.some(call => call.path === '/api/nodes/root-1' && call.method === 'DELETE')).toBeTruthy()
})
