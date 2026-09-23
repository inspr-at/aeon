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
