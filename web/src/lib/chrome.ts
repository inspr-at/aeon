// SPDX-License-Identifier: AGPL-3.0-only
import { ref } from 'vue'

// The quote editor can fold the app header away to give the page more room (its
// chevron beside PDF). Only pages that say so in their route (meta.foldHeader)
// honour it, so the header is always back everywhere else.
export const headerFolded = ref(false)
