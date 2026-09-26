// SPDX-License-Identifier: AGPL-3.0-only
import { afterEach, describe, expect, it, vi } from 'vitest'
import { accessChanged, can, clearPermissions, myPermissions, myWorkspaceRole, permissionsAvailable, permissionsKnown, refreshPermissions } from '../src/lib/authz'

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
  it('keeps the current answer while an access change is asked again (no flicker)', async () => {
    let finish!: (value: { ok: boolean; json: () => Promise<unknown> }) => void
    const fetch = vi.fn()
      .mockImplementationOnce(async () => ({ ok: true, json: async () => ({ workspace: { role: null, permissions: ['members.read'] }, project: null }) }))
      .mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    vi.stubGlobal('fetch', fetch)
    await refreshPermissions()
    const pending = accessChanged()
    expect(can('members.read')).toBe(true)
    finish({ ok: true, json: async () => ({ workspace: { role: null, permissions: [] }, project: null }) })
    await pending
    expect(can('members.read')).toBe(false)
  })
  it('ignores an answer started before sign-out', async () => {
    let finishOld!: (value: { ok: boolean; json: () => Promise<unknown> }) => void
    const old = new Promise<{ ok: boolean; json: () => Promise<unknown> }>(resolve => { finishOld = resolve })
    const fetch = vi.fn().mockImplementationOnce(() => old).mockImplementationOnce(async () => ({ ok: true, json: async () => ({ workspace: { role: null, permissions: [] }, project: null }) }))
    vi.stubGlobal('fetch', fetch)
    const pending = refreshPermissions()
    clearPermissions()
    await refreshPermissions()
    finishOld({ ok: true, json: async () => ({ workspace: { role: null, permissions: ['keys.manage'] }, project: null }) })
    await pending
    expect(can('keys.manage')).toBe(false)
  })
  it('Access reads my role and permissions from the same answer', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ workspace: { role: { id: 'r', key: 'member', name: 'Member' }, permissions: ['nodes.read', 'members.read'] }, project: null }) })))
    expect(permissionsKnown()).toBe(false)
    await refreshPermissions()
    expect(permissionsKnown()).toBe(true)
    expect(permissionsAvailable()).toBe(true)
    expect(myWorkspaceRole()?.name).toBe('Member')
    expect([...myPermissions()].sort()).toEqual(['members.read', 'nodes.read'])
  })
  it('a server without the endpoint is known, unavailable and grants nothing', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false })))
    await refreshPermissions()
    expect(permissionsKnown()).toBe(true)
    expect(permissionsAvailable()).toBe(false)
    expect(myPermissions().size).toBe(0)
  })
})
