<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { onBeforeUnmount, ref, watchEffect } from 'vue'
import { usePreference } from '../lib/preferences'

// The handle between a list and its docked panel. Dragging sets the panel width
// (by default for every docked panel: tickets and agent sessions); double-click or
// Home goes back to the default. The width is the person's preference on the
// server. A panel with its own width (the docked quote) names its preference
// field, its CSS variable, the panel element, and how much the list keeps.
const props = withDefaults(defineProps<{ field?: string; cssVar?: string; target?: string; reserve?: number }>(), { field: 'panel', cssVar: '--panel-user-w', target: '', reserve: 0 })
const pref = usePreference<Record<string, number | undefined>>('layout')
const dragging = ref(false)
const MIN = 380
const root = document.documentElement
watchEffect(() => {
  const width = pref.value.value?.[props.field]
  if (typeof width === 'number' && width >= MIN) root.style.setProperty(props.cssVar, `${Math.round(width)}px`)
  else root.style.removeProperty(props.cssVar)
})
function current() {
  const panel = props.target ? document.querySelector<HTMLElement>(props.target) : null
  if (panel) return panel.getBoundingClientRect().width
  return parseFloat(getComputedStyle(root).getPropertyValue('--panel-w')) || document.querySelector<HTMLElement>('.ticket-ws.panel, .session-panel, .quote-dock')?.getBoundingClientRect().width || 560
}
// Never narrower than MIN, never wider than 72 % of the window or than leaves the list its room.
function clamp(width: number) { return Math.round(Math.max(MIN, Math.min(window.innerWidth * 0.72, window.innerWidth - props.reserve, width))) }
function set(width: number, delay = 400) { pref.save({ ...(pref.value.value ?? {}), [props.field]: clamp(width) }, delay) }
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
  root.style.setProperty(props.cssVar, `${clamp(window.innerWidth - event.clientX - 10)}px`)
}
let startX = 0
function end(event: PointerEvent) {
  if (!dragging.value) return
  dragging.value = false
  // A press without a drag changes nothing (so a double press can reset).
  if (Math.abs(event.clientX - startX) < 3) { const saved = pref.value.value?.[props.field]; if (typeof saved === 'number') root.style.setProperty(props.cssVar, `${saved}px`); else root.style.removeProperty(props.cssVar); return }
  set(window.innerWidth - event.clientX - 10)
}
function reset() {
  const rest = { ...(pref.value.value ?? {}) }
  delete rest[props.field]
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
  position: fixed; z-index: 16; top: calc(var(--header-h) + 10px); bottom: calc(var(--footer-h) + 10px); right: calc(var(--panel-w) + 10px);
  width: 12px; margin-right: 1px; cursor: col-resize; touch-action: none; outline: none; display: grid; place-items: center;
}
.grip { width: 4px; height: 44px; border-radius: 999px; background: var(--line-2); opacity: 0; transition: opacity .15s ease, background .15s ease, height .15s ease; }
.splitter:hover .grip, .splitter:focus-visible .grip, .splitter.dragging .grip { opacity: 1; background: var(--teal); height: 64px; }
.splitter:focus-visible .grip { box-shadow: var(--focus-ring); }
@media (max-width: 1099px) { .splitter { display: none; } }
/* A panel that splits the window from 900px (the docked quote) shows its handle there too. */
@media (min-width: 900px) and (max-width: 1099px) { .splitter.early { display: grid; } }
</style>
