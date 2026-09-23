<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import AppIcon from '../AppIcon.vue'
import MarkdownBody from '../MarkdownBody.vue'

// A plain Markdown textarea with a live preview toggle. Cmd/Ctrl+Enter saves,
// Escape cancels (asking first when there are changes).
const props = withDefaults(defineProps<{ modelValue: string; label: string; saving?: boolean; saveLabel?: string; placeholder?: string; minRows?: number; compact?: boolean }>(), { saveLabel: 'Save', placeholder: 'Write in Markdown…', minRows: 5 })
const emit = defineEmits<{ 'update:modelValue': [value: string]; save: []; cancel: [] }>()
const area = ref<HTMLTextAreaElement>()
const preview = ref(false)
const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)
const empty = computed(() => !props.modelValue.trim())

function grow() {
  const el = area.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(Math.max(el.scrollHeight + 2, props.minRows * 21 + 18), 560)}px`
}
function input(event: Event) { emit('update:modelValue', (event.target as HTMLTextAreaElement).value); grow() }
function keydown(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') { event.preventDefault(); if (!empty.value) emit('save') }
  else if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); emit('cancel') }
}
async function showWrite() { preview.value = false; await nextTick(); area.value?.focus(); grow() }
function focus() { area.value?.focus(); const length = area.value?.value.length ?? 0; area.value?.setSelectionRange(length, length) }
onMounted(async () => { await nextTick(); grow() })
defineExpose({ focus })
</script>

<template>
  <div class="md-editor" :class="{ compact }" @keydown="keydown">
    <div class="editor-head">
      <div class="seg" role="radiogroup" :aria-label="`${label} mode`">
        <button type="button" role="radio" :aria-checked="!preview" @click="showWrite">Write</button>
        <button type="button" role="radio" :aria-checked="preview" @click="preview = true">Preview</button>
      </div>
      <span class="md-hint"><AppIcon name="edit" :size="12" />Markdown</span>
    </div>
    <textarea
      v-show="!preview" ref="area" class="md-area" :value="modelValue" :aria-label="`${label}, Markdown`" :placeholder="placeholder" :disabled="saving" spellcheck="true" @input="input"
    />
    <div v-if="preview" class="md-preview" tabindex="0" :aria-label="`${label} preview`">
      <MarkdownBody v-if="!empty" :body="modelValue" />
      <p v-else class="nothing">Nothing to preview yet.</p>
    </div>
    <div class="editor-foot">
      <span class="keys"><kbd class="keycap">{{ mac ? '⌘' : 'Ctrl' }}</kbd><kbd class="keycap"><AppIcon name="enter" /></kbd> save · <kbd class="keycap">esc</kbd> cancel</span>
      <span class="spacer" />
      <button type="button" class="btn sm" :disabled="saving" @click="emit('cancel')">Cancel</button>
      <button type="button" class="btn sm on" :disabled="saving || empty" @click="emit('save')">{{ saving ? 'Saving…' : saveLabel }}</button>
    </div>
  </div>
</template>

<style scoped>
.md-editor { display: grid; gap: 8px; }
.editor-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.editor-head .seg button { height: 24px; padding: 0 10px; font-size: 12px; }
.md-hint { display: inline-flex; align-items: center; gap: 5px; font-size: 11.5px; color: var(--ink-3); }
.md-area {
  width: 100%; min-height: 90px; padding: 10px 12px; border: 1px solid var(--glass-edge); border-radius: var(--radius-s); resize: vertical;
  background: var(--field-bg); color: var(--ink); box-shadow: var(--field-inset), 0 0 0 1px var(--line);
  font: 13px/1.6 var(--mono); font-variant-ligatures: none; font-feature-settings: "liga" 0, "calt" 0;
}
.md-area:focus { box-shadow: var(--focus-ring); }
.md-preview { min-height: 90px; padding: 10px 12px; border-radius: var(--radius-s); background: var(--surface-sunken); box-shadow: inset 0 0 0 1px var(--line); }
.nothing { font-size: 13px; color: var(--ink-3); }
.editor-foot { display: flex; align-items: center; gap: 8px; }
.keys { display: inline-flex; align-items: center; gap: 3px; font-size: 11.5px; color: var(--ink-3); }
.spacer { flex: 1; }
.compact .md-area { min-height: 64px; }
@media (hover: none) { .keys { display: none; } }
</style>
