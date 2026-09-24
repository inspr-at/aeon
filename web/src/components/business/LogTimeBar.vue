<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script lang="ts">
export interface LogTicket { id: string; key: string; title: string }
export interface LogRequest { ticket: LogTicket; costUnitId: string; currency: string; day: Date; seconds: number; startMinutes: number | null; note: string }
// The cost unit last used, for the session (no device storage, like other preferences).
let lastCostUnit = ''
</script>
<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { listNodes } from '../../lib/api'
import { recents } from '../../lib/recents'
import { dayKey, parseTimeInput, WEEKDAYS } from '../../lib/week'
import { formatSpan, parseDurationInput } from './duration'
import { formatAmount } from './money'
import { useBusiness } from '../../stores/business'
import AppIcon from './BizIcon.vue'
import PickerMenu, { type PickOption } from './PickerMenu.vue'

// One line to log time: a ticket, the cost unit that prices it, the day, and a
// duration typed the way people say it ("1h30", "90m", "1.5"). Enter logs it and
// keeps ticket, cost unit and day for the next entry.
const props = defineProps<{ days: Date[]; suggestions: LogTicket[]; busy: boolean; preset?: LogTicket | null }>()
const emit = defineEmits<{ log: [request: LogRequest] }>()
const business = useBusiness()
const ticket = ref<LogTicket | null>(null)
const costUnitId = ref(lastCostUnit)
const dayIndex = ref(0)
const duration = ref('')
const start = ref('')
const note = ref('')
const picker = ref<HTMLElement | null>(null)
const pickButton = ref<HTMLButtonElement>()
const durationInput = ref<HTMLInputElement>()
const tried = ref(false)

const todayKey = dayKey(new Date())
watch(() => props.days, days => {
  const today = days.findIndex(day => dayKey(day) === todayKey)
  dayIndex.value = today === -1 ? 0 : today
}, { immediate: true })
watch(() => props.preset, value => { if (value) ticket.value = value }, { immediate: true })
const day = computed(() => props.days[dayIndex.value])
const utcDay = computed(() => day.value ? new Date(Date.UTC(day.value.getFullYear(), day.value.getMonth(), day.value.getDate())).toISOString().slice(0, 10) : '')
// Hourly cost units with a rate on that day, in any currency.
const hourly = computed(() => business.costUnits.filter(unit => !['cancelled', 'archived', 'done'].includes(unit.node.state) && unit.rates.some(rate => rate.unit === 'hour' && rate.effective_from <= utcDay.value && (!rate.effective_until || rate.effective_until > utcDay.value))))
watch(hourly, units => { if (!units.some(unit => unit.node.id === costUnitId.value)) costUnitId.value = units.length === 1 ? units[0].node.id : '' }, { immediate: true })
watch(costUnitId, value => { if (value) lastCostUnit = value })
const rate = computed(() => {
  const unit = business.costUnit(costUnitId.value)
  const currencies = [...new Set(unit?.rates.filter(r => r.unit === 'hour').map(r => r.currency) ?? [])].sort((a, b) => (a === 'EUR' ? -1 : b === 'EUR' ? 1 : a.localeCompare(b)))
  for (const currency of currencies) { const found = business.rateOn(costUnitId.value, 'hour', currency, utcDay.value); if (found) return found }
  return null
})
const seconds = computed(() => parseDurationInput(duration.value))
const startMinutes = computed(() => start.value.trim() ? parseTimeInput(start.value) : null)
const problem = computed(() => {
  if (!ticket.value) return 'Choose a ticket.'
  if (!costUnitId.value) return hourly.value.length ? 'Choose a cost unit.' : 'No cost unit has an hourly rate on this day. Add one under Rates.'
  if (!rate.value) return 'This cost unit has no hourly rate on this day.'
  if (!duration.value.trim()) return 'Enter how long, like 1h30 or 90m.'
  if (!seconds.value) return 'Use a duration like 1h30, 90m or 1.5 (up to 24h).'
  if (start.value.trim() && startMinutes.value === null) return 'Start is a time like 9:30.'
  return ''
})
const preview = computed(() => seconds.value && rate.value ? `${formatSpan(seconds.value)} · ${formatAmount(multiply(rate.value.bill_amount, seconds.value), rate.value.currency)} ${rate.value.currency}` : '')
// Exact amount the server will book: rate × seconds / 3600, rounded half up to four places.
function multiply(amount: string, secs: number) {
  const [whole, frac = ''] = amount.split('.')
  const units = BigInt(whole + frac.padEnd(4, '0').slice(0, 4)) * BigInt(secs)
  const rounded = (units + 1800n) / 3600n
  const text = rounded.toString().padStart(5, '0')
  return `${text.slice(0, -4)}.${text.slice(-4)}`
}

