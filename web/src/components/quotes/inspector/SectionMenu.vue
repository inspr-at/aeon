<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import FloatingPanel from '../../work/FloatingPanel.vue'
import QuoteIcon from './QuoteIcon.vue'

// A section's actions, opened from its handle, a right click on its heading, or
// its row in the outline. Delete sits last and quiet: it asks nothing, and the
// toast that follows offers Undo.
const props = defineProps<{ anchor: HTMLElement; index: number; count: number; label: string }>()
const emit = defineEmits<{ close: [restoreFocus: boolean]; add: []; up: []; down: []; remove: [] }>()
const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)
// The action runs while its section is still known, then the menu closes.
function pick(action: 'add' | 'up' | 'down' | 'remove') {
  if (action === 'add') emit('add')
  else if (action === 'up') emit('up')
  else if (action === 'down') emit('down')
  else emit('remove')
  emit('close', false)
}
function keys(event: KeyboardEvent) {
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  const items = [...(event.currentTarget as HTMLElement).querySelectorAll<HTMLButtonElement>('button:not(:disabled)')]
  const at = items.indexOf(document.activeElement as HTMLButtonElement)
  event.preventDefault(); event.stopPropagation()
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : Math.max(0, Math.min(items.length - 1, at + (event.key === 'ArrowDown' ? 1 : -1)))
  items[next]?.focus()
}
void props
</script>

<template>
  <FloatingPanel :anchor="anchor" :width="236" :label="label" @close="restore => emit('close', restore)">
    <div class="section-menu" role="menu" :aria-label="label" @keydown="keys">
      <button type="button" role="menuitem" class="menu-item" data-autofocus @click="pick('add')"><QuoteIcon name="section-add" :size="15" /><span>Add section below</span></button>
      <button type="button" role="menuitem" class="menu-item" :disabled="index === 0" @click="pick('up')"><QuoteIcon name="arrow-up" :size="15" /><span>Move up</span><kbd class="menu-keys">{{ mac ? '⌥' : 'Alt' }}↑</kbd></button>
      <button type="button" role="menuitem" class="menu-item" :disabled="index >= count - 1" @click="pick('down')"><QuoteIcon name="arrow-down" :size="15" /><span>Move down</span><kbd class="menu-keys">{{ mac ? '⌥' : 'Alt' }}↓</kbd></button>
      <div class="menu-sep" role="separator" />
      <button type="button" role="menuitem" class="menu-item quiet" @click="pick('remove')"><QuoteIcon name="trash" :size="15" /><span>Delete section</span></button>
    </div>
  </FloatingPanel>
</template>

<style scoped>
.section-menu { display: grid; gap: 1px; }
.menu-item { display: flex; align-items: center; gap: 10px; width: 100%; height: 34px; padding: 0 10px; border: 0; border-radius: 8px; background: transparent; color: var(--ink); font-size: 13.5px; text-align: left; }
.menu-item span { flex: 1; }
.menu-item svg { color: var(--ink-2); }
@media (hover: hover) { .menu-item:hover:not(:disabled) { background: var(--row-hover); } }
.menu-item:focus-visible { background: var(--row-selected); box-shadow: inset 0 0 0 1px var(--glass-rim); }
.menu-item:disabled { color: var(--ink-3); }
.menu-item:disabled svg { color: var(--ink-3); }
.menu-item.quiet { color: var(--ink-2); }
@media (hover: hover) { .menu-item.quiet:hover { color: var(--danger); background: var(--danger-bg); } .menu-item.quiet:hover svg { color: var(--danger); } }
.menu-keys { font: 500 11px/1 var(--mono); color: var(--ink-3); }
.menu-sep { height: 1px; margin: 4px 6px; background: var(--line); }
</style>
