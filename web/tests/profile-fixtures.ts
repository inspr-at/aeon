// SPDX-License-Identifier: AGPL-3.0-only
// An in-memory document profile API (U19, AEON-110) with the server's rules
// (internal/business/quotes/profiles.go): revisions on every save, stale
// revisions refused, archived profiles frozen, undo that restores the previous
// revision, unarchives, or archives a first revision, archive clearing the
// default for new quotes, uploaded assets served same-origin, and the quote
// settings' default_profile_id. Layered over mockWork and mockBusiness. Every
// name, colour and file is invented.
import { readFileSync } from 'node:fs'
import type { Page } from '@playwright/test'
import { defaultProfile } from '../src/lib/quotes/profile'
import { switchLocale } from '../src/lib/quotes/profileForm'
import type { QuoteProfileDefinition } from '../src/lib/quotes/types'

export const FONT = readFileSync(new URL('../src/assets/fonts/jetbrains.woff2', import.meta.url))
export const PROFILE = { steel: '7e5a0000-0000-4000-8000-000000000001', plain: '7e5a0000-0000-4000-8000-000000000002', old: '7e5a0000-0000-4000-8000-000000000003' }
const FONT_ASSET = '7e5aa55e-0000-4000-8000-000000000001'
const MARK_ASSET = '7e5aa55e-0000-4000-8000-000000000002'
// A small mark: a rounded square and a bar, in the profile's accent.
export const MARK_SVG = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 40"><rect x="2" y="4" width="32" height="32" rx="8" fill="#2a6f86"/><rect x="44" y="14" width="72" height="12" rx="6" fill="#23363a"/></svg>'

interface Revision { name: string; definition: QuoteProfileDefinition }
interface Stored { id: string; revisions: Revision[]; archived: boolean }
export interface ProfileWorld {
  profiles: Stored[]
  assets: Map<string, { type: string; body: Buffer | string }>
  settings: Record<string, unknown> & { revision: number; default_profile_id?: string }
  counter: { next: number }
}

// Beispiel Stahl: the classic layout in steel blue, with its own font and mark.
export function steelDefinition(options: { font?: boolean } = {}): QuoteProfileDefinition {
  const d = defaultProfile()
  d.colors = { ink: '#23363a', muted: '#4f6468', soft: '#627477', accent: '#2a6f86', rule: '#cfdadc', paper: '#ffffff' }
  d.fonts = options.font ? [{ role: 'display', family: 'Beispiel Mono', weight: 400, style: 'normal', asset_id: FONT_ASSET }] : []
  d.footer = { ...d.footer, asset_id: MARK_ASSET }
  return d
}
export function profileWorld(options: { empty?: boolean; defaultId?: string; font?: boolean } = {}): ProfileWorld {
  const plain = defaultProfile()
  plain.layout_variant = 'standard'; switchLocale(plain, 'en')
  plain.colors = { ink: '#1f2d2f', muted: '#56676a', soft: '#a4b1b3', accent: '#8a5a2b', rule: '#dde3e3', paper: '#fffdf8' }
  const old = defaultProfile()
  old.colors = { ...old.colors, accent: '#7a4f9a' }
  const profiles: Stored[] = options.empty ? [] : [
    { id: PROFILE.steel, revisions: [{ name: 'Beispiel Stahl GmbH', definition: steelDefinition() }, { name: 'Beispiel Stahl GmbH', definition: steelDefinition(options) }], archived: false },
    { id: PROFILE.plain, revisions: [{ name: 'Studio English', definition: plain }], archived: false },
    { id: PROFILE.old, revisions: [{ name: 'Messe 2025', definition: old }], archived: true },
  ]
  return {
    profiles,
    assets: new Map([[FONT_ASSET, { type: 'font/woff2', body: FONT }], [MARK_ASSET, { type: 'image/svg+xml', body: MARK_SVG }]]),
    settings: { revision: 2, numbering_time_zone: 'Europe/Vienna', default_currency: 'EUR', sender: { company: 'Beispiel Studio GmbH', street: 'Musterweg 1', postal_code: '8010', city: 'Graz', country: 'Österreich', email: 'hallo@beispiel.invalid' }, defaults: {}, layout: {}, smtp_confirmation_enabled: false, smtp_configured: false, ...(options.defaultId === '' || options.empty ? {} : { default_profile_id: options.defaultId ?? PROFILE.steel }) },
    counter: { next: 10 },
  }
}
const row = (p: Stored, revision = p.revisions.length) => ({ id: p.id, name: p.revisions[revision - 1]!.name, revision, definition: structuredClone(p.revisions[revision - 1]!.definition), archived: p.archived })

