// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { choiceById, linkBody, linkedSentence, relationLabel, RELATION_CHOICES, unlinkedSentence } from '../src/lib/relations.ts'

test('a relation reads from the open ticket: outgoing, incoming, and relates both ways', () => {
  assert.equal(relationLabel({ type: 'blocks', source_node_id: 'a' }, 'a'), 'Blocks')
  assert.equal(relationLabel({ type: 'blocks', source_node_id: 'b' }, 'a'), 'Blocked by')
  assert.equal(relationLabel({ type: 'duplicates', source_node_id: 'b' }, 'a'), 'Duplicated by')
  assert.equal(relationLabel({ type: 'implements', source_node_id: 'b' }, 'a'), 'Implemented by')
  assert.equal(relationLabel({ type: 'cites', source_node_id: 'a' }, 'a'), 'Cites')
  assert.equal(relationLabel({ type: 'relates', source_node_id: 'b' }, 'a'), 'Relates to')
  assert.equal(relationLabel({ type: 'relates', source_node_id: 'a' }, 'a'), 'Relates to')
})

test('every work type is offered once in each direction, relates once', () => {
  assert.deepEqual(RELATION_CHOICES.map(choice => choice.label), ['Blocked by', 'Blocks', 'Relates to', 'Duplicates', 'Duplicated by', 'Cites', 'Implements', 'Implemented by', 'Cited by'])
  assert.equal(new Set(RELATION_CHOICES.map(choice => `${choice.type}:${choice.outgoing}`)).size, RELATION_CHOICES.length)
  assert.equal(choiceById('nope').id, 'relates')
})

test('the request points the way the choice reads', () => {
  assert.deepEqual(linkBody(choiceById('blocks'), 'self', 'other'), { source_node_id: 'self', target_node_id: 'other', type: 'blocks' })
  assert.deepEqual(linkBody(choiceById('blocked-by'), 'self', 'other'), { source_node_id: 'other', target_node_id: 'self', type: 'blocks' })
  assert.deepEqual(linkBody(choiceById('implemented-by'), 'self', 'other'), { source_node_id: 'other', target_node_id: 'self', type: 'implements' })
})

test('toasts say what changed in words', () => {
  assert.equal(linkedSentence('blocks', 'A-1', 'A-2'), 'A-1 now blocks A-2')
  assert.equal(linkedSentence('relates', 'A-1', 'A-2'), 'A-1 now relates to A-2')
  assert.equal(unlinkedSentence('duplicates', 'A-1', 'A-2'), 'A-1 no longer duplicates A-2')
})
