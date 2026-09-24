// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { AVATAR_PALETTE, avatarColor, avatarSources, centred, clampView, cropOf, deriveInitials, pan, sha256FirstByte, uploadError, uploadProblem, zoomAt } from '../src/lib/avatar.ts'

test('the initials colour follows the server: palette[sha256(id)[0] % 12]', () => {
  for (const id of ['11111111-1111-4111-8111-111111111111', '22222222-2222-4222-8222-222222222222', 'a', '', 'x'.repeat(200)]) {
    assert.equal(sha256FirstByte(id), createHash('sha256').update(id).digest()[0], id)
  }
  const id = '11111111-1111-4111-8111-111111111111'
  assert.equal(avatarColor(id), AVATAR_PALETTE[createHash('sha256').update(id).digest()[0] % 12])
})

test('initials derive like the server', () => {
  assert.equal(deriveInitials({ first_name: 'Markus', last_name: 'Barta' }), 'MB')
  assert.equal(deriveInitials({ first_name: '', preferred_name: 'mira', last_name: 'Holm' }), 'MH')
  assert.equal(deriveInitials({ short_name: 'mba' }), 'M')
  assert.equal(deriveInitials({ first_name: 'Élodie', last_name: '' }), 'É')
  assert.equal(deriveInitials({}), '?')
})

test('sources pick sizes sharp on 2x screens, versioned when the hash is known', () => {
  const s = avatarSources('p1', 40, { '64': 'aa', '128': 'bb' })
  assert.equal(s.src, '/api/people/p1/avatar/64?v=aa')
  assert.equal(s.srcset, '/api/people/p1/avatar/64?v=aa 1.6x, /api/people/p1/avatar/128?v=bb 3.2x, /api/people/p1/avatar/256 6.4x')
})

test('uploads are checked in plain words', () => {
  assert.match(uploadProblem({ type: 'image/heic', size: 10 }), /PNG, JPEG or WebP/)
  assert.match(uploadProblem({ type: 'image/png', size: 12 * 1024 * 1024 }), /12\.0 MB; a photo can be up to 8 MB/)
  assert.equal(uploadProblem({ type: 'image/jpeg', size: 1000 }), '')
  assert.match(uploadError(413, ''), /larger than 8 MB/)
  assert.match(uploadError(400, 'invalid image'), /could not be read/)
})

test('the crop always covers the viewport and maps to image pixels', () => {
  // A landscape 2000x1000 photo in a 300px viewport: covered at height.
  let v = centred(300, 2000, 1000)
  assert.deepEqual(cropOf(v), { x: 500, y: 0, size: 1000 })
  v = pan(v, 10_000, 10_000) // pinned to the top-left
  assert.deepEqual(cropOf(v), { x: 0, y: 0, size: 1000 })
  v = zoomAt(v, 2) // zooming about the centre keeps the centre
  assert.deepEqual(cropOf(v), { x: 250, y: 250, size: 500 })
  v = zoomAt(v, 9) // clamped to the maximum
  assert.equal(v.zoom, 5)
  v = clampView({ ...v, zoom: 0.2 })
  assert.equal(v.zoom, 1)
  const c = cropOf(pan(v, -10_000, -10_000))
  assert.deepEqual(c, { x: 1000, y: 0, size: 1000 })
})

test('zones and languages read well', async () => {
  const { matchZone, zoneParts, zoneOffset, datePreview, localeName } = await import('../src/lib/zones.ts')
  assert.deepEqual(zoneParts('America/Argentina/Salta'), { city: 'Salta', region: 'America / Argentina' })
  assert.deepEqual(zoneParts('UTC'), { city: 'UTC', region: '' })
  assert.ok(matchZone('America/New_York', 'new york'))
  assert.ok(!matchZone('Europe/Vienna', 'tokyo'))
  assert.equal(zoneOffset('Europe/Vienna', new Date('2026-09-24T12:00:00Z')), 'GMT+2')
  assert.equal(zoneOffset('UTC', new Date('2026-09-24T12:00:00Z')), 'GMT')
  assert.match(datePreview('de-AT', 'Europe/Vienna', new Date('2026-09-24T12:00:00Z')), /Donnerstag, 24\. September 2026 · 24\.09\.26, 14:00/)
  assert.equal(localeName('de-AT').english, 'Austrian German')
})
