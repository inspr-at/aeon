<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import { collaboratorColor, type PresenceSnapshot } from '../../../lib/quotePresence'
import Avatar from '../../Avatar.vue'
import FloatingPanel from '../../work/FloatingPanel.vue'

// Who else has this quote open: their avatars ringed in their presence colour
// (the colour of their caret on the page), editing ones first. A click lists
// everyone with what they are doing. Nobody else here shows nothing at all.
const props = defineProps<{ presence: PresenceSnapshot | null; principalId: string; compact?: boolean }>()
interface Person { id: string; name: string; mode: 'viewing' | 'editing' | 'idle'; tabs: number }
const RANK = { editing: 0, viewing: 1, idle: 2 } as const
const people = computed<Person[]>(() => {
  const map = new Map<string, Person>()
  for (const s of props.presence?.sessions ?? []) {
    if (s.principal_id === props.principalId) continue
    const known = map.get(s.principal_id)
    if (known) { known.tabs++; if (RANK[s.mode] < RANK[known.mode]) known.mode = s.mode }
    else map.set(s.principal_id, { id: s.principal_id, name: s.name || 'Someone', mode: s.mode, tabs: 1 })
  }
  return [...map.values()].sort((a, b) => RANK[a.mode] - RANK[b.mode] || a.name.localeCompare(b.name))
})
const shown = computed(() => people.value.slice(0, props.compact ? 2 : 3))
const more = computed(() => people.value.length - shown.value.length)
const MODE = { editing: 'Editing', viewing: 'Viewing', idle: 'Away' } as const
const summary = computed(() => {
  const n = people.value.length
  const editing = people.value.filter(p => p.mode === 'editing').length
  return `${n} ${n === 1 ? 'other person' : 'other people'} here${editing ? `, ${editing} editing` : ''}`
})
const anchor = ref<HTMLElement | null>(null)
const button = ref<HTMLButtonElement>()
function toggle(event: MouseEvent) { anchor.value = anchor.value ? null : event.currentTarget as HTMLElement }
function close(restore: boolean) { anchor.value = null; if (restore) button.value?.focus() }
</script>

<template>
  <button v-if="people.length" ref="button" type="button" class="people" :aria-label="summary" :aria-expanded="!!anchor" aria-haspopup="dialog" :data-tip="summary" @click="toggle">
    <span v-for="person in shown" :key="person.id" class="ring" :class="person.mode" :style="{ '--ring': collaboratorColor(person.id) }">
      <Avatar :id="person.id" :name="person.name" :size="22" />
    </span>
    <span v-if="more > 0" class="more">+{{ more }}</span>
  </button>
  <FloatingPanel v-if="anchor" :anchor="anchor" :width="260" align="end" label="People on this quote" @close="close">
    <p class="eyebrow head">On this quote now</p>
    <ul class="list">
      <li v-for="person in people" :key="person.id" class="person">
        <span class="ring" :class="person.mode" :style="{ '--ring': collaboratorColor(person.id) }"><Avatar :id="person.id" :name="person.name" :size="26" /></span>
        <span class="who"><span class="name">{{ person.name }}</span><span class="mode">{{ MODE[person.mode] }}<template v-if="person.tabs > 1"> · {{ person.tabs }} tabs</template></span></span>
        <span class="swatch" :style="{ background: collaboratorColor(person.id) }" aria-hidden="true" />
      </li>
    </ul>
    <p class="note">Presence is a courtesy, not a lock. If two people change the same text, whoever saves second is asked to choose.</p>
  </FloatingPanel>
</template>

<style scoped>
.people { display: inline-flex; align-items: center; height: 30px; padding: 0 6px 0 4px; border: 0; border-radius: 999px; background: transparent; cursor: pointer; }
@media (hover: hover) { .people:hover { background: var(--row-hover); } }
.people:focus-visible { box-shadow: var(--focus-ring); }
.people .ring + .ring { margin-left: -6px; }
.ring { display: inline-grid; place-items: center; border-radius: 50%; box-shadow: 0 0 0 2px var(--surface-raised-2), 0 0 0 3.5px var(--ring); }
.ring.idle { box-shadow: 0 0 0 2px var(--surface-raised-2), 0 0 0 3.5px var(--line-2); opacity: .75; }
.more { margin-left: 6px; font: 600 11.5px/1 var(--mono); color: var(--ink-2); font-variant-numeric: tabular-nums; }
.head { padding: 4px 10px 6px; }
.list { display: grid; gap: 2px; margin: 0; padding: 0; list-style: none; }
.person { display: flex; align-items: center; gap: 10px; min-height: 40px; padding: 0 10px; border-radius: 8px; }
.person .ring { box-shadow: 0 0 0 2px var(--surface-raised), 0 0 0 3.5px var(--ring); }
.person .ring.idle { box-shadow: 0 0 0 2px var(--surface-raised), 0 0 0 3.5px var(--line-2); }
.who { display: grid; flex: 1; min-width: 0; }
.name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13.5px; font-weight: 600; color: var(--ink); }
.mode { font-size: 12px; color: var(--ink-2); }
.swatch { flex-shrink: 0; width: 10px; height: 10px; border-radius: 3px; }
.note { margin: 6px 10px 4px; font-size: 12px; line-height: 1.45; color: var(--ink-3); }
</style>
