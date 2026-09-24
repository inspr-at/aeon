<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import { DATE_PRESETS, dateRange, dayText, type DatePreset, type QuoteFilter } from '../../../lib/quotes/list'
import BizIcon from '../../business/BizIcon.vue'
import FloatingPanel from '../../work/FloatingPanel.vue'

// The quote-date filter: a few ranges most people want, or two days of your own.
// The button names the range once one is chosen.
const props = defineProps<{ value: QuoteFilter['date']; today: string }>()
const emit = defineEmits<{ change: [value: QuoteFilter['date']] }>()
const anchor = ref<HTMLElement | null>(null)
const button = ref<HTMLButtonElement>()
const active = computed(() => props.value.preset !== 'any' && (props.value.preset !== 'custom' || !!props.value.from || !!props.value.to))
const label = computed(() => {
  if (!active.value) return 'Date'
  if (props.value.preset !== 'custom') return DATE_PRESETS.find(p => p.id === props.value.preset)!.label
  const { from, to } = dateRange(props.value, props.today)
  return from && to ? `${dayText(from)} – ${dayText(to)}` : from ? `From ${dayText(from)}` : `Until ${dayText(to)}`
})
function toggleOpen(event: MouseEvent) { anchor.value = anchor.value ? null : event.currentTarget as HTMLElement }
function close(restore: boolean) { anchor.value = null; if (restore) button.value?.focus() }
function pick(preset: DatePreset) {
  const range = preset === 'custom' && props.value.preset !== 'custom' ? dateRange({ ...props.value, preset: '30d' }, props.today) : { from: props.value.from, to: props.value.to }
  emit('change', { preset, from: range.from, to: range.to })
}
function setDay(which: 'from' | 'to', event: Event) { emit('change', { ...props.value, preset: 'custom', [which]: (event.target as HTMLInputElement).value }) }
function keys(event: KeyboardEvent) {
  if (!['ArrowDown', 'ArrowUp'].includes(event.key) || (event.target as HTMLElement).getAttribute('type') === 'date') return
  const items = [...(event.currentTarget as HTMLElement).querySelectorAll<HTMLInputElement>('input[type="radio"]')]
  const index = items.indexOf(document.activeElement as HTMLInputElement)
  event.preventDefault(); event.stopPropagation()
  const next = items[event.key === 'ArrowDown' ? Math.min(items.length - 1, index + 1) : Math.max(0, index - 1)]
  next?.focus(); next?.click()
}
</script>

<template>
  <button ref="button" type="button" class="btn sm facet-btn" :class="{ on: active }" aria-haspopup="dialog" :aria-expanded="!!anchor" @click="toggleOpen">
    <BizIcon name="calendar" :size="13" class="lead" /><span class="facet-label">{{ label }}</span><BizIcon name="chevron" :size="12" class="facet-chevron" />
  </button>
  <FloatingPanel v-if="anchor" :anchor="anchor" :width="280" label="Filter by quote date" @close="close">
    <div @keydown="keys">
      <div class="facet-head">
        <p class="eyebrow">Quote date</p>
        <button v-if="active" type="button" class="clear" @click="emit('change', { preset: 'any', from: '', to: '' })">Clear</button>
      </div>
      <div class="options" role="radiogroup" aria-label="Quote date">
        <label v-for="preset in DATE_PRESETS" :key="preset.id" class="option">
          <input class="radio" type="radio" name="quote-date" :checked="value.preset === preset.id" :data-autofocus="value.preset === preset.id ? '' : undefined" @change="pick(preset.id)" />
          <span class="option-label">{{ preset.label }}</span>
        </label>
      </div>
      <div v-if="value.preset === 'custom'" class="between">
        <label class="day-field"><span>From</span><input class="field" type="date" :value="value.from" :max="value.to || undefined" @change="setDay('from', $event)" /></label>
        <label class="day-field"><span>To</span><input class="field" type="date" :value="value.to" :min="value.from || undefined" @change="setDay('to', $event)" /></label>
      </div>
      <p class="note">The date on the quote itself; drafts without one use the day they were created.</p>
    </div>
  </FloatingPanel>
</template>

<style scoped>
.facet-btn { gap: 6px; max-width: 260px; padding: 0 9px 0 10px; font-weight: 600; color: var(--ink-2); }
.facet-btn:hover, .facet-btn[aria-expanded="true"] { color: var(--ink); }
.facet-btn.on { color: var(--teal-ink); }
.facet-label { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.lead, .facet-chevron { flex-shrink: 0; color: var(--ink-3); }
.facet-btn.on .lead { color: var(--teal); }
.facet-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-height: 26px; padding: 2px 6px 4px 10px; }
.clear { height: 24px; padding: 0 8px; border: 0; border-radius: 999px; background: transparent; color: var(--teal-ink); font-size: 12px; font-weight: 600; }
.clear:hover { background: var(--row-selected); }
.options { display: grid; gap: 1px; }
.option { display: flex; align-items: center; gap: 10px; min-height: 32px; padding: 0 10px; border-radius: 8px; font-size: 13.5px; cursor: pointer; }
@media (hover: hover) { .option:hover { background: var(--row-hover); } }
.option:focus-within { background: var(--row-selected); }
.radio { width: 15px; height: 15px; margin: 0; accent-color: var(--teal); }
.option-label { flex: 1; min-width: 0; }
.between { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; padding: 8px 10px 2px; }
.day-field { display: grid; gap: 4px; font-size: 12px; color: var(--ink-2); }
.day-field .field { height: 32px; padding: 0 8px; font-size: 13px; }
.note { padding: 8px 10px 4px; font-size: 12px; line-height: 1.45; color: var(--ink-3); }
</style>