export async function mockProfiles(page: Page, world: ProfileWorld, options: { failUpload?: boolean } = {}) {
  const calls: { method: string; path: string; body: unknown }[] = []
  await page.route(/\/api\/(quote-profiles|quotes\/settings|quotes\/[^/]+\/profile)(\/|\?|$)/, async route => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname, method = request.method()
    let body: Record<string, unknown> = {}
    try { body = request.postDataJSON() ?? {} } catch { body = {} }
    calls.push({ method, path, body: method === 'POST' && path.endsWith('/assets') ? '<file>' : body })
    const bad = (status: number, error: string) => route.fulfill({ status, json: { error } })
    if (path === '/api/quotes/settings') {
      if (method === 'GET') return route.fulfill({ json: world.settings })
      if (body.expected_revision !== world.settings.revision) return bad(409, 'settings revision is stale')
      const next: ProfileWorld['settings'] = { ...world.settings, ...body, revision: world.settings.revision + 1 }
      delete (next as Record<string, unknown>).expected_revision
      if (!body.default_profile_id) delete next.default_profile_id
      world.settings = next
      return route.fulfill({ json: world.settings })
    }
    if (/^\/api\/quotes\/[^/]+\/profile$/.test(path)) return route.fallback()
    if (path === '/api/quote-profiles/assets' && method === 'POST') {
      if (options.failUpload) return bad(400, 'expected TTF, OTF, WOFF2, PNG or safe SVG')
      const raw = request.postDataBuffer() ?? Buffer.alloc(0)
      const head = raw.subarray(0, 4).toString('latin1')
      const type = head === 'wOF2' ? 'font/woff2' : head === 'OTTO' ? 'font/otf' : raw[0] === 0 && raw[1] === 1 ? 'font/ttf' : head === '\x89PNG' ? 'image/png' : raw.toString('utf8').trimStart().startsWith('<svg') ? 'image/svg+xml' : ''
      if (!type) return bad(400, 'expected TTF, OTF, WOFF2, PNG or safe SVG')
      const id = `7e5aa55e-0000-4000-8000-${String(++world.counter.next).padStart(12, '0')}`
      // Fonts are served as the repository's own font so the specimen can load.
      world.assets.set(id, type.startsWith('font/') ? { type: 'font/woff2', body: FONT } : { type, body: raw })
      return route.fulfill({ status: 201, json: { id, sha256: 'ab'.repeat(32), content_type: type, size: raw.length } })
    }
    const asset = /^\/api\/quote-profiles\/assets\/([^/]+)$/.exec(path)
    if (asset) {
      const found = world.assets.get(asset[1]!)
      return found ? route.fulfill({ body: found.body, contentType: found.type }) : bad(404, 'not found')
    }
    if (path === '/api/quote-profiles') {
      if (method === 'GET') return route.fulfill({ json: [...world.profiles].sort((a, b) => row(a).name.localeCompare(row(b).name)).map(p => row(p)) })
      if (body.expected_revision) return bad(400, 'invalid expected revision')
      const id = `7e5a0000-0000-4000-8000-${String(++world.counter.next).padStart(12, '0')}`
      const stored: Stored = { id, revisions: [{ name: String(body.name).trim(), definition: body.definition as QuoteProfileDefinition }], archived: false }
      world.profiles.push(stored)
      return route.fulfill({ status: 201, json: row(stored) })
    }
    const one = /^\/api\/quote-profiles\/([^/]+)(\/undo)?$/.exec(path)
    const stored = one && world.profiles.find(p => p.id === one[1])
    if (!one || !stored) return bad(404, 'not found')
    if (one[2]) {
      if (body.expected_revision !== stored.revisions.length) return bad(409, 'profile revision is stale')
      if (stored.archived) stored.archived = false
      else if (stored.revisions.length === 1) { stored.archived = true; if (world.settings.default_profile_id === stored.id) { delete world.settings.default_profile_id; world.settings.revision++ } }
      else stored.revisions.push(structuredClone(stored.revisions[stored.revisions.length - 2]!))
      return route.fulfill({ json: row(stored) })
    }
    if (method === 'GET') {
      const revision = Number(url.searchParams.get('revision') ?? stored.revisions.length)
      return stored.revisions[revision - 1] ? route.fulfill({ json: row(stored, revision) }) : bad(404, 'not found')
    }
    if (method === 'DELETE') {
      if (stored.archived) return bad(404, 'not found')
      stored.archived = true
      if (world.settings.default_profile_id === stored.id) { delete world.settings.default_profile_id; world.settings.revision++ }
      return route.fulfill({ status: 204, body: '' })
    }
    if (method === 'PATCH') {
      if (stored.archived || body.expected_revision !== stored.revisions.length) return bad(409, 'profile revision is stale or archived')
      stored.revisions.push({ name: String(body.name).trim(), definition: body.definition as QuoteProfileDefinition })
      return route.fulfill({ json: row(stored) })
    }
    return bad(405, 'method not allowed')
  })
  return calls
}
