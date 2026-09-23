// SPDX-License-Identifier: AGPL-3.0-only
import { afterEach, test } from 'node:test'
import assert from 'node:assert/strict'
import { api, getSession } from '../src/lib/api.ts'

const originalFetch = globalThis.fetch
afterEach(() => { globalThis.fetch = originalFetch })

function respond(body: unknown, status = 200) {
  globalThis.fetch = async () => new Response(JSON.stringify(body), {
    status, headers: { 'Content-Type': 'application/json' },
  })
}

test('accepts the authenticated principal and tenant', async () => {
  const identity = { principal: { id: 'p1', name: 'Markus Barta' }, tenant: { id: 't1', name: 'INSPR' } }
  respond(identity)
  assert.deepEqual(await getSession(), { identity, devMode: false })
})

test('a bare 401 still redirects to sign-in with development disabled', async () => {
  globalThis.fetch = async () => new Response(null, { status: 401 })
  assert.deepEqual(await getSession(), { identity: null, devMode: false })
})

test('only a literal server true enables development sign-in', async () => {
  for (const dev_mode of [undefined, null, false, 'true', 1]) {
    respond({ dev_mode }, 401)
    assert.equal((await getSession()).devMode, false)
  }
  respond({ dev_mode: true }, 401)
  assert.deepEqual(await getSession(), { identity: null, devMode: true })
})

test('rejects malformed identities and server errors', async () => {
  for (const body of [{}, { principal: { name: 'Someone' }, tenant: { name: 'INSPR' } }]) {
    respond(body)
    await assert.rejects(getSession(), /Invalid session/)
  }
  respond({ dev_mode: true }, 503)
  await assert.rejects(getSession(), /Session unavailable/)
})

test('API requests use same-origin credentials, no cache and a timeout signal', async () => {
  globalThis.fetch = async (url, options) => {
    assert.equal(url, '/api/auth/logout')
    assert.equal(options?.method, 'POST')
    assert.equal(options?.credentials, 'same-origin')
    assert.equal(options?.cache, 'no-store')
    assert.ok(options?.signal instanceof AbortSignal)
    assert.equal(new Headers(options?.headers).get('Accept'), 'application/json')
    return new Response(null, { status: 204 })
  }
  assert.equal((await api('/auth/logout', { method: 'POST' })).status, 204)
})

test('R1 list and search calls encode filters and opaque cursors', async () => {
  const { getNodes, searchNodes } = await import('../src/lib/api.ts')
  const urls: string[] = []
  globalThis.fetch = async url => { urls.push(String(url)); return Response.json({ items: [], next_cursor: null }) }
  await getNodes({ kind_id: 'kind-1', state: 'in progress', parent_id: 'root', include_descendants: false, sort: 'title', direction: 'desc', cursor: 'a+/=' })
  await searchNodes('a & b/文', { cursor: 'next=2' })
  const list = new URL(urls[0]!, 'https://aeon.invalid')
  assert.equal(list.searchParams.get('cursor'), 'a+/=')
  assert.equal(list.searchParams.get('state'), 'in progress')
  assert.equal(list.searchParams.get('include_descendants'), 'false')
  assert.equal(list.searchParams.get('sort'), 'title')
  const search = new URL(urls[1]!, 'https://aeon.invalid')
  assert.equal(search.searchParams.get('q'), 'a & b/文')
})

test('R1 mutations send JSON, tolerate 204, and surface API failures', async () => {
  const { updateNode, deleteView, APIError } = await import('../src/lib/api.ts')
  globalThis.fetch = async (url, init) => {
    assert.equal(url, '/api/nodes/node%2Fid')
    assert.equal(init?.method, 'PATCH')
    assert.equal(new Headers(init?.headers).get('Content-Type'), 'application/json')
    assert.deepEqual(JSON.parse(String(init?.body)), { body: '# Changed' })
    return Response.json({ id: 'node/id', body: '# Changed' })
  }
  assert.equal((await updateNode('node/id', { body: '# Changed' })).body, '# Changed')
  globalThis.fetch = async () => new Response(null, { status: 204 })
  assert.equal(await deleteView('view-1'), undefined)
  respond({ error: 'Owner only' }, 403)
  await assert.rejects(deleteView('view-1'), error => error instanceof APIError && error.status === 403 && error.message === 'Owner only')
  globalThis.fetch = async () => new Response('gateway unavailable', { status: 502 })
  await assert.rejects(deleteView('view-1'), /Request failed \(502\)/)
})

test('EventSource listens to named R1 events, reconnects and releases its connection', async () => {
  const { subscribeWorkspace } = await import('../src/lib/api.ts')
  const original = globalThis.EventSource
  let changes = 0, closed = false, stream: MockStream | undefined
  const connections: boolean[] = []
  class MockStream extends EventTarget {
    onopen?: () => void
    onerror?: () => void
    onmessage?: () => void
    constructor(url: string) { super(); assert.equal(url, '/api/events/stream'); stream = this }
    close() { closed = true }
  }
  globalThis.EventSource = MockStream as unknown as typeof EventSource
  try {
    const stop = subscribeWorkspace(() => changes++, live => connections.push(live))
    stream!.onopen!()
    stream!.dispatchEvent(new Event('node.updated'))
    stream!.dispatchEvent(new Event('view.created'))
    stream!.dispatchEvent(new Event('kind.deleted'))
    stream!.onerror!(); stream!.onopen!()
    assert.equal(changes, 5)
    assert.deepEqual(connections, [true, false, true])
    stop(); assert.equal(closed, true)
  } finally { globalThis.EventSource = original }
})
