// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { ORDER_BYTES, byRank, keepOrder, nearestCell, orderBytes, place, prunedOrder, sameOrder } from '../src/lib/projectOrder.ts'

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

const uuid = (i: number) => `00000000-0000-4000-8000-${String(i).padStart(12, '0')}`

test('keepOrder saves visible projects once and keeps every active place while the list fits', () => {
  assert.deepEqual(keepOrder(['a', 'gone', 'b', 'a'], new Set(['a', 'b'])), ['a', 'b'])
  // 400 active projects fit whole, well past the old 240, under the 16 KiB limit.
  const active = Array.from({ length: 400 }, (_, i) => uuid(i))
  assert.deepEqual(keepOrder(active, new Set(active)), active)
  assert.ok(orderBytes(active) <= ORDER_BYTES && orderBytes(active) < 16 * 1024)
})

test('over the byte budget, archived places go first (last ranked first), then the end of the list', () => {
  const ids = Array.from({ length: 430 }, (_, i) => uuid(i))
  const archived = new Set(ids.filter((_, i) => i % 10 === 0))
  const kept = keepOrder(ids, new Set(ids), archived)
  assert.ok(orderBytes(kept) <= ORDER_BYTES)
  // Every active project keeps its place; only archived ones left, the last ranked first.
  assert.ok(ids.filter(id => !archived.has(id)).every(id => kept.includes(id)))
  const gone = ids.filter(id => !kept.includes(id))
  assert.ok(gone.every(id => archived.has(id)))
  assert.deepEqual(gone, [...archived].slice(-gone.length))
  // With only active projects over budget, the end of the list gives way.
  const many = Array.from({ length: 500 }, (_, i) => uuid(i))
  const trimmed = keepOrder(many, new Set(many))
  assert.deepEqual(trimmed, many.slice(0, trimmed.length))
  assert.ok(orderBytes(trimmed) <= ORDER_BYTES && orderBytes([...trimmed, many[trimmed.length]!]) > ORDER_BYTES)
})

test('prunedOrder writes only when the visible projects change the saved order', () => {
  const saved = ['a', 'b', 'c']
  assert.equal(prunedOrder(saved, new Set(['a', 'b', 'c', 'new'])), null)
  // Archiving keeps the place while the list fits.
  assert.equal(prunedOrder(saved, new Set(['a', 'b', 'c']), new Set(['b'])), null)
  // Deleted or no longer shown (access changed): its place goes.
  assert.deepEqual(prunedOrder(saved, new Set(['a', 'c'])), ['a', 'c'])
  assert.deepEqual(prunedOrder(['a', 'a', 'b'], new Set(['a', 'b'])), ['a', 'b'])
})
