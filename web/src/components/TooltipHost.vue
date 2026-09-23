<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
// One tooltip for the whole app: any element with data-tip gets it on hover
// (after a short delay) and on keyboard focus. Screen readers use the element's
// own label; the tooltip is decoration for sighted pointer and keyboard users.
const text = ref('')
const x = ref(0)
const y = ref(0)
const below = ref(false)
const tip = ref<HTMLElement>()
let target: HTMLElement | null = null
let timer: ReturnType<typeof setTimeout> | undefined

function find(node: EventTarget | null): HTMLElement | null {
  return node instanceof Element ? node.closest<HTMLElement>('[data-tip]') : null
}
async function show(element: HTMLElement) {
  const value = element.dataset.tip
  if (!value) return
  target = element
  text.value = value
  await nextTick()
  const rect = element.getBoundingClientRect()
  const width = tip.value?.offsetWidth ?? 0
  const height = tip.value?.offsetHeight ?? 0
  below.value = rect.top - height - 8 < 8
  x.value = Math.round(Math.min(Math.max(8, rect.left + rect.width / 2 - width / 2), innerWidth - width - 8))
  y.value = Math.round(below.value ? rect.bottom + 8 : rect.top - height - 8)
}
function hide() { clearTimeout(timer); target = null; text.value = '' }
function over(event: PointerEvent) {
  if (event.pointerType === 'touch') return
  const element = find(event.target)
  if (element === target) return
  hide()
  if (element) timer = setTimeout(() => void show(element), 380)
}
function focusIn(event: FocusEvent) {
  const element = find(event.target)
  if (element && element.matches(':focus-visible')) void show(element)
}
onMounted(() => {
  document.addEventListener('pointerover', over)
  document.addEventListener('focusin', focusIn)
  document.addEventListener('focusout', hide)
  document.addEventListener('pointerdown', hide)
  document.addEventListener('keydown', hide)
  document.addEventListener('scroll', hide, true)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerover', over)
  document.removeEventListener('focusin', focusIn)
  document.removeEventListener('focusout', hide)
  document.removeEventListener('pointerdown', hide)
  document.removeEventListener('keydown', hide)
  document.removeEventListener('scroll', hide, true)
  clearTimeout(timer)
})
</script>

<template>
  <div v-if="text" ref="tip" class="tooltip" :class="{ below }" :style="{ transform: `translate(${x}px, ${y}px)` }" aria-hidden="true">{{ text }}</div>
</template>

<style scoped>
.tooltip {
  position: fixed; z-index: 80; top: 0; left: 0; max-width: 320px; padding: 5px 10px; border-radius: 8px; pointer-events: none;
  background: var(--tip-bg); color: var(--tip-ink); font-size: 12.5px; line-height: 1.4; white-space: pre-line;
  box-shadow: 0 0 0 1px var(--glass-rim), 0 10px 24px -10px rgba(0, 0, 0, .5);
}
</style>
