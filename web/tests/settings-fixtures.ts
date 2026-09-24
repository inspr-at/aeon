// SPDX-License-Identifier: AGPL-3.0-only
// The server side of Settings for UI tests: the personal profile (greeting),
// agent keys and quote settings. Registered after the work and business mocks,
// it answers only its own routes and lets the rest fall through.
import type { Page } from '@playwright/test'

export interface SettingsMockOptions { greeting?: boolean; failPatch?: boolean; noQuotes?: boolean; noKeys?: boolean }
export function settingsData(options: SettingsMockOptions = {}) {
  return {
    profile: {
      principal_id: '11111111-1111-4111-8111-111111111111', email: 'markus@barta.com', first_name: 'Markus', last_name: 'Barta', preferred_name: '', short_name: 'mba',
      initials: 'MB', timezone: 'Europe/Vienna', locale: 'de-AT', greeting_enabled: options.greeting ?? true, avatar_color: 'teal', avatar_hashes: {}, week_start: 1, revision: 3,
    },
    keys: options.noKeys ? [] : [
      { id: 'k1', principal_id: 'a1', name: 'aeon-coordinator', prefix: 'c0or', scopes: [], created_at: '2026-09-20T08:00:00Z', expires_at: null, last_used_at: '2026-09-24T07:40:00Z', revoked_at: null },
      { id: 'k2', principal_id: 'a2', name: 'pharos-deployer', prefix: 'ph4r', scopes: ['nodes:read', 'journey:write'], created_at: '2026-09-12T08:00:00Z', expires_at: '2026-12-31T00:00:00Z', last_used_at: null, revoked_at: null },
      { id: 'k3', principal_id: 'a3', name: 'old-importer', prefix: 'imp0', scopes: ['imports:write'], created_at: '2026-08-01T08:00:00Z', expires_at: null, last_used_at: '2026-08-02T08:00:00Z', revoked_at: '2026-08-03T08:00:00Z' },
    ],
    quotes: { revision: 2, numbering_time_zone: 'Europe/Vienna', default_currency: 'EUR', sender: { company: 'INSPR Studio', city: 'Graz', country: 'AT', iban: 'AT00 0000 0000 0000 0000' }, defaults: {}, layout: {}, smtp_confirmation_enabled: true, smtp_configured: true },
    patches: [] as Record<string, unknown>[],
  }
}
export type SettingsData = ReturnType<typeof settingsData>

export async function mockSettings(page: Page, data: SettingsData, options: SettingsMockOptions = {}) {
  await page.route('**/api/**', async route => {
    const request = route.request(), path = new URL(request.url()).pathname, method = request.method()
    if (path === '/api/me/profile') {
      if (method === 'PATCH') {
        const body = request.postDataJSON() as Record<string, unknown>
        data.patches.push(body)
        if (options.failPatch) return route.fulfill({ status: 500, json: { error: 'database operation failed' } })
        Object.assign(data.profile, body, { revision: data.profile.revision + 1 })
      }
      return route.fulfill({ json: data.profile })
    }
    if (path === '/api/agent-keys' && method === 'GET') return route.fulfill({ json: { keys: data.keys } })
    if (path === '/api/quotes/settings' && method === 'GET') {
      if (options.noQuotes) return route.fulfill({ status: 403, json: { code: 'forbidden', message: 'quotes plugin is not enabled' } })
      return route.fulfill({ json: data.quotes })
    }
    return route.fallback()
  })
}
