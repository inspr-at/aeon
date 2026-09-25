<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import type { GroupDef } from '../../lib/projectGroups'
import AppIcon from '../AppIcon.vue'
import GroupMarker from './GroupMarker.vue'

// One chip per group beside the filter: its marker, name and count. A filled dot
// means shown, a ring hidden; a click shows or hides the group. Chips also take
// dropped projects. The last chip starts a new group.
defineProps<{ chips: { group: GroupDef; count: number; hidden: boolean }[]; dropOn: string | null; creating: boolean }>()
const emit = defineEmits<{ toggle: [id: string]; create: [anchor: HTMLElement] }>()
// One line that never wraps (the page below never moves when chips arrive); when
// the chips do not fit, the row scrolls sideways and fades at the edge that has more.
const row = ref<HTMLElement>()
const fade = ref<'' | 'end' | 'start' | 'both'>('')
function measure() {
  const el = row.value
  if (!el) return
  const more = el.scrollWidth - el.clientWidth > 1
  const start = more && el.scrollLeft > 1, end = more && el.scrollLeft < el.scrollWidth - el.clientWidth - 1
  fade.value = start && end ? 'both' : start ? 'start' : end ? 'end' : ''
}
let sizer: ResizeObserver | undefined
onMounted(() => { measure(); sizer = new ResizeObserver(measure); if (row.value) { sizer.observe(row.value); for (const child of row.value.children) sizer.observe(child) } })
onBeforeUnmount(() => sizer?.disconnect())
// Keyboard focus on a chip out of view scrolls it in.
function focusIn(event: FocusEvent) { (event.target as HTMLElement).scrollIntoView?.({ block: 'nearest', inline: 'nearest' }) }
</script>

<template>
  <div ref="row" class="group-chips" :class="fade && `fade-${fade}`" role="group" aria-label="Groups" @scroll.passive="measure" @focusin="focusIn">
    <button
      v-for="chip in chips" :key="chip.group.id" type="button" class="group-chip" :class="{ off: chip.hidden, 'drop-on': dropOn === chip.group.id }"
      :aria-pressed="!chip.hidden" :aria-label="`${chip.group.name}, ${chip.count} ${chip.count === 1 ? 'project' : 'projects'}`"
      :data-tip="chip.hidden ? `Hidden · click to show ${chip.group.name}` : `Shown · click to hide ${chip.group.name}`" :data-group-drop="chip.group.id"
      @click="emit('toggle', chip.group.id)"
    >
      <GroupMarker :group="chip.group" :hollow="chip.hidden" />
      <span class="chip-name">{{ chip.group.name }}</span>
      <span class="chip-count mono">{{ chip.count }}</span>
    </button>
    <button type="button" class="chip-add" :aria-expanded="creating" aria-haspopup="dialog" data-tip="New group · your own until an admin shares it" @click="emit('create', $event.currentTarget as HTMLElement)">
      <AppIcon name="plus" :size="12" /><span>Group</span>
    </button>
  </div>
</template>

<style scoped>
/* Padding inside the scroller keeps focus rings and phone hit areas unclipped. */
.group-chips { display: flex; align-items: center; flex-wrap: nowrap; gap: 6px; min-width: 0; margin: -6px -4px; padding: 6px 4px; overflow-x: auto; overscroll-behavior-x: contain; scrollbar-width: none; }
.group-chips::-webkit-scrollbar { display: none; }
.group-chips.fade-end { mask-image: linear-gradient(to right, #000 calc(100% - 36px), transparent); }
.group-chips.fade-start { mask-image: linear-gradient(to left, #000 calc(100% - 36px), transparent); }
.group-chips.fade-both { mask-image: linear-gradient(to right, transparent, #000 36px, #000 calc(100% - 36px), transparent); }
.group-chip, .chip-add { flex-shrink: 0; }
.group-chip, .chip-add {
  display: inline-flex; align-items: center; gap: 7px; height: 30px; padding: 0 11px 0 10px; border: 0; border-radius: 999px;
  background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); color: var(--ink); font-size: 12.5px; font-weight: 600; white-space: nowrap;
}
.chip-name { max-width: 16ch; overflow: hidden; text-overflow: ellipsis; }
.chip-count { font-size: 11px; font-weight: 600; color: var(--ink-2); }
.group-chip.off { background: transparent; color: var(--ink-2); font-weight: 500; }
.group-chip.off .chip-count { color: var(--ink-3); }
@media (hover: hover) { .group-chip:hover, .chip-add:hover { background: var(--row-hover); box-shadow: inset 0 0 0 1px var(--line-2); } }
.group-chip:active, .chip-add:active { background: var(--row-selected); }
.group-chip:focus-visible, .chip-add:focus-visible { box-shadow: var(--focus-ring); }
.group-chip.drop-on { background: var(--row-selected); box-shadow: inset 0 0 0 1.5px var(--teal); }
.chip-add { gap: 5px; padding: 0 11px 0 9px; background: transparent; box-shadow: inset 0 0 0 1px var(--line-2); color: var(--teal-ink); }
.chip-add svg { color: var(--teal-ink); }
/* Phones: chips a full touch target high. */
@media (max-width: 760px) {
  .group-chip, .chip-add { height: 44px; padding: 0 14px 0 13px; font-size: 13px; }
}
</style>
