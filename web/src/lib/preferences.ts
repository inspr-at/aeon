// SPDX-License-Identifier: AGPL-3.0-only
// Per-person UI preferences on the server (GET/PUT /api/preferences/{key}); no
// browser storage. Reads are cached for the session and writes are debounced, so
// dragging a column edge sends one request when the drag settles.
import { ref, type Ref } from 'vue'
import { api } from './api.ts'

type Json = Record<string, unknown>
const cache = new Map<string, Ref<Json | null>>()
const loads = new Map<string, Promise<void>>()
const timers = new Map<string, ReturnType<typeof setTimeout>>()

export async function readPreference(key: string): Promise<Json | null> {
  try {
    const response = await api(`/preferences/${key}`)
    if (!response.ok) return null
    const body = await response.json()
    return body && typeof body.value === 'object' && !Array.isArray(body.value) ? body.value : null
  } catch { return null }
}
export async function writePreference(key: string, value: Json): Promise<boolean> {
  try {
    const response = await api(`/preferences/${key}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ value }) })
    return response.ok
  } catch { return false }
}

// A reactive preference: loads once per key, then `save` updates it locally at once
// and on the server after `delay` ms of quiet. `delay` 0 still waits for this turn's
// saves to settle, then writes. That wait is a message, not a timer: a timer is
// stalled when the page clock is faked or the machine is loaded, so the screen can
// show the change while the write has not started.
const soon = new Set<string>()
function writeSoon(key: string, current: () => Json | null) {
  if (soon.has(key)) return
  soon.add(key)
  const channel = new MessageChannel()
  channel.port1.onmessage = () => {
    soon.delete(key)
    // A delayed save took over; it will write the latest value.
    if (timers.has(key)) return
    const value = current()
    if (value) void writePreference(key, value)
  }
  channel.port2.postMessage(undefined)
}
export function usePreference<T extends object>(key: string) {
  let value = cache.get(key) as Ref<T | null> | undefined
  if (!value) { value = ref(null) as Ref<T | null>; cache.set(key, value as Ref<Json | null>) }
  const target = value
  if (!loads.has(key)) loads.set(key, readPreference(key).then(stored => { if (stored && target.value === null) target.value = stored as T }))
  const ready = loads.get(key)!
  function save(next: T, delay = 400) {
    target.value = next
    const pending = timers.get(key)
    if (pending) clearTimeout(pending)
    timers.delete(key)
    if (delay <= 0) { writeSoon(key, () => target.value as Json | null); return }
    timers.set(key, setTimeout(() => { timers.delete(key); void writePreference(key, target.value as Json) }, delay))
  }
  return { value: target, ready, save }
}
