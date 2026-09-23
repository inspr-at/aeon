// SPDX-License-Identifier: AGPL-3.0-only
import { ref } from 'vue'

// Follow the OS until the user explicitly chooses a theme. No device storage.
const preference = window.matchMedia('(prefers-color-scheme: dark)')
export const dark = ref(preference.matches)
let chosen = false
preference.addEventListener('change', (event) => {
  if (!chosen) dark.value = event.matches
})
export function toggleTheme() {
  chosen = true
  dark.value = !dark.value
  document.documentElement.dataset.theme = dark.value ? 'dark' : 'light'
}
