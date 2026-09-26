// SPDX-License-Identifier: AGPL-3.0-only
// Ticket keys named in text (release notes) resolved to tickets of this
// tenant: one lookup per batch of up to 100 unknown keys, remembered for the
// session. A key the server does not know stays unresolved, so it renders as
// plain text rather than a dead link.
import { reactive } from 'vue'
import { lookupNodeKeys } from './api.ts'

export interface TicketRef { key: string; id: string; title: string; state: string; projectId: string }

const BATCH = 100
// upper-cased key asked for → the ticket, or null when this tenant has none.
const known = reactive(new Map<string, TicketRef | null>())
const pending = new Map<string, Promise<void>>()

export const normalKey = (key: string) => key.trim().toUpperCase()

export function ticketRef(key: string): TicketRef | null | undefined {
  return known.get(normalKey(key))
}

export async function resolveTicketKeys(keys: string[]): Promise<void> {
  const wanted = [...new Set(keys.map(normalKey))].filter(key => key && !known.has(key))
  const waits = wanted.filter(key => pending.has(key)).map(key => pending.get(key)!)
  const asking = wanted.filter(key => !pending.has(key))
  for (let at = 0; at < asking.length; at += BATCH) {
    const batch = asking.slice(at, at + BATCH)
    const request = lookupNodeKeys(batch).then(page => {
      const found = new Map(page.items.filter(item => item.requested_key).map(item => [item.requested_key!, item]))
      for (const key of batch) {
        const item = found.get(key)
        known.set(key, item && item.project_id ? { key: item.key, id: item.id, title: item.title, state: item.state, projectId: item.project_id } : null)
      }
    }, () => { /* unresolved keys stay plain; a later call asks again */ }).finally(() => { for (const key of batch) pending.delete(key) })
    for (const key of batch) pending.set(key, request)
    waits.push(request)
  }
  await Promise.all(waits)
}

// Keys asked for while rendering (each link asks for its own) go out together
// on the next tick: a release's keys cost one request, not one per key.
let queued: string[] = []
export function wantTicketKey(key: string) {
  const wanted = normalKey(key)
  if (!wanted || known.has(wanted) || pending.has(wanted)) return
  if (!queued.length) queueMicrotask(() => { const keys = queued; queued = []; void resolveTicketKeys(keys) })
  queued.push(wanted)
}

// Tests start from a clean slate.
export function forgetTicketKeys() { known.clear(); pending.clear(); queued = [] }
