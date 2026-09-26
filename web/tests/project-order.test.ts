// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { ORDER_LIMIT, byRank, keepOrder, nearestCell, place, sameOrder } from '../src/lib/projectOrder.ts'

test('custom order: ranked projects first, new ones after them and equal among themselves', () => {
  const compare = byRank(new Map([['b', 0], ['a', 1]]))
  const sorted = [{ id: 'new' }, { id: 'a' }, { id: 'b' }].sort(compare).map(p => p.id)
  assert.deepEqual(sorted, ['b', 'a', 'new'])
  assert.equal(compare({ id: 'x' }, { id: 'y' }), 0)
})

test('place moves a card within its group and keeps other groups where they are', () => {
  const order = ['x', 'a', 'y', 'b', 'c']
  const group = ['a', 'b', 'c']
  assert.deepEqual(place(order, group, ['c'], 0), ['x', 'c', 'a', 'y', 'b'])
  assert.deepEqual(place(order, group, ['a'], 2), ['x', 'y', 'b', 'c', 'a'])
  assert.deepEqual(place(order, group, ['a'], 9), ['x', 'y', 'b', 'c', 'a'])
  assert.deepEqual(place(order, group, ['b'], 1), order)
  // A selection moves as one block, in its order.
  assert.deepEqual(place(['a', 'b', 'c', 'd'], ['a', 'b', 'c', 'd'], ['c', 'a'], 2), ['b', 'd', 'a', 'c'])
  // The group as shown may come in any order: the full order decides (b, a shown; m goes after a).
  assert.deepEqual(place(['b', 'a', 'm'], ['a', 'b', 'm'], ['m'], 2), ['b', 'a', 'm'])
  // Nothing to place among: unchanged.
  assert.deepEqual(place(['a'], ['a'], ['a'], 0), ['a'])
})

test('sameOrder and nearestCell', () => {
  assert.equal(sameOrder(['a', 'b'], ['a', 'b']), true)
  assert.equal(sameOrder(['a', 'b'], ['b', 'a']), false)
  const cells = [
    { left: 0, top: 0, right: 100, bottom: 100 }, { left: 116, top: 0, right: 216, bottom: 100 },
    { left: 0, top: 116, right: 100, bottom: 216 },
  ]
  assert.equal(nearestCell(cells, 150, 50), 1)
  assert.equal(nearestCell(cells, 50, 300), 2)
  assert.equal(nearestCell(cells, 300, 60), 1)
  assert.equal(nearestCell([], 0, 0), -1)
})

test('keepOrder saves visible projects once, and stays well under the 16 KiB preference limit', () => {
  assert.deepEqual(keepOrder(['a', 'gone', 'b', 'a'], new Set(['a', 'b'])), ['a', 'b'])
  const ids = Array.from({ length: 600 }, (_, i) => `00000000-0000-4000-8000-${String(i).padStart(12, '0')}`)
  const archived = new Set(ids.slice(0, 500))
  const kept = keepOrder(ids, new Set(ids), archived)
  assert.equal(kept.length, ORDER_LIMIT)
  // Live projects stay; archived ones fill the rest in order.
  assert.deepEqual(kept.slice(0, 140), ids.slice(0, 140))
  assert.ok(ids.slice(500).every(id => kept.includes(id)))
  assert.ok(new TextEncoder().encode(JSON.stringify({ ids: kept })).length < 10 * 1024)
  assert.deepEqual(keepOrder(ids.slice(0, 3), new Set(ids), new Set(), 2), ids.slice(0, 2))
})
