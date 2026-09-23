<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import type { Dimension, FacetOption } from '../../lib/ticketList'
import AppIcon from '../AppIcon.vue'
import PersonAvatar from './PersonAvatar.vue'
import PriorityIcon from './PriorityIcon.vue'
import StatusIcon from './StatusIcon.vue'

defineProps<{ dimension: Dimension; options: FacetOption[]; selected: string[] }>()
const emit = defineEmits<{ toggle: [value: string] }>()
// Keep keyboard focus on the option just toggled, also when its label text was clicked.
function changed(event: Event, value: string) {
  (event.target as HTMLInputElement).focus({ preventScroll: true })
  emit('toggle', value)
}
function move(event: KeyboardEvent) {
  if (!['ArrowDown', 'ArrowUp', 'j', 'k'].includes(event.key)) return
  const root = event.currentTarget as HTMLElement
  const items = [...root.querySelectorAll<HTMLInputElement>('input')]
  const index = items.indexOf(document.activeElement as HTMLInputElement)
  const next = event.key === 'ArrowDown' || event.key === 'j' ? Math.min(items.length - 1, index + 1) : Math.max(0, index - 1)
  event.preventDefault(); event.stopPropagation()
  items[next]?.focus()
}
</script>

<template>
  <div class="facet-options" role="group" @keydown="move">
    <label v-for="option in options" :key="option.value" class="facet-option" :class="{ muted: !option.count && !selected.includes(option.value) }">
      <input class="check-box" type="checkbox" :checked="selected.includes(option.value)" @change="changed($event, option.value)" />
      <StatusIcon v-if="dimension === 'status'" :state="option.value" />
      <PriorityIcon v-else-if="dimension === 'priority' && option.value !== 'none'" :priority="option.value" />
      <span v-else-if="dimension === 'priority'" class="no-icon" />
      <PersonAvatar v-else-if="dimension === 'assignee' && option.value !== 'none'" :name="option.label" :size="18" />
      <AppIcon v-else-if="dimension === 'assignee'" name="user" :size="14" class="unassigned" />
      <AppIcon v-else :name="option.value === 'epic' ? 'epic' : option.value === 'task' ? 'task' : 'ticket'" :size="14" class="kind" :class="option.value" />
      <span class="option-label">{{ option.label }}</span>
      <span class="count mono">{{ option.count ?? '' }}</span>
    </label>
  </div>
</template>

<style scoped>
.facet-options { display: grid; gap: 1px; }
.facet-option { display: flex; align-items: center; gap: 10px; min-height: 32px; padding: 0 10px; border-radius: 8px; font-size: 13.5px; cursor: pointer; }
@media (hover: hover) { .facet-option:hover { background: var(--row-hover); } }
.facet-option:focus-within { background: var(--row-selected); }
.facet-option:active { background: var(--row-selected); }
.facet-option.muted .option-label { color: var(--ink-3); }
.option-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.count { font-size: 11.5px; color: var(--ink-3); }
.no-icon { width: 14px; height: 2px; border-radius: 2px; background: var(--line-2); }
.kind, .unassigned { color: var(--ink-3); }
.kind.epic { color: var(--gold); }
</style>
