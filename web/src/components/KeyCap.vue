<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import AppIcon, { type IconName } from './AppIcon.vue'

// One key on a keycap. Symbol keys are drawn from the icon set, centred in the
// cap, never typed as text symbols; `mod` and `alt` follow the platform (Command
// and Option on a Mac, Ctrl and Alt elsewhere). Anything else is its own label.
const props = defineProps<{ k: string }>()
const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)
const SYMBOLS: Record<string, [IconName, string]> = {
  enter: ['enter', 'Enter'], backspace: ['backspace', 'Backspace'],
  up: ['arrow-up', 'Up arrow'], down: ['arrow-down', 'Down arrow'], left: ['arrow-left', 'Left arrow'], right: ['arrow', 'Right arrow'],
  minus: ['minus', 'Minus'], shift: ['shift', 'Shift'],
}
const key = computed<{ icon?: IconName; label: string }>(() => {
  if (props.k === 'mod') return mac ? { icon: 'command', label: 'Command' } : { label: 'Ctrl' }
  if (props.k === 'alt') return mac ? { icon: 'option', label: 'Option' } : { label: 'Alt' }
  const symbol = SYMBOLS[props.k]
  return symbol ? { icon: symbol[0], label: symbol[1] } : { label: props.k }
})
</script>

<template>
  <kbd class="keycap"><template v-if="key.icon"><AppIcon :name="key.icon" /><span class="sr-only">{{ key.label }}</span></template><template v-else>{{ key.label }}</template></kbd>
</template>