const ticketOptions = computed<PickOption[]>(() => {
  const seen = new Set<string>()
  const out: PickOption[] = []
  for (const item of [...props.suggestions, ...recents.filter(r => r.type === 'ticket').map(r => ({ id: '', key: r.key, title: r.title }))]) {
    if (seen.has(item.key)) continue
    seen.add(item.key)
    out.push({ value: item.id || `key:${item.key}`, label: item.title, badge: item.key })
  }
  return out.slice(0, 10)
})
async function searchTickets(term: string): Promise<PickOption[]> {
  const page = await listNodes({ q: term, kind: ['ticket', 'task', 'epic'], sort: /^[a-z]+-\d*$/i.test(term) ? 'key' : '-updated_at', limit: 10 })
  return page.items.map(item => ({ value: item.id, label: item.title, badge: item.key, hint: item.project?.title }))
}
async function chooseTicket(option: PickOption) {
  picker.value = null
  let id = option.value
  if (id.startsWith('key:')) {
    const key = id.slice(4)
    const page = await listNodes({ q: key, sort: 'key', limit: 5 })
    id = page.items.find(item => item.key === key)?.id ?? ''
    if (!id) return
  }
  ticket.value = { id, key: option.badge ?? '', title: option.label }
  await nextTick(); durationInput.value?.focus()
}
function closePicker(restore: boolean) { picker.value = null; if (restore) pickButton.value?.focus() }
function submit() {
  tried.value = true
  if (problem.value || props.busy || !ticket.value || !rate.value || !seconds.value || !day.value) return
  emit('log', { ticket: ticket.value, costUnitId: costUnitId.value, currency: rate.value.currency, day: day.value, seconds: seconds.value, startMinutes: startMinutes.value, note: note.value.trim() })
}
function reset() { duration.value = ''; start.value = ''; note.value = ''; tried.value = false; void nextTick(() => durationInput.value?.focus()) }
function keys(event: KeyboardEvent) { if (event.key === 'Enter' && (event.target as HTMLElement).tagName === 'INPUT') { event.preventDefault(); submit() } }
defineExpose({ reset, focus: () => (ticket.value ? durationInput.value : pickButton.value)?.focus(), openPicker: () => { picker.value = pickButton.value ?? null } })
</script>

