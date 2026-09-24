// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { automaticColumns, layoutWidths, moveColumn, orderOf, releaseLabel, tagList, TITLE_TARGET, visibleColumns, widthOf } from '../src/lib/columns.ts'
import { byPosition, positionBetween, positionOf, type Attachment } from '../src/lib/attachments.ts'

const ids = (width: number, options: Parameters<typeof visibleColumns>[1]) => visibleColumns(width, options).columns.map(c => c.id)

test('automatic columns follow the table width and the data', () => {
  assert.deepEqual(automaticColumns(700, { assigned: true }), ['key', 'title', 'status', 'priority'])
  assert.deepEqual(automaticColumns(800, { assigned: true }), ['key', 'title', 'status', 'priority', 'updated'])
  assert.deepEqual(automaticColumns(1200, { assigned: true }), ['key', 'title', 'status', 'priority', 'assignee', 'updated'])
  assert.deepEqual(automaticColumns(1200, {}), ['key', 'title', 'status', 'priority', 'updated'])
  // Wide tables show Assignee even when nobody is assigned yet.
  assert.deepEqual(automaticColumns(1600, {}), ['key', 'title', 'status', 'priority', 'assignee', 'epic', 'created', 'updated'])
  assert.deepEqual(automaticColumns(1600, { assigned: true, estimate: true, release: true, tags: true }), ['key', 'title', 'status', 'priority', 'assignee', 'epic', 'release', 'tags', 'estimate', 'created', 'updated'])
  // Wide extras step aside before Title gets cramped: 1500px cannot hold them all beside 420px of title.
  const present = { assigned: true, estimate: true, release: true, tags: true }
  assert.deepEqual(ids(1500, { phone: false, present }), ['key', 'title', 'status', 'priority', 'assignee', 'epic', 'created', 'updated'])
  assert.deepEqual(ids(1700, { phone: false, present }), ['key', 'title', 'status', 'priority', 'assignee', 'epic', 'release', 'tags', 'created', 'updated'])
  assert.deepEqual(ids(2400, { phone: false, present }), ['key', 'title', 'status', 'priority', 'assignee', 'epic', 'release', 'tags', 'estimate', 'created', 'updated'])
})

test('a saved choice fixes order and visibility; what cannot fit steps aside', () => {
  const prefs = { order: ['updated', 'status', 'epic'] as const, visible: ['updated', 'status', 'epic', 'estimate'] as const }
  const p = { order: [...prefs.order], visible: [...prefs.visible] }
  assert.deepEqual(ids(2000, { phone: false, prefs: p }), ['key', 'title', 'updated', 'status', 'epic', 'estimate'])
  // 118 key + 240 title + 104 + 138 + 220 + 96 = 916: estimate goes first, then epic.
  assert.deepEqual(ids(900, { phone: false, prefs: p }), ['key', 'title', 'updated', 'status', 'epic'])
  assert.deepEqual(ids(700, { phone: false, prefs: p }), ['key', 'title', 'updated', 'status'])
  assert.equal(visibleColumns(2000, { phone: false, prefs: p }).customised, true)
  assert.deepEqual(ids(390, { phone: true, prefs: p }), ['key', 'title', 'status', 'priority', 'updated'])
})

test('order keeps Key and Title first and appends unknown or missing columns', () => {
  assert.deepEqual(orderOf({ order: ['title', 'created', 'bogus' as never, 'created'] }).slice(0, 4), ['key', 'title', 'created', 'status'])
  assert.equal(orderOf(null).length, 11)
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

test('wide tables stop Title near 960px and give the spare width to the text columns', () => {
  const wide = ['key', 'title', 'status', 'priority', 'assignee', 'epic', 'created', 'updated'] as const
  const sum = (w: Partial<Record<string, number>>) => Object.values(w).reduce((a: number, b) => a + (b ?? 0), 0)
  // Narrow: nothing grows, Title takes the rest.
  assert.deepEqual(layoutWidths([...wide], 1400), { key: 118, status: 138, priority: 112, assignee: 156, epic: 220, created: 104, updated: 104 })
  // 2460px: Epic and Assignee grow (up to their maximum), Title keeps about its target.
  const at2460 = layoutWidths([...wide], 2460)
  assert.equal(at2460.status, 138)
  assert.equal(at2460.epic, 480)
  assert.equal(at2460.assignee, 320)
  const title = 2460 - sum(at2460)
  assert.ok(title >= TITLE_TARGET && title < TITLE_TARGET + 140, `title ${title}`)
  // Between: growth is proportional and Title sits at the target.
  const at2000 = layoutWidths([...wide], 2000)
  assert.ok(at2000.epic! > 220 && at2000.epic! < 480 && at2000.assignee! > 156 && at2000.assignee! < 320)
  assert.ok(Math.abs(2000 - sum(at2000) - TITLE_TARGET) <= 2)
  // A width the person gave wins: Epic stays, Title's own width becomes its target.
  const sized = layoutWidths([...wide], 2000, { widths: { epic: 200, title: 700 } })
  assert.equal(sized.epic, 200)
  assert.ok(Math.abs(2000 - sum(sized) - 700) <= 2 || sized.assignee === 320)
  // Live drag widths count as sized too.
  assert.equal(layoutWidths([...wide], 2000, null, { assignee: 180 }).assignee, 180)
})

test('release and tags read the classic fields', () => {
  assert.equal(releaseLabel({ release: { id: 1, label: ' v4.7.8 ' } }), 'v4.7.8')
  assert.equal(releaseLabel({ release: '1.10.0' }), '1.10.0')
  assert.equal(releaseLabel({ release: null }), '')
  assert.equal(releaseLabel(undefined), '')
  assert.deepEqual(tagList({ tags: [{ id: 16, name: 'CUSTOMERPORTAL', color: 'blue' }, 'hsb8', { name: ' ' }, 7] }), [{ name: 'CUSTOMERPORTAL', color: 'blue' }, { name: 'hsb8', color: '' }])
  assert.deepEqual(tagList({ tags: null }), [])
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
