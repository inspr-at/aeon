<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import MarkdownIt from 'markdown-it'
import { ATTACHMENT_REF, contentUrl } from '../lib/attachments'
const props = defineProps<{ body: string }>()
const emit = defineEmits<{ openAttachment: [id: string] }>()
// Raw HTML stays text; markdown-it rejects script/data links. Only this ticket's own
// attachments render as images (![caption](attachment:<id>)); any other image stays
// text, so viewing another principal's Markdown never loads third-party resources.
const markdown = new MarkdownIt({ html: false, linkify: false })
markdown.renderer.rules.image = (tokens, index) => {
  const token = tokens[index]
  const src = String(token.attrGet('src') ?? '')
  const alt = markdown.utils.escapeHtml(token.content || '')
  const match = ATTACHMENT_REF.exec(src)
  if (!match) return markdown.utils.escapeHtml(`![${token.content}](${src})`)
  const id = match[1]
  return `<button type="button" class="md-attachment" data-attachment="${id}" aria-label="Open ${alt || 'attachment'}"><img src="${contentUrl(id, 'preview')}" alt="${alt}" loading="lazy" decoding="async"></button>`
}

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
function click(event: MouseEvent) {
  const button = (event.target as HTMLElement).closest<HTMLElement>('.md-attachment')
  if (button?.dataset.attachment) { event.preventDefault(); emit('openAttachment', button.dataset.attachment) }
}
</script>
<template><div class="markdown-body" v-html="rendered" @click="click" /></template>
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
/* Task lists hang: the box sits in its own column, centred on the first line's
   x-height, and wrapped lines align with the text, not under the box. */
.markdown-body :deep(.task-list-item) { position: relative; display: block; list-style: none; margin-left: -1.4em; padding-left: 22px; }
.markdown-body :deep(.task-box) {
  appearance: none; position: absolute; left: 0; top: calc(.825em + .08em - 7px); display: grid; place-items: center; width: 14px; height: 14px; margin: 0; border-radius: 4px;
  background: var(--field-bg); box-shadow: inset 0 0 0 1.5px var(--line-2);
}
.markdown-body :deep(.task-box:checked) { background: var(--st-ok); box-shadow: none; }
.markdown-body :deep(.task-box:checked::after) { content: ''; width: 7px; height: 4px; border-left: 1.6px solid var(--surface); border-bottom: 1.6px solid var(--surface); transform: translateY(-1px) rotate(-45deg); }
.markdown-body :deep(pre) { overflow: auto; margin: 0 0 1em; padding: 12px 14px; border-radius: var(--radius-s); background: var(--surface-sunken); box-shadow: inset 0 0 0 1px var(--line); }
.markdown-body :deep(code) { font-family: var(--mono); font-size: .92em; font-variant-ligatures: none; font-feature-settings: "liga" 0, "calt" 0; }
/* Inline code is a quiet tint; only code blocks keep a frame. */
.markdown-body :deep(:not(pre) > code) { padding: .5px 4px; border-radius: 4px; background: var(--code-bg); color: var(--ink); }
.markdown-body :deep(pre code) { font-size: .88em; }
/* A quote reads as typography: indented on a quiet tint, no coloured rule (rule 11). */
.markdown-body :deep(blockquote) { margin: 0 0 1em 12px; padding: 8px 14px; border-radius: 10px; background: var(--surface-2); color: var(--ink-2); }
.markdown-body :deep(blockquote > :last-child) { margin-bottom: 0; }
.markdown-body :deep(blockquote p) { color: var(--ink-2); }
.markdown-body :deep(hr) { height: 1px; margin: 1.4em 0; border: 0; background: linear-gradient(90deg, transparent, var(--line-2), transparent); }
.markdown-body :deep(table) { display: block; overflow: auto; margin: 0 0 1em; border-collapse: collapse; font-size: 13px; }
.markdown-body :deep(td), .markdown-body :deep(th) { padding: 6px 10px; border: 1px solid var(--line); text-align: left; }
.markdown-body :deep(th) { font: 500 10.5px var(--mono); letter-spacing: .1em; text-transform: uppercase; color: var(--ink-3); }
.markdown-body :deep(a) { color: var(--teal); text-decoration: underline; text-decoration-color: var(--gold); text-underline-offset: 3px; }
.markdown-body :deep(.md-attachment) { display: block; max-width: 100%; margin: .4em 0 1em; padding: 0; border: 0; border-radius: 10px; overflow: hidden; background: var(--surface-sunken, var(--code-bg)); box-shadow: inset 0 0 0 1px var(--line), 0 10px 26px -18px rgba(16, 35, 39, .5); cursor: zoom-in; }
.markdown-body :deep(.md-attachment img) { display: block; max-width: 100%; height: auto; }
.markdown-body :deep(.md-attachment:focus-visible) { box-shadow: var(--focus-ring); }
</style>
