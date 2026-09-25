// SPDX-License-Identifier: AGPL-3.0-only
import { afterEach, describe, expect, it, vi } from 'vitest'
import { accessChanged, can, clearPermissions, refreshPermissions } from '../src/lib/authz'

afterEach(() => { clearPermissions(); vi.unstubAllGlobals() })

describe('can()', () => {
  it('fails closed until the server returns effective grants', async () => {
    const fetch = vi.fn(async () => ({ ok: true, json: async () => ({ workspace: { role: null, permissions: ['nodes.read'] }, project: null }) }))
    vi.stubGlobal('fetch', fetch)
    expect(can('nodes.read')).toBe(false)
    await refreshPermissions()
    expect(can('nodes.read')).toBe(true)
    expect(can('nodes.write')).toBe(false)
  })
  it('unions project and workspace permissions only for that project', async () => {
    const fetch = vi.fn(async (url: string) => ({ ok: true, json: async () => ({
      workspace: { role: null, permissions: ['nodes.read'] },
      project: url.includes('project_id=a') ? { id: 'a', role: null, permissions: ['comments.write'] } : null,
    }) }))
    vi.stubGlobal('fetch', fetch)
    await refreshPermissions('a')
    expect(can('nodes.read', 'a')).toBe(true)
    expect(can('comments.write', 'a')).toBe(true)
    expect(can('comments.write')).toBe(false)
    await refreshPermissions('b')
    expect(can('comments.write', 'b')).toBe(false)
  })
  it('drops revoked access after an access change or failed refresh', async () => {
    let allowed = true
    vi.stubGlobal('fetch', vi.fn(async () => allowed
      ? { ok: true, json: async () => ({ workspace: { role: null, permissions: ['keys.manage'] }, project: null }) }
      : { ok: false }))
    await refreshPermissions()
    expect(can('keys.manage')).toBe(true)
    allowed = false
    await accessChanged()
    expect(can('keys.manage')).toBe(false)
  })
})
