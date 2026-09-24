<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { caretTopAtLinear, collectTextNodes, lineBoundaryTarget, offsetInRoot, pointInTextNodes, type LineEdge } from '../../../lib/quotes/caret'
const props = withDefaults(defineProps<{ modelValue: string; editable?: boolean; label: string; tag?: string; multiline?: boolean }>(), { editable: false, tag: 'span', multiline: true })
const emit = defineEmits<{ 'update:modelValue': [value: string]; focus: [] }>()
const root = ref<HTMLElement>()
const composing = ref(false)
let edge: LineEdge | null = null
watch(() => props.modelValue, value => { if (root.value && !composing.value && root.value.textContent !== value) root.value.textContent = value })
function input() { if (!composing.value) emit('update:modelValue', root.value?.textContent ?? '') }
function paste(event: ClipboardEvent) {
  if (!props.editable) return
  event.preventDefault()
  const plain = (event.clipboardData?.getData('text/plain') ?? '').replace(/\r\n?/g, '\n')
  const selection = window.getSelection()
  if (!selection?.rangeCount) return
  selection.deleteFromDocument()
  const text = document.createTextNode(props.multiline ? plain : plain.replace(/\n/g, ' '))
  selection.getRangeAt(0).insertNode(text)
  selection.collapse(text, text.length)
  input()
}
function keydown(event: KeyboardEvent) {
  if (!props.editable || composing.value || event.isComposing) return
  if (!props.multiline && event.key === 'Enter') { event.preventDefault(); return }
  if (event.key !== 'Home' && event.key !== 'End' || event.ctrlKey || event.metaKey || event.altKey) { edge = null; return }
  const el = root.value, selection = window.getSelection()
  if (!el || !selection?.focusNode || !el.contains(selection.focusNode)) return
  const texts = collectTextNodes(el)
  const value = el.textContent ?? ''
  const offset = offsetInRoot(el, selection.focusNode, selection.focusOffset)
  const result = lineBoundaryTarget(event.key, offset, value, edge, i => caretTopAtLinear(texts, i))
  const target = pointInTextNodes(texts, result.target)
  if (!target) return
  event.preventDefault()
  edge = result.edge
  if (event.shiftKey && selection.anchorNode) selection.setBaseAndExtent(selection.anchorNode, selection.anchorOffset, target.node, target.offset)
  else selection.collapse(target.node, target.offset)
  void nextTick()
}
</script>
<template>
  <component :is="tag" ref="root" class="quote-text" :contenteditable="editable ? 'true' : undefined" :role="editable ? 'textbox' : undefined" :aria-label="editable ? label : undefined" :aria-multiline="editable ? multiline : undefined" :data-placeholder="label" spellcheck="true" @focus="emit('focus')" @input="input" @paste="paste" @keydown="keydown" @compositionstart="composing = true" @compositionend="composing = false; input()">{{ modelValue }}</component>
</template>
<style scoped>
.quote-text { white-space: pre-wrap; overflow-wrap: anywhere; outline: none; min-width: 0; }
.quote-text[contenteditable="true"]:empty::before { content: attr(data-placeholder); color: var(--ink-3); opacity: .6; }
.quote-text[contenteditable="true"]:focus-visible { box-shadow: 0 2px 0 var(--teal); }
</style>
