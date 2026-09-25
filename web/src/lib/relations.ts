// SPDX-License-Identifier: AGPL-3.0-only
// Relations as a ticket reads them: the server's types (api/openapi.yaml
// RelationCreate) seen from one end, so "X blocks this" is "Blocked by X".
// Free of Vue for unit tests.
import type { Relation, RelationType } from './api.ts'

export interface RelationChoice { id: string; type: RelationType; outgoing: boolean; label: string; hint: string }

// The work vocabulary, in the order the relation list shows its groups and the
// picker's three by three grid reads: waiting first, then sameness, then
// delivery. CRM links (customer_of, contact_for) belong to customers and
// contacts, not here.
export const RELATION_CHOICES: RelationChoice[] = [
  { id: 'blocked-by', type: 'blocks', outgoing: false, label: 'Blocked by', hint: 'waits for the other to finish' },
  { id: 'blocks', type: 'blocks', outgoing: true, label: 'Blocks', hint: 'the other waits for this' },
  { id: 'relates', type: 'relates', outgoing: true, label: 'Relates to', hint: 'connected, no order' },
  { id: 'duplicates', type: 'duplicates', outgoing: true, label: 'Duplicates', hint: 'this repeats the other' },
  { id: 'duplicated-by', type: 'duplicates', outgoing: false, label: 'Duplicated by', hint: 'the other repeats this' },
  { id: 'cites', type: 'cites', outgoing: true, label: 'Cites', hint: 'this refers to the other' },
  { id: 'implements', type: 'implements', outgoing: true, label: 'Implements', hint: 'this delivers the other' },
  { id: 'implemented-by', type: 'implements', outgoing: false, label: 'Implemented by', hint: 'the other delivers this' },
  { id: 'cited-by', type: 'cites', outgoing: false, label: 'Cited by', hint: 'the other refers to this' },
]
export const RELATION_ORDER = RELATION_CHOICES.map(choice => choice.label)

// The label of a relation seen from `selfId`; relates reads the same both ways.
export function relationLabel(relation: Pick<Relation, 'type' | 'source_node_id'>, selfId: string): string {
  const outgoing = relation.type === 'relates' || relation.source_node_id === selfId
  return RELATION_CHOICES.find(choice => choice.type === relation.type && choice.outgoing === outgoing)?.label ?? 'Relates to'
}
export function choiceById(id: string | null | undefined): RelationChoice {
  return RELATION_CHOICES.find(choice => choice.id === id) ?? RELATION_CHOICES[2]
}

// The request for linking `self` to `other` as the choice reads.
export function linkBody(choice: RelationChoice, selfId: string, otherId: string) {
  return choice.outgoing
    ? { source_node_id: selfId, target_node_id: otherId, type: choice.type }
    : { source_node_id: otherId, target_node_id: selfId, type: choice.type }
}

const PRESENT: Record<RelationType, string> = {
  blocks: 'blocks', relates: 'relates to', implements: 'implements', cites: 'cites', duplicates: 'duplicates',
  customer_of: 'is the customer of', contact_for: 'is a contact for',
}
const NEGATED: Record<RelationType, string> = {
  blocks: 'no longer blocks', relates: 'no longer relates to', implements: 'no longer implements', cites: 'no longer cites', duplicates: 'no longer duplicates',
  customer_of: 'is no longer the customer of', contact_for: 'is no longer a contact for',
}
// "ABC-1 now blocks ABC-2" and "ABC-1 no longer blocks ABC-2", with the
// source first so the sentence reads the way the link points.
export function linkedSentence(type: RelationType, sourceKey: string, targetKey: string) { return `${sourceKey} now ${PRESENT[type]} ${targetKey}` }
export function unlinkedSentence(type: RelationType, sourceKey: string, targetKey: string) { return `${sourceKey} ${NEGATED[type]} ${targetKey}` }
