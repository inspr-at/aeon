// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { keyState, sectionOf, senderLine, settingsLink, visibleSections, type AgentKey } from '../src/lib/settings.ts'

test('Personal is for everyone; the other sections are for admins', () => {
  assert.deepEqual(visibleSections(false).map(s => s.id), ['personal'])
  assert.deepEqual(visibleSections(true).map(s => s.label), ['Personal', 'Workspace', 'Business', 'Projects'])
  assert.equal(sectionOf('business'), 'business')
  assert.equal(sectionOf(undefined), 'personal')
  assert.equal(sectionOf('nope'), 'personal')
  assert.equal(settingsLink('business', 'quotes'), '/settings/business#quotes')
})

test('agent keys read as active, expired or revoked', () => {
  const key = (extra: Partial<AgentKey>): AgentKey => ({ id: '1', principal_id: 'p', name: 'k', prefix: 'ab12', scopes: [], created_at: '2026-09-01T00:00:00Z', expires_at: null, last_used_at: null, revoked_at: null, ...extra })
  const now = Date.parse('2026-09-24T12:00:00Z')
  assert.equal(keyState(key({}), now), 'active')
  assert.equal(keyState(key({ expires_at: '2026-09-20T00:00:00Z' }), now), 'expired')
  assert.equal(keyState(key({ expires_at: '2026-10-20T00:00:00Z' }), now), 'active')
  assert.equal(keyState(key({ revoked_at: '2026-09-02T00:00:00Z', expires_at: '2026-09-20T00:00:00Z' }), now), 'revoked')
})

test('the quote sender reads as one line of what is set', () => {
  assert.equal(senderLine({ company: 'INSPR Studio', city: 'Graz', country: 'AT', iban: 'x' }), 'INSPR Studio · Graz · AT')
  assert.equal(senderLine({ company: ' ', city: 'Graz' }), 'Graz')
  assert.equal(senderLine(null), '')
})
