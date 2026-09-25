// SPDX-License-Identifier: AGPL-3.0-only
import { ref } from 'vue'
import { usePreference } from './preferences'

// Row height is the person's own choice and follows them to every device
// (GET/PUT /api/preferences/list:display); until it loads, lists are comfortable.
export type Density = 'comfortable' | 'compact'
export const density = ref<Density>('comfortable')
let loading: Promise<void> | null = null
export function useDensity() {
  const pref = usePreference<{ density?: Density }>('list:display')
  loading ??= pref.ready.then(() => {
    const saved = pref.value.value?.density
    if (saved === 'compact' || saved === 'comfortable') density.value = saved
  })
  function set(value: Density) {
    density.value = value
    pref.save({ ...(pref.value.value ?? {}), density: value }, 0)
  }
  return { density, set }
}
