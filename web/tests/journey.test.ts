// SPDX-License-Identifier: AGPL-3.0-only
import test from 'node:test'
import assert from 'node:assert/strict'
import { featurePicks, featureState, orderedTickets, ticketGroups, putPlan, postAction, putProfile, agreeRequirements, acceptDraft, type WalkerTicket, type ReleaseWalker, type IntakeDraft } from '../src/lib/journey.ts'

const tickets: WalkerTicket[] = [
  { ticket_node_id:'a',key:'T-1',title:'First',included:true,position:0,feature_node_id:'feature' },
  { ticket_node_id:'b',key:'T-2',title:'Second',included:false,position:1,feature_node_id:'feature' },
]
test('A3 cycles partial to all to none to the remembered partial selection', () => {
  assert.equal(featureState(tickets),'some')
  const memory = tickets.filter(t => t.included).map(t => t.ticket_node_id)
  assert.deepEqual(featurePicks(tickets,memory),['a','b'])
  const all = tickets.map(t => ({ ...t,included:true }))
  assert.equal(featureState(all),'all'); assert.deepEqual(featurePicks(all,memory),[])
  const none = tickets.map(t => ({ ...t,included:false }))
  assert.equal(featureState(none),'none'); assert.deepEqual(featurePicks(none,memory),['a'])
  assert.deepEqual(featurePicks(none,['a','deleted']),['a'])
  assert.equal(featureState([]),'empty'); assert.deepEqual(featurePicks([],memory),[])
})
test('server order, empty features, and explicit ungrouped tickets survive grouping', () => {
  const walker: ReleaseWalker = { project_node_id:'p',release_node_id:'r',revision:9,state:'planning',features:[{feature_node_id:'feature',epic_key:'E-1',title:'Feature',selection:'some'},{feature_node_id:'empty',epic_key:'E-2',title:'Empty',selection:'empty'}],tickets:[tickets[1]!,{...tickets[0]!,position:2},{ticket_node_id:'c',key:'T-3',title:'Manual',included:false,position:0}] }
  assert.deepEqual(orderedTickets(walker).map(t=>t.ticket_node_id),['c','b','a'])
  const groups = ticketGroups(walker)
  assert.equal(groups[1]?.tickets.length,0); assert.equal(groups[2]?.feature,undefined)
  assert.deepEqual(groups[0]?.tickets.map(t=>t.ticket_node_id),['b','a'])
  assert.equal(walker.tickets[0]?.ticket_node_id,'b')
})
test('writes preserve exact contract revisions, IDs and idempotency; conflicts reject', async () => {
  const original = globalThis.fetch
  const calls: {path:string;init?:RequestInit}[] = []
  globalThis.fetch = (async (path,init) => { calls.push({path:String(path),init});return new Response('{}',{status:200}) }) as typeof fetch
  try {
    await putPlan('project','release',{expected_revision:7,ordered_ticket_ids:['b','a'],included_ticket_ids:['a']})
    assert.equal(calls[0]?.path,'/api/projects/project/releases/release/plan')
    assert.equal(calls[0]?.init?.method,'PUT')
    assert.deepEqual(JSON.parse(String(calls[0]?.init?.body)),{expected_revision:7,ordered_ticket_ids:['b','a'],included_ticket_ids:['a']})
    await putProfile('project','enterprise',12)
    assert.deepEqual(JSON.parse(String(calls[1]?.init?.body)),{profile:'enterprise',expected_revision:12})
    await postAction('project',{action:'start_build',expected_revision:12,idempotency_key:'retry-key',approval_request_id:'gate',release_id:'release'})
    assert.equal(JSON.parse(String(calls[2]?.init?.body)).idempotency_key,'retry-key')
    await agreeRequirements('project',{expected_revision:4,approval_request_id:'gate',idempotency_key:'agreement-key'})
    assert.equal(calls[3]?.path,'/api/projects/project/requirements/agree')
    await acceptDraft('project',{id:'draft',base_event_id:42} as IntakeDraft)
    assert.deepEqual(JSON.parse(String(calls[4]?.init?.body)),{expected_base_event_id:42})
    globalThis.fetch = (async () => new Response(JSON.stringify({error:'Stale revision'}),{status:409})) as typeof fetch
    await assert.rejects(()=>putProfile('project','personal',1),{status:409,message:'Stale revision'})
  } finally { globalThis.fetch=original }
})
