<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script lang="ts">
import { reactive } from 'vue'
// People whose picture is known to be missing this session: their initials show
// without asking again.
const missing = reactive(new Set<string>())
export function forgetMissing(id: string) { missing.delete(id) }
</script>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { avatarColor, avatarSources } from '../lib/avatar'
import { initials as nameInitials } from '../lib/work'
import { useProfile } from '../stores/profile'
import AppIcon from './AppIcon.vue'

// One avatar for every person on screen: their picture at the right size
// (srcset over the 32/64/128/256 variants), else their initials on their colour.
// Agents keep the robot glyph. Decorative unless given a label, since a name
// sits beside it almost everywhere.
const props = withDefaults(defineProps<{
  id?: string | null; name: string; size?: number; kind?: 'person' | 'agent'; label?: string; picture?: boolean
}>(), { id: null, size: 24, kind: 'person', label: undefined, picture: true })
const me = useProfile()
const mine = computed(() => !!props.id && props.id === me.id)
const hashes = computed(() => mine.value ? me.profile?.avatar_hashes ?? {} : {})
const letters = computed(() => mine.value && me.profile?.initials ? me.profile.initials : nameInitials(props.name))
const color = computed(() => mine.value ? me.color : avatarColor(props.id ?? props.name))
// My own picture is asked for only when my profile says there is one.
// picture false: the caller knows there is none (the members list says so).
const wanted = computed(() => props.kind === 'person' && props.picture && !!props.id && !missing.has(props.id) && (!mine.value || me.hasPicture))
const sources = computed(() => props.id ? avatarSources(props.id, props.size, hashes.value) : null)
const loaded = ref(false)
watch(() => sources.value?.src, () => { loaded.value = false })
function failed() { if (props.id && !mine.value) missing.add(props.id); loaded.value = false }
</script>

<template>
  <span
    class="avatar" :class="[`c-${color}`, { agent: kind === 'agent', loaded }]" :style="{ '--size': `${size}px` }"
    :role="label ? 'img' : undefined" :aria-label="label" :aria-hidden="label ? undefined : 'true'"
  >
    <AppIcon v-if="kind === 'agent'" name="agent" :size="Math.round(size * .6)" />
    <template v-else>
      <span class="letters">{{ letters }}</span>
      <img v-if="wanted && sources" :key="sources.src" :src="sources.src" :srcset="sources.srcset" alt="" :width="size" :height="size" loading="lazy" decoding="async" draggable="false" @load="loaded = true" @error="failed" />
    </template>
  </span>
</template>

<style scoped>
/* Twelve muted palette hues (the server's avatar_color), light and dark. */
.avatar {
  --c: .05; --h: 250;
  position: relative; display: inline-grid; place-items: center; flex-shrink: 0; width: var(--size); height: var(--size); border-radius: 50%; overflow: hidden;
  background: oklch(var(--avatar-l-bg) var(--c) var(--h)); color: oklch(var(--avatar-l-ink) calc(var(--c) * 1.6) var(--h));
  box-shadow: inset 0 0 0 1px var(--avatar-rim);
  font: 650 calc(var(--size) * .38)/1 var(--font); letter-spacing: .01em; user-select: none;
}
.letters { font-variant-numeric: tabular-nums; }
img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity .2s ease; }
.loaded img { opacity: 1; }
.loaded .letters { visibility: hidden; }
.agent { background: var(--surface-2); color: var(--ink-2); box-shadow: inset 0 0 0 1px var(--line); }
.c-slate { --h: 255; --c: .02; } .c-sage { --h: 150; --c: .04; } .c-moss { --h: 125; --c: .06; } .c-ocean { --h: 222; --c: .06; }
.c-steel { --h: 238; --c: .03; } .c-denim { --h: 258; --c: .07; } .c-iris { --h: 290; --c: .07; } .c-plum { --h: 330; --c: .06; }
.c-rose { --h: 12; --c: .07; } .c-clay { --h: 45; --c: .07; } .c-sand { --h: 82; --c: .06; } .c-teal { --h: 188; --c: .06; }
@media (prefers-reduced-motion: reduce) { img { transition: none; } }
</style>
