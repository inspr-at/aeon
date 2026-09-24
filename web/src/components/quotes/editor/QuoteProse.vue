<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { collectTextNodes, glyphTopAtLinear, lineBoundaryTarget, offsetInRoot, pointInTextNodes, type LineEdge } from '../../../lib/quotes/caret'
import type { QuoteEditor } from '../../../lib/quotes/editor'
import { boundary, deleteBackward, deleteForward, markRuns, markerLabels, nodesFor, reconcileInput, replaceText } from '../../../lib/quotes/prose'
import type { TextNode, TextPoint, TextSelection } from '../../../lib/quotes/types'
const props = defineProps<{ editor: QuoteEditor; sectionId: string; body: string; nodes: TextNode[]; sectionNumber: number; editable?: boolean }>()
const emit = defineEmits<{ select: [selection: TextSelection]; moveSection: [direction: number] }>()
const root = ref<HTMLElement>()
const shown = computed(() => nodesFor(props.body, props.nodes))
const markers = computed(() => markerLabels(shown.value, props.sectionNumber))
const composing = ref(false)
let pending: TextPoint | null = null
let visualEdge: { nodeId: string; edge: LineEdge } | null = null
const textRoot = (id: string) => [...(root.value?.querySelectorAll<HTMLElement>('[data-text-id]') ?? [])].find(el => el.dataset.textId === id)
function textPoint(node: Node | null, offset: number): TextPoint | null {
  const el = (node instanceof Element ? node : node?.parentElement)?.closest<HTMLElement>('[data-text-id]')
  if (!el || !root.value?.contains(el)) return null
  const value = el.textContent ?? ''
  const raw = offsetInRoot(el, node!, offset)
  const at = Math.max(0, Math.min(raw, value.length))
  return { nodeId: el.dataset.textId!, offset: boundary(value, at) ? at : at - 1 }
}
function read(): TextSelection | null {
  const live = window.getSelection()
  if (!live) return null
  const anchor = textPoint(live.anchorNode, live.anchorOffset), focus = textPoint(live.focusNode, live.focusOffset)
  return anchor && focus ? { sectionId: props.sectionId, anchor, focus } : null
}
function select(preserveTyping = false) {
  const selection = read()
  if (!selection) return
  props.editor.select({ sectionId: props.sectionId, text: selection }, preserveTyping)
  emit('select', selection)
  if (visualEdge && visualEdge.nodeId !== selection.focus.nodeId || visualEdge && visualEdge.edge.offset !== selection.focus.offset) visualEdge = null
}
function place(point: TextPoint) {
  const el = textRoot(point.nodeId), live = window.getSelection()
  if (!el || !live) return
  const texts = collectTextNodes(el), target = pointInTextNodes(texts, point.offset)
  if (target) live.collapse(target.node, target.offset)
  else live.collapse(el, 0)
}
watch(() => [props.body, props.nodes], () => { if (pending && !composing.value) { const at = pending; pending = null; void nextTick(() => { place(at); select(true) }) } }, { deep: true })
function commit(nodes: TextNode[], caret: TextPoint) {
  pending = caret
  props.editor.editSection(props.sectionId, { nodes })
  void nextTick(() => { if (pending) { const at = pending; pending = null; place(at); select(true) } })
}
function replace(inserted: string) {
  const selection = read()
  if (!selection) return
  const edit = replaceText(shown.value, selection.anchor, selection.focus, inserted, props.editor.typingBits ?? undefined)
  commit(edit.nodes, edit.caret)
}
function beforeInput(event: InputEvent) {
  if (!props.editable || composing.value || event.isComposing) return
  const selection = read()
  if (!selection) return
  if (event.inputType === 'insertText' || event.inputType === 'insertCompositionText') {
    if (event.data == null) return
    event.preventDefault(); replace(event.data)
  } else if (event.inputType === 'insertParagraph' || event.inputType === 'insertLineBreak') {
    event.preventDefault(); replace('\n')
  } else if (event.inputType === 'deleteContentBackward' || event.inputType === 'deleteContentForward') {
    event.preventDefault()
    if (selection.anchor.nodeId !== selection.focus.nodeId || selection.anchor.offset !== selection.focus.offset) replace('')
    else {
      const edit = event.inputType === 'deleteContentBackward' ? deleteBackward(shown.value, selection.focus) : deleteForward(shown.value, selection.focus)
      commit(edit.nodes, edit.caret)
    }
  }
}
function input() {
  if (composing.value) return
  const next = shown.value.map(node => {
    const value = textRoot(node.id)?.textContent ?? node.text
    return value === node.text ? node : reconcileInput(node, value, props.editor.typingBits ?? undefined)
  })
  if (next.some((node, index) => node !== shown.value[index])) {
    const focus = read()?.focus
    commit(next, focus ?? { nodeId: next[0]!.id, offset: 0 })
  }
}
function paste(event: ClipboardEvent) {
  if (!props.editable) return
  event.preventDefault()
  replace((event.clipboardData?.getData('text/plain') ?? '').replace(/\r\n?/g, '\n'))
}
function keydown(event: KeyboardEvent) {
  if (!props.editable || composing.value || event.isComposing) return
  if ((event.ctrlKey || event.metaKey) && ['b', 'i'].includes(event.key.toLowerCase())) {
    event.preventDefault(); select(); props.editor.setMarks(event.key.toLowerCase() === 'b' ? 'bold' : 'italic'); return
  }
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'z') { event.preventDefault(); event.shiftKey ? props.editor.redo() : props.editor.undo(); return }
  if (event.ctrlKey && event.key.toLowerCase() === 'y') { event.preventDefault(); props.editor.redo(); return }
  if (event.key === 'Tab') { event.preventDefault(); select(); event.shiftKey ? props.editor.outdent() : props.editor.indent(); return }
  if (event.altKey && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) { event.preventDefault(); emit('moveSection', event.key === 'ArrowUp' ? -1 : 1); return }
  if (event.key !== 'Home' && event.key !== 'End' || event.ctrlKey || event.metaKey || event.altKey) { visualEdge = null; return }
  const selection = read(), el = selection && textRoot(selection.focus.nodeId), live = window.getSelection()
  if (!selection || !el || !live) return
  const texts = collectTextNodes(el), value = el.textContent ?? ''
  const remembered = visualEdge?.nodeId === selection.focus.nodeId ? visualEdge.edge : null
  const result = lineBoundaryTarget(event.key, selection.focus.offset, value, remembered, i => glyphTopAtLinear(texts, i))
  const target = pointInTextNodes(texts, result.target)
  if (!target) return
  event.preventDefault()
  visualEdge = { nodeId: selection.focus.nodeId, edge: result.edge }
  if (event.shiftKey && live.anchorNode) live.setBaseAndExtent(live.anchorNode, live.anchorOffset, target.node, target.offset)
  else live.collapse(target.node, target.offset)
  select()
}
function compositionEnd() {
  composing.value = false
  input()
}
</script>
<template>
  <div ref="root" class="quote-prose" :contenteditable="editable ? 'true' : undefined" role="textbox" :aria-label="`Section ${sectionNumber} text`" aria-multiline="true" spellcheck="true" @focus="select()" @keyup="select()" @mouseup="select()" @beforeinput="beforeInput" @input="input" @paste="paste" @keydown="keydown" @compositionstart="composing = true" @compositionend="compositionEnd">
    <div v-for="(node, index) in shown" :key="node.id" class="quote-prose-row" :data-node-id="node.id" :class="{ item: node.kind === 'item' }" :style="node.kind === 'item' ? { paddingLeft: `${(node.depth ?? 0) * 1.35 + 1.5}em` } : undefined">
      <span v-if="node.kind === 'item'" class="quote-marker" contenteditable="false" aria-hidden="true" :style="{ transform: `translate(${node.marker_x_mm ?? '0'}mm, ${node.marker_y_mm ?? '0'}mm)` }">{{ markers[index] }}</span>
      <span class="quote-prose-text" :data-text-id="node.id" :style="node.text_start_mm ? { marginLeft: `${node.text_start_mm}mm` } : undefined"><template v-for="(run, part) in markRuns(node)" :key="part"><strong v-if="run.bold && run.italic"><em>{{ run.text }}</em></strong><strong v-else-if="run.bold">{{ run.text }}</strong><em v-else-if="run.italic">{{ run.text }}</em><template v-else>{{ run.text }}</template></template></span>
    </div>
  </div>
</template>
<style scoped>
.quote-prose { outline: none; min-height: 1em; overflow-wrap: anywhere; white-space: pre-wrap; font-synthesis: style; }
.quote-prose-row { position: relative; min-height: 1.45em; line-height: 1.45; }
.quote-prose-row.item { break-inside: avoid; }
.quote-marker { position: absolute; left: 0; min-width: 1.2em; white-space: nowrap; user-select: none; }
.quote-prose-text { display: inline; }
.quote-prose:focus-visible { outline: 1px solid var(--teal); outline-offset: 3px; }
</style>
