// SPDX-License-Identifier: AGPL-3.0-only
import { toRaw } from 'vue'

// Quote documents and selections are JSON values. Unwrap every level before
// structuredClone: Vue can proxy nested arrays and objects independently.
export function cloneQuoteValue<T>(value: T): T {
  const plain = (input: unknown): unknown => {
    if (input === null || typeof input !== 'object') return input
    const raw = toRaw(input)
    if (Array.isArray(raw)) return raw.map(plain)
    return Object.fromEntries(Object.entries(raw).map(([key, item]) => [key, plain(item)]))
  }
  return structuredClone(plain(value)) as T
}
