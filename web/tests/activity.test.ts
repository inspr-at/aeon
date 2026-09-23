// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildTimeline, canWrite, commentEditable, describeChange } from '../src/lib/activity.ts'
import type { ActivityItem } from '../src/lib/api.ts'

const mba = { id: 'p-1', name: 'mba' }, mira = { id: 'p-2', name: 'Mira' }
const at = (minutes: number) => new Date(Date.parse('2026-09-23T12:00:00Z') + minutes * 60_000).toISOString()
const change = (id: string, minutes: number, changes: ActivityItem['changes'], author = mba): ActivityItem => ({ id, at: at(minutes), type: 'change', author, changes })

test('the timeline runs oldest first and collapses one person’s changes within five minutes', () => {
  const items: ActivityItem[] = [
    change('5', 30, [{ field: 'status', from: 'qa', to: 'done' }]),
    { id: '4', at: at(29), type: 'comment', author: mba, body_markdown: 'Shipped.' },
    change('3', 3, [{ field: 'priority', from: 'medium', to: 'high' }]),
    change('2', 1, [{ field: 'status', from: 'backlog', to: 'in-progress' }]),
    change('1', 0, [{ field: 'status', from: 'new', to: 'backlog' }]),
    { id: '0', at: at(-60), type: 'created', author: mba },
  ]
  const timeline = buildTimeline(items)
  assert.deepEqual(timeline.map(entry => entry.kind), ['created', 'changes', 'comment', 'changes'])
  assert.deepEqual((timeline[1] as { changes: unknown }).changes, [
    { field: 'status', from: 'new', to: 'in-progress' },
    { field: 'priority', from: 'medium', to: 'high' },
  ])
})

test('changes that cancel out disappear; another author or a longer gap starts a new line', () => {
  assert.deepEqual(buildTimeline([
    change('2', 2, [{ field: 'priority', from: 'high', to: 'medium' }]),
    change('1', 0, [{ field: 'priority', from: 'medium', to: 'high' }]),
  ]), [])
  assert.equal(buildTimeline([
    change('2', 2, [{ field: 'status', from: 'backlog', to: 'qa' }], mira),
    change('1', 0, [{ field: 'status', from: 'new', to: 'backlog' }]),
  ]).length, 2)
  assert.equal(buildTimeline([
    change('2', 9, [{ field: 'status', from: 'backlog', to: 'qa' }]),
    change('1', 0, [{ field: 'status', from: 'new', to: 'backlog' }]),
  ]).length, 2)
})

test('changes read in product words', () => {
  assert.deepEqual(describeChange({ field: 'status', from: 'in-progress', to: 'qa' }), { label: 'changed status', from: 'In progress', to: 'QA' })
  assert.deepEqual(describeChange({ field: 'assignee', from: null, to: 'mba' }), { label: 'assigned it to', to: 'mba' })
  assert.deepEqual(describeChange({ field: 'priority', from: null, to: 'low' }), { label: 'changed priority', from: 'No priority', to: 'Low' })
  assert.equal(describeChange({ field: 'parent', from: 'a', to: 'b' }).label, 'moved it to another parent')
})

test('own comments stay editable for 15 minutes; viewers are read-only', () => {
  const now = Date.parse(at(14))
  assert.equal(commentEditable({ at: at(0), author: { id: 'p-1' } }, 'p-1', now), true)
  assert.equal(commentEditable({ at: at(0), author: { id: 'p-1' } }, 'p-1', Date.parse(at(15))), false)
  assert.equal(commentEditable({ at: at(0), author: { id: 'p-2' } }, 'p-1', now), false)
  assert.equal(canWrite(['member']), true)
  assert.equal(canWrite(['viewer']), false)
  assert.equal(canWrite(undefined), true)
})
