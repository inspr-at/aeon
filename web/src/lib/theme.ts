// SPDX-License-Identifier: AGPL-3.0-only
import { ref } from 'vue'

// Light, Dark, or System (follow the OS). The choice lasts for the session; no device storage.
export type ThemeChoice = 'light' | 'dark' | 'system'
const preference = window.matchMedia('(prefers-color-scheme: dark)')
export const themeChoice = ref<ThemeChoice>('system')
export const dark = ref(preference.matches)
preference.addEventListener('change', (event) => {
  if (themeChoice.value === 'system') dark.value = event.matches
})
export function setTheme(choice: ThemeChoice) {
  themeChoice.value = choice
  if (choice === 'system') {
    delete document.documentElement.dataset.theme
    dark.value = preference.matches
  } else {
    document.documentElement.dataset.theme = choice
    dark.value = choice === 'dark'
  }
}
export function toggleTheme() { setTheme(dark.value ? 'light' : 'dark') }
