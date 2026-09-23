// SPDX-License-Identifier: AGPL-3.0-only
import { ref } from 'vue'

// Per-session display preferences. Kept in memory only (no device storage),
// like the theme choice: they survive navigation, not reloads.
export const density = ref<'comfortable' | 'compact'>('comfortable')
