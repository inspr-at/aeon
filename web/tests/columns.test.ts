// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { automaticColumns, moveColumn, orderOf, visibleColumns, widthOf } from '../src/lib/columns.ts'
import { byPosition, positionBetween, positionOf, type Attachment } from '../src/lib/attachments.ts'

const ids = (width: number, options: Parameters<typeof visibleColumns>[1]) => visibleColumns(width, options).columns.map(c => c.id)

test('automatic columns follow the table width and the data', () => {
  assert.deepEqual(automaticColumns(700, true), ['key', 'title', 'status', 'priority'])
  assert.deepEqual(automaticColumns(800, true), ['key', 'title', 'status', 'priority', 'updated'])
  assert.deepEqual(automaticColumns(1200, true), ['key', 'title', 'status', 'priority', 'assignee', 'updated'])
  assert.deepEqual(automaticColumns(1200, false), ['key', 'title', 'status', 'priority', 'updated'])
  assert.deepEqual(automaticColumns(1600, false), ['key', 'title', 'status', 'priority', 'epic', 'created', 'updated'])
  assert.deepEqual(automaticColumns(1600, true, true), ['key', 'title', 'status', 'priority', 'assignee', 'epic', 'estimate', 'created', 'updated'])
})

test('a saved choice fixes order and visibility; what cannot fit steps aside', () => {
  const prefs = { order: ['updated', 'status', 'epic'] as const, visible: ['updated', 'status', 'epic', 'estimate'] as const }
  const p = { order: [...prefs.order], visible: [...prefs.visible] }
  assert.deepEqual(ids(2000, { phone: false, anyAssigned: false, prefs: p }), ['key', 'title', 'updated', 'status', 'epic', 'estimate'])
  // 118 key + 240 title + 104 + 138 + 220 + 96 = 916: estimate goes first, then epic.
  assert.deepEqual(ids(900, { phone: false, anyAssigned: false, prefs: p }), ['key', 'title', 'updated', 'status', 'epic'])
  assert.deepEqual(ids(700, { phone: false, anyAssigned: false, prefs: p }), ['key', 'title', 'updated', 'status'])
  assert.equal(visibleColumns(2000, { phone: false, anyAssigned: false, prefs: p }).customised, true)
  assert.deepEqual(ids(390, { phone: true, anyAssigned: true, prefs: p }), ['key', 'title', 'status', 'priority', 'updated'])
})

test('order keeps Key and Title first and appends unknown or missing columns', () => {
  assert.deepEqual(orderOf({ order: ['title', 'created', 'bogus' as never, 'created'] }).slice(0, 4), ['key', 'title', 'created', 'status'])
  assert.equal(orderOf(null).length, 9)
  const order = orderOf(null)
  assert.deepEqual(moveColumn(order, 'priority', -1).slice(0, 4), ['key', 'title', 'priority', 'status'])
  assert.deepEqual(moveColumn(order, 'status', -1), order)
  assert.deepEqual(moveColumn(order, 'key', 1), order)
})

test('widths clamp to each column’s bounds', () => {
  assert.equal(widthOf('status', null), 138)
  assert.equal(widthOf('status', { widths: { status: 20 } }), 84)
  assert.equal(widthOf('status', { widths: { status: 9999 } }), 260)
  assert.equal(widthOf('status', { widths: { status: Number.NaN } }), 138)
})

test('attachment positions are decimal strings the server accepts', () => {
  assert.equal(positionBetween(undefined, undefined), '1024')
  assert.equal(positionBetween(1, 2), '1.5')
  assert.equal(positionBetween(undefined, 1), '-1023')
  assert.equal(positionBetween(3, undefined), '1027')
  assert.match(positionBetween(1, 1.000000000001), /^-?\d{1,14}(\.\d{1,15})?$/)
  const a = (id: string, position: string) => ({ id, position }) as Attachment
  assert.deepEqual([a('b', '2.000000000000000'), a('a', '1.5'), a('c', '1.5')].sort(byPosition).map(x => x.id), ['a', 'c', 'b'])
  assert.equal(positionOf(a('x', 'nope')), 0)
})
