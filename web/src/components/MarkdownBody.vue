<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import MarkdownIt from 'markdown-it'
const props = defineProps<{ body: string }>()
// Raw HTML stays text; markdown-it rejects script/data links. Images stay text
// so viewing another principal's Markdown never loads third-party resources.
const markdown = new MarkdownIt({ html: false, linkify: false }).disable('image')

// GitHub-style task lists: "- [ ] open" and "- [x] done" render as read-only checkboxes.
markdown.core.ruler.after('inline', 'task-lists', state => {
  const tokens = state.tokens
  for (let index = 2; index < tokens.length; index++) {
    const inline = tokens[index]
    if (inline.type !== 'inline' || tokens[index - 1].type !== 'paragraph_open' || tokens[index - 2].type !== 'list_item_open') continue
    const first = inline.children?.[0]
    const match = first?.type === 'text' ? /^\[([ xX])\]\s+/.exec(first.content) : null
    if (!first || !match) continue
    first.content = first.content.slice(match[0].length)
    const box = new state.Token('task_checkbox', '', 0)
    box.meta = { checked: match[1] !== ' ' }
    inline.children!.unshift(box)
    tokens[index - 2].attrJoin('class', 'task-list-item')
  }
})
markdown.renderer.rules.task_checkbox = (tokens, index) =>
  `<input class="task-box" type="checkbox" disabled${tokens[index].meta?.checked ? ' checked' : ''} aria-label="${tokens[index].meta?.checked ? 'Done' : 'Not done'}"> `

const rendered = computed(() => markdown.render(props.body))
</script>
<template><div class="markdown-body" v-html="rendered" /></template>
<style scoped>
.markdown-body { overflow-wrap: anywhere; font-size: 14px; line-height: 1.65; color: var(--ink); }
.markdown-body > :deep(:first-child) { margin-top: 0; }
.markdown-body :deep(p) { margin: 0 0 .85em; color: var(--ink); }
.markdown-body :deep(h1), .markdown-body :deep(h2), .markdown-body :deep(h3), .markdown-body :deep(h4) { font-family: var(--font); font-weight: 650; letter-spacing: -.01em; line-height: 1.3; margin: 1.4em 0 .5em; color: var(--ink); }
.markdown-body :deep(h1) { font-size: 1.3em; }
.markdown-body :deep(h2) { font-size: 1.15em; }
.markdown-body :deep(h3), .markdown-body :deep(h4) { font-size: 1em; }
.markdown-body :deep(ul), .markdown-body :deep(ol) { margin: 0 0 .85em; padding-left: 1.4em; }
.markdown-body :deep(li) { margin: .2em 0; }
.markdown-body :deep(li > p) { margin: 0; }
.markdown-body :deep(li::marker) { color: var(--ink-3); }
.markdown-body :deep(.task-list-item) { list-style: none; margin-left: -1.4em; display: block; }
.markdown-body :deep(.task-box) {
  appearance: none; display: inline-grid; place-items: center; vertical-align: -2px; width: 14px; height: 14px; margin: 0 6px 0 0; border-radius: 4px;
  background: var(--field-bg); box-shadow: inset 0 0 0 1.5px var(--line-2);
}
.markdown-body :deep(.task-box:checked) { background: var(--st-ok); box-shadow: none; }
.markdown-body :deep(.task-box:checked::after) { content: ''; width: 7px; height: 4px; border-left: 1.6px solid var(--surface); border-bottom: 1.6px solid var(--surface); transform: translateY(-1px) rotate(-45deg); }
.markdown-body :deep(pre) { overflow: auto; margin: 0 0 1em; padding: 12px 14px; border-radius: var(--radius-s); background: var(--surface-sunken); box-shadow: inset 0 0 0 1px var(--line); }
.markdown-body :deep(code) { font-family: var(--mono); font-size: .86em; font-variant-ligatures: none; font-feature-settings: "liga" 0, "calt" 0; }
.markdown-body :deep(:not(pre) > code) { padding: 1px 5px; border-radius: 5px; background: var(--surface-sunken); box-shadow: inset 0 0 0 1px var(--line); }
.markdown-body :deep(blockquote) { margin: 0 0 1em; border-left: 3px solid var(--aqua); padding: 2px 0 2px 14px; color: var(--ink-2); }
.markdown-body :deep(blockquote p) { color: var(--ink-2); }
.markdown-body :deep(hr) { height: 1px; margin: 1.4em 0; border: 0; background: linear-gradient(90deg, transparent, var(--line-2), transparent); }
.markdown-body :deep(table) { display: block; overflow: auto; margin: 0 0 1em; border-collapse: collapse; font-size: 13px; }
.markdown-body :deep(td), .markdown-body :deep(th) { padding: 6px 10px; border: 1px solid var(--line); text-align: left; }
.markdown-body :deep(th) { font: 500 10.5px var(--mono); letter-spacing: .1em; text-transform: uppercase; color: var(--ink-3); }
.markdown-body :deep(a) { color: var(--teal); text-decoration: underline; text-decoration-color: var(--gold); text-underline-offset: 3px; }
</style>
