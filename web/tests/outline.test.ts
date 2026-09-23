// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { childMap, epicStats, flattenOutline, missingAncestors, type OutlineSource } from '../src/lib/outline.ts'
import { compareRows } from '../src/lib/ticketList.ts'
import type { ListItem } from '../src/lib/api.ts'

function node(id: string, kind: string, parent: string, extra: Partial<ListItem> = {}): ListItem {
  return { id, key: id.toUpperCase(), kind_id: kind, title: id, body: '', fields: {}, state: 'backlog', parent_id: parent, position: '0', created_at: '', updated_at: '2026-09-20T00:00:00Z', kind_slug: kind, kind_label: kind, priority: null, assignee: null, parent: null, children_count: 0, project: null, ...extra }
}
const nodes = new Map([
  ['e1', node('e1', 'epic', 'p', { children_count: 2 })],
  ['t1', node('t1', 'ticket', 'e1')],
  ['t2', node('t2', 'ticket', 'e1', { children_count: 1 })],
  ['k1', node('k1', 'task', 't2')],
  ['l1', node('l1', 'ticket', 'p')],
])
const children: Record<string, string[]> = { e1: ['t1', 't2'], t2: ['k1'] }
function source(patch: Partial<OutlineSource> = {}): OutlineSource {
  return {
    rootId: 'p', epics: ['e1'], loose: ['l1'], looseHasMore: false, looseLoading: false,
    node: id => nodes.get(id), children: id => children[id] ? { ids: children[id], hasMore: false, loading: false } : null,
    hasChildren: id => !!children[id], expanded: () => true, dimmed: () => false, stats: () => null,
    noEpicCollapsed: false, createUnder: null, ...patch,
  }
}

test('the outline flattens epics, their children with guides, then the No epic group', () => {
  const entries = flattenOutline(source())
  assert.deepEqual(entries.map(entry => entry.type === 'row' ? `${entry.key}@${entry.tree.depth}` : entry.type), ['e1@0', 't1@1', 't2@1', 'k1@2', 'group', 'l1@0'])
  const rows = Object.fromEntries(entries.flatMap(entry => entry.type === 'row' ? [[entry.key, entry.tree]] : []))
  assert.equal(rows.t1.last, false)
  assert.equal(rows.t2.last, true)
  assert.deepEqual(rows.k1.guides, [false])
  assert.deepEqual(rows.t1.guides, [])
})

test('collapsed rows hide children; unloaded children show skeletons; create goes first', () => {
  assert.deepEqual(flattenOutline(source({ expanded: id => id !== 'e1' })).map(entry => entry.key), ['e1', 'no-epic', 'l1'])
  const loading = flattenOutline(source({ children: () => null }))
  assert.deepEqual(loading.slice(0, 3).map(entry => entry.type), ['row', 'skeleton', 'skeleton'])
  const creating = flattenOutline(source({ createUnder: 'e1', expanded: () => false }))
  assert.deepEqual(creating.slice(0, 2).map(entry => entry.type), ['row', 'create'])
  assert.deepEqual(flattenOutline(source({ noEpicCollapsed: true })).slice(-1).map(entry => entry.type), ['group'])
})

test('match sets group by parent, find missing ancestors and order siblings', () => {
  const rows = [node('a', 'ticket', 'e9', { updated_at: '2026-09-21T00:00:00Z' }), node('b', 'ticket', 'e9', { updated_at: '2026-09-22T00:00:00Z' }), node('c', 'ticket', 'p')]
  const map = childMap(rows, compareRows([{ field: 'updated_at', desc: true }]))
  assert.deepEqual(map.get('e9')!.map(row => row.id), ['b', 'a'])
  assert.deepEqual(missingAncestors(new Map(rows.map(row => [row.id, row])), 'p'), ['e9'])
  const byPriority = compareRows([{ field: 'priority', desc: false }])
  assert.deepEqual([node('x', 'ticket', 'p'), node('y', 'ticket', 'p', { priority: 'high' })].sort(byPriority).map(row => row.id), ['y', 'x'])
  const byKey = compareRows([{ field: 'key', desc: false }])
  assert.deepEqual([node('p-10', 'ticket', 'p', { key: 'P-10' }), node('p-9', 'ticket', 'p', { key: 'P-9' })].sort(byKey).map(row => row.key), ['P-9', 'P-10'])
})

test('epic progress leaves cancelled work out of scope', () => {
  const closed = ['done', 'cancelled'], done = ['done']
  assert.deepEqual(epicStats({ done: 7, cancelled: 2, backlog: 3 }, s => closed.includes(s), s => done.includes(s), s => s === 'cancelled'), { done: 7, scope: 10, total: 12, open: 3 })
})