<template>
  <form class="log-bar" aria-label="Log time" @submit.prevent="submit" @keydown="keys">
    <button ref="pickButton" type="button" class="pick ticket-pick" :class="{ unset: !ticket }" aria-haspopup="dialog" :aria-expanded="!!picker" :aria-label="ticket ? `Ticket: ${ticket.key} ${ticket.title}` : 'Ticket: choose a ticket'" @click="picker = picker ? null : ($event.currentTarget as HTMLElement)">
      <span v-if="ticket" class="key-badge">{{ ticket.key }}</span><AppIcon v-else name="ticket" :size="14" />
      <span class="pick-text">{{ ticket?.title ?? 'Ticket' }}</span><AppIcon name="chevron" :size="12" class="chev" />
    </button>
    <label class="select-wrap">
      <span class="sr-only">Cost unit</span>
      <select v-model="costUnitId" class="pick select" :class="{ unset: !costUnitId }" aria-label="Cost unit">
        <option value="" disabled>Cost unit</option>
        <option v-for="unit in hourly" :key="unit.node.id" :value="unit.node.id">{{ unit.node.title }}</option>
      </select>
      <AppIcon name="chevron" :size="12" class="select-chev" />
    </label>
    <div class="seg days" role="radiogroup" aria-label="Day">
      <button v-for="(d, i) in days" :key="i" type="button" role="radio" :aria-checked="dayIndex === i" :aria-label="`${WEEKDAYS[i]} ${d.getDate()}`" :class="{ today: dayKey(d) === todayKey }" @click="dayIndex = i">{{ WEEKDAYS[i].slice(0, 2) }}</button>
    </div>
    <input v-model="start" class="field start" inputmode="numeric" placeholder="Start" aria-label="Start time (optional)" autocomplete="off" :data-tip="'Optional, like 9:30. Empty: after the day’s last entry'" />
    <input ref="durationInput" v-model="duration" class="field duration" :class="{ bad: tried && !!duration.trim() && !seconds }" placeholder="1h30" aria-label="Duration" autocomplete="off" />
    <input v-model="note" class="field note" placeholder="Note" aria-label="Note (optional)" maxlength="500" autocomplete="off" />
    <button type="submit" class="btn primary log-btn" :disabled="busy"><AppIcon name="plus" :size="13" />{{ busy ? 'Logging…' : 'Log' }}</button>
    <p class="log-status" :class="{ warn: tried && !!problem }" aria-live="polite">{{ tried && problem ? problem : preview || 'Enter logs it. Ticket, cost unit and day stay for the next entry.' }}</p>
    <PickerMenu v-if="picker" :anchor="picker" title="Ticket" :options="ticketOptions" :search="searchTickets" :current="ticket?.id" placeholder="Search tickets by key or title…" :width="380" empty="No ticket matches." @choose="chooseTicket" @close="closePicker" />
  </form>
</template>

<style scoped>
.log-bar { display: grid; grid-template-columns: minmax(180px, 1.4fr) minmax(130px, .8fr) auto 72px 84px minmax(100px, 1fr) auto; align-items: center; gap: 8px; }
.pick { display: flex; align-items: center; gap: 8px; min-width: 0; height: 34px; padding: 0 10px 0 11px; border: 1px solid var(--glass-edge); border-radius: var(--radius-s); background: var(--field-bg); box-shadow: var(--field-inset), 0 0 0 1px var(--line); color: var(--ink); font-size: 13.5px; text-align: left; }
.pick:hover { box-shadow: var(--field-inset), 0 0 0 1px var(--glass-rim); }
.pick:focus-visible { box-shadow: var(--focus-ring); }
.pick.unset { color: var(--ink-3); }
.pick svg { flex-shrink: 0; color: var(--ink-3); }
.pick .key-badge { height: 20px; padding: 0 6px; font-size: 10.5px; }
.pick-text { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.select-wrap { position: relative; display: block; min-width: 0; }
.select { width: 100%; appearance: none; padding-right: 26px; cursor: pointer; }
.select-chev { position: absolute; right: 10px; top: 50%; margin-top: -6px; color: var(--ink-3); pointer-events: none; }
.days button { min-width: 30px; padding: 0 7px; font-family: var(--mono); font-size: 11.5px; text-transform: uppercase; }
.days button.today:not([aria-checked="true"]) { color: var(--teal-ink); }
.field { height: 34px; font-size: 13.5px; }
.start, .duration { font-family: var(--mono); font-size: 13px; font-variant-ligatures: none; }
.duration.bad { box-shadow: var(--field-inset), 0 0 0 1px var(--danger-line); color: var(--danger); }
.log-btn { height: 34px; }
.log-status { grid-column: 1 / -1; min-height: 18px; font-size: 12px; color: var(--ink-3); }
.log-status.warn { color: var(--gold-ink); }
@container (max-width: 980px) {
  .log-bar { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
  .days { grid-column: 1 / -1; justify-self: start; }
  .note { grid-column: 1 / -1; }
  .log-btn { grid-column: 2; justify-self: end; }
}
@media (max-width: 720px) {
  .log-bar { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
  .ticket-pick, .select-wrap, .note, .days { grid-column: 1 / -1; }
  .days { display: grid; grid-template-columns: repeat(7, 1fr); justify-self: stretch; }
  .days button { height: 36px; }
  .pick, .field, .log-btn { height: 44px; font-size: 16px; }
  .log-btn { grid-column: 1 / -1; justify-self: stretch; }
}
</style>
