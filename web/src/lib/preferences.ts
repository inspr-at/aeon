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
// Whoever needs to know that a write failed listens here (a page can say so).
const failures = new Set<(key: string) => void>()
export function onPreferenceFailure(listener: (key: string) => void) { failures.add(listener); return () => { failures.delete(listener) } }

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
    if (!response.ok) failures.forEach(listener => listener(key))
    return response.ok
  } catch { failures.forEach(listener => listener(key)); return false }
}

// A reactive preference: loads once per key, then `save` updates it locally at once
// and on the server after `delay` ms of quiet.
export function usePreference<T extends object>(key: string) {
  let value = cache.get(key) as Ref<T | null> | undefined
  if (!value) { value = ref(null) as Ref<T | null>; cache.set(key, value as Ref<Json | null>) }
  const target = value
  if (!loads.has(key)) loads.set(key, readPreference(key).then(stored => { if (stored && target.value === null) target.value = stored as T }))
  const ready = loads.get(key)!
  function save(next: T, delay = 400) {
    target.value = next
    clearTimeout(timers.get(key))
    timers.set(key, setTimeout(() => { timers.delete(key); void writePreference(key, next as Json) }, delay))
  }
  return { value: target, ready, save }
}
