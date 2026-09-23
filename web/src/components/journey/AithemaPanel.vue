<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref, nextTick } from 'vue'
import type { Intake, IntakeDraft } from '../../lib/journey'
import MarkdownBody from '../MarkdownBody.vue'
defineProps<{ intake: Intake; busy: boolean; canEdit: boolean }>()
defineEmits<{ accept: [draft: IntakeDraft] }>()
const query = ref(''), expanded = ref<string[]>([])
const normalize = computed(() => query.value.toLocaleLowerCase())
function toggle(id: string) { expanded.value = expanded.value.includes(id) ? expanded.value.filter(x => x !== id) : [...expanded.value, id] }
async function reveal(source: string, turn?: string) {
  query.value = ''
  if (!expanded.value.includes(source)) expanded.value.push(source)
  await nextTick()
  const target = document.getElementById(turn ? `turn-${turn}` : `source-${source}`)
  target?.scrollIntoView({ block: 'nearest' }); target?.focus({ preventScroll: true })
}
// AIT-34's conversation component is supplied by the coordinator through this
// slot (JourneyView -> InspirePanel -> AithemaPanel), with project-id/refresh.
</script>
<template>
  <div class="j-stack">
    <section class="j-card" data-aithema-conversation tabindex="-1"><div class="eyebrow">Aithema · conversation</div><slot><p>The conversation appears here when Aithema is connected. Saved sources and transcripts remain available below.</p></slot></section>
    <section class="j-card"><div class="j-card-head"><h2>Sources</h2><span class="j-meta">{{ intake.sources.length }} stored</span></div>
      <p v-if="!intake.sources.length">No sources yet. Start with a conversation, a note, a link or a file.</p>
      <label v-if="intake.turns.length" class="j-label">Search transcript<input v-model="query" type="search" /></label>
      <article v-for="source in intake.sources" :key="source.id" :id="`source-${source.id}`" tabindex="-1" class="source">
        <div class="j-card-head"><div><b>{{ source.label }}</b><p class="j-meta">{{ source.kind }} · {{ new Date(source.created_at).toLocaleDateString() }}</p></div><button v-if="intake.turns.some(t => t.source_id === source.id)" class="j-button" :aria-expanded="expanded.includes(source.id)" @click="toggle(source.id)">Transcript</button></div>
        <p v-if="source.locator" class="j-note">{{ source.locator }}</p>
        <ol v-if="expanded.includes(source.id)" class="transcript"><li v-for="turn in intake.turns.filter(t => t.source_id === source.id && t.body.toLocaleLowerCase().includes(normalize)).sort((a,b) => a.ordinal-b.ordinal)" :key="turn.id" :id="`turn-${turn.id}`" tabindex="-1"><span class="j-meta">{{ turn.ordinal + 1 }} · {{ turn.speaker }}</span><MarkdownBody :body="turn.body" /></li></ol>
      </article>
    </section>
    <section v-for="draft in intake.drafts" :key="draft.id" class="j-card"><div class="j-card-head"><div><div class="eyebrow">Cited {{ draft.kind }} · {{ draft.status }}</div><h2>{{ draft.title }}</h2></div><button v-if="draft.status === 'proposed' && canEdit" class="j-button" :disabled="busy" @click="$emit('accept', draft)">Accept draft</button></div><MarkdownBody :body="draft.body" /><ul class="citations"><li v-for="cite in draft.citations" :key="`${cite.source_id}:${cite.locator}`"><button class="j-text" @click="reveal(cite.source_id,cite.turn_id)">{{ intake.sources.find(s => s.id === cite.source_id)?.label || 'Source unavailable' }} · {{ cite.locator }}</button></li></ul></section>
  </div>
</template>
<style scoped>
.source { border-top:1px solid var(--line); padding:12px 0; }.transcript { padding-left:20px; }.transcript li { padding:8px 0; border-bottom:1px solid var(--line); }.citations { font:11px/1.8 var(--mono); color:var(--ink-2); padding-left:18px; }
</style>
