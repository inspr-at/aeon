// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { byRank, nearestCell, place, sameOrder } from '../src/lib/projectOrder.ts'

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
