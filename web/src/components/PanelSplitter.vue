<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { onBeforeUnmount, ref, watchEffect } from 'vue'
import { usePreference } from '../lib/preferences'

// The handle between a list and its docked panel. Dragging sets the panel width
// for every docked panel (tickets and agent sessions); double-click or Home goes
// back to the default. The width is the person's preference on the server.
const pref = usePreference<{ panel?: number }>('layout')
const dragging = ref(false)
const MIN = 380
const root = document.documentElement
watchEffect(() => {
  const width = pref.value.value?.panel
  if (typeof width === 'number' && width >= MIN) root.style.setProperty('--panel-user-w', `${Math.round(width)}px`)
  else root.style.removeProperty('--panel-user-w')
})
function current() { return parseFloat(getComputedStyle(root).getPropertyValue('--panel-w')) || document.querySelector<HTMLElement>('.ticket-ws.panel, .session-panel')?.getBoundingClientRect().width || 560 }
function clamp(width: number) { return Math.round(Math.max(MIN, Math.min(window.innerWidth * 0.72, width))) }
function set(width: number, delay = 400) { pref.save({ ...(pref.value.value ?? {}), panel: clamp(width) }, delay) }
let lastPress = -Infinity
function start(event: PointerEvent) {
  if (event.button !== 0) return
  event.preventDefault()
  // A double press resets (dblclick is unreliable after pointerdown's preventDefault).
  if (event.timeStamp - lastPress < 350) { lastPress = -Infinity; reset(); return }
  lastPress = event.timeStamp
  dragging.value = true
  startX = event.clientX
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}
function move(event: PointerEvent) {
  if (!dragging.value) return
  // The panel sits 10px from the right edge.
  root.style.setProperty('--panel-user-w', `${clamp(window.innerWidth - event.clientX - 10)}px`)
}
let startX = 0
function end(event: PointerEvent) {
  if (!dragging.value) return
  dragging.value = false
  // A press without a drag changes nothing (so a double press can reset).
  if (Math.abs(event.clientX - startX) < 3) { const saved = pref.value.value?.panel; if (typeof saved === 'number') root.style.setProperty('--panel-user-w', `${saved}px`); else root.style.removeProperty('--panel-user-w'); return }
  set(window.innerWidth - event.clientX - 10)
}
function reset() {
  const { panel: _panel, ...rest } = pref.value.value ?? {}
  pref.save(rest, 0)
}
function keydown(event: KeyboardEvent) {
  const step = event.shiftKey ? 64 : 16
  if (event.key === 'ArrowLeft') { event.preventDefault(); set(current() + step, 300) }
  else if (event.key === 'ArrowRight') { event.preventDefault(); set(current() - step, 300) }
  else if (event.key === 'Home') { event.preventDefault(); reset() }
}
onBeforeUnmount(() => { dragging.value = false })
</script>

<template>
  <div
    class="splitter" :class="{ dragging }" role="separator" aria-orientation="vertical" tabindex="0" aria-label="Resize the panel"
    :aria-valuenow="Math.round(current())" :aria-valuemin="MIN" data-tip="Drag to resize · double-click to reset"
    @pointerdown="start" @pointermove="move" @pointerup="end" @pointercancel="end" @keydown="keydown"
  ><span class="grip" /></div>
</template>

<style scoped>
.splitter {
  position: fixed; z-index: 16; top: calc(var(--header-h) + 10px); bottom: 10px; right: calc(var(--panel-w) + 10px);
  width: 12px; margin-right: 1px; cursor: col-resize; touch-action: none; outline: none; display: grid; place-items: center;
}
.grip { width: 4px; height: 44px; border-radius: 999px; background: var(--line-2); opacity: 0; transition: opacity .15s ease, background .15s ease, height .15s ease; }
.splitter:hover .grip, .splitter:focus-visible .grip, .splitter.dragging .grip { opacity: 1; background: var(--teal); height: 64px; }
.splitter:focus-visible .grip { box-shadow: var(--focus-ring); }
@media (max-width: 1099px) { .splitter { display: none; } }
</style>
