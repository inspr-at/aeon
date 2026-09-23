<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import MarkdownIt from 'markdown-it'
const props = defineProps<{ body: string }>()
// Raw HTML stays text; markdown-it rejects script/data links. Images stay text
// so viewing another principal's Markdown never loads third-party resources.
const markdown = new MarkdownIt({ html: false, linkify: false }).disable('image')
const rendered = computed(() => markdown.render(props.body))
</script>
<template><div class="markdown-body" v-html="rendered" /></template>
<style scoped>
.markdown-body { overflow-wrap: anywhere; line-height: 1.7; }
.markdown-body :deep(p) { margin: 0 0 1em; color: var(--ink); }
.markdown-body :deep(h1), .markdown-body :deep(h2), .markdown-body :deep(h3) { font-size: 1.35em; font-weight: 600; margin: 1em 0 .5em; }
.markdown-body :deep(pre) { overflow: auto; padding: 16px; background: var(--surface-2); border-radius: var(--radius-s); }
.markdown-body :deep(code) { font-family: var(--mono); font-size: .85em; }
.markdown-body :deep(blockquote) { margin: 1em 0; border-left: 3px solid var(--aqua); padding-left: 16px; }
.markdown-body :deep(table) { display: block; overflow: auto; border-collapse: collapse; }
.markdown-body :deep(td), .markdown-body :deep(th) { padding: 8px; border: 1px solid var(--line); }
.markdown-body :deep(a) { text-decoration: underline; }
</style>
