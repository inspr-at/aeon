<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { absoluteTime } from '../../lib/work'
import { harnessLabel } from '../../lib/agentState'
import { chipText, elapsedFor, liveSummary, phaseLabel, who, type LiveAgent } from '../../lib/liveAgents'
import { useLiveAgents } from '../../stores/liveAgents'
import AppIcon from '../AppIcon.vue'
import LiveBot from './LiveBot.vue'

// The agents working in one project right now (AEON-184), as a small living
// chip: their robots at work, the lead's name and ticket, and for how long.
// It sits beside the card's or row's link (never inside it), so it can be a
// button: hover, focus or a click shows who works on what; each agent opens
// its session in Agents, each ticket its page. Status only: nothing here acts.
// Variants: `card` (name, ticket key, elapsed) and `row` (robots and key; on
// a phone robots only).
const props = withDefaults(defineProps<{ agents: LiveAgent[]; project: { title: string; routeKey: string }; variant?: 'card' | 'row' }>(), { variant: 'card' })
const live = useLiveAgents()
const id = useId()
const trigger = ref<HTMLButtonElement>()
const panel = ref<HTMLElement>()
const root = ref<HTMLElement>()
const open = ref(false)
const x = ref(0)
const y = ref(0)
const above = ref(true)
const asleep = ref(false)

const faces = computed(() => props.agents.slice(0, props.variant === 'card' ? 3 : 2))
const more = computed(() => props.agents.length - faces.value.length)
const chip = computed(() => chipText(props.agents))
const summary = computed(() => liveSummary(props.agents))
const lead = computed(() => props.agents[0])
const elapsed = computed(() => lead.value ? elapsedFor(lead.value, live.serverNow) : '')
// Without a ticket, a lead that is still starting (or stopping) says so.
const leadPhase = computed(() => lead.value && lead.value.phase !== 'working' ? phaseLabel(lead.value).toLowerCase() : '')
const clockFormat = new Intl.DateTimeFormat('en-GB', { hour: '2-digit', minute: '2-digit' })
const clock = (iso: string) => clockFormat.format(Date.parse(iso))
const ticketHref = (agent: LiveAgent) => agent.ticket && props.project.routeKey ? `/p/${encodeURIComponent(props.project.routeKey)}/${encodeURIComponent(agent.ticket.key)}` : ''

// ---------- Opening ----------
let openTimer: ReturnType<typeof setTimeout> | undefined
let closeTimer: ReturnType<typeof setTimeout> | undefined
let hovering = false
let quietFocus = false
function show(focusInside = false) {
  clearTimeout(openTimer); clearTimeout(closeTimer)
  if (!open.value) { open.value = true; void nextTick(() => { place(); if (focusInside) firstLink()?.focus() }) }
  else if (focusInside) firstLink()?.focus()
}
function hide(restore = false) {
  clearTimeout(openTimer); clearTimeout(closeTimer)
  if (!open.value) return
  open.value = false
  if (restore) { quietFocus = true; trigger.value?.focus(); quietFocus = false }
}
const firstLink = () => panel.value?.querySelector<HTMLElement>('a[href]') ?? null
function enter(event: PointerEvent) {
  if (event.pointerType === 'touch') return
  hovering = true
  clearTimeout(closeTimer)
  if (!open.value) openTimer = setTimeout(() => show(), 240)
}
function leave(event: PointerEvent) {
  if (event.pointerType === 'touch') return
  hovering = false
  clearTimeout(openTimer)
  closeTimer = setTimeout(() => { if (!hovering && !within(document.activeElement)) hide() }, 180)
}
const within = (node: Element | null) => !!node && (!!panel.value?.contains(node) || node === trigger.value)
function toggle(event: MouseEvent) {
  // A keyboard click (detail 0) opens and moves into the list.
  if (open.value && event.detail !== 0) { hide(); return }
  show(event.detail === 0)
}
function focusIn() { if (!quietFocus && trigger.value?.matches(':focus-visible')) show() }
function focusOut(event: FocusEvent) {
  if (within(event.relatedTarget as Element | null) || hovering) return
  hide()
}
function keydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && open.value) { event.preventDefault(); event.stopPropagation(); hide(true); return }
  if (event.key === 'ArrowDown' && event.target === trigger.value) { event.preventDefault(); show(true); return }
  if (event.key !== 'Tab' || !panel.value?.contains(event.target as Node)) return
  const links = [...panel.value.querySelectorAll<HTMLElement>('a[href]')]
  const edge = event.shiftKey ? links[0] : links[links.length - 1]
  // Leaving the list at either end returns to the chip, so Tab goes on from there.
  if (event.target === edge) { event.preventDefault(); hide(true) }
}
function outside(event: PointerEvent) { if (open.value && !within(event.target as Element)) hide() }

// ---------- Placing ----------
// Above the chip when there is room (cards keep their footers in view), else below.
function place() {
  const anchor = trigger.value, pop = panel.value
  if (!anchor || !pop) return
  const rect = anchor.getBoundingClientRect()
  const width = pop.offsetWidth, height = pop.offsetHeight
  above.value = rect.top - height - 10 > 64 || rect.bottom + height + 10 > innerHeight
  x.value = Math.round(Math.min(Math.max(8, rect.left - 6), innerWidth - width - 8))
  y.value = Math.round(above.value ? Math.max(8, rect.top - height - 8) : rect.bottom + 8)
}
let frame = 0
const replace = () => { if (open.value && !frame) frame = requestAnimationFrame(() => { frame = 0; place() }) }
watch(() => props.agents.length, () => { if (open.value) void nextTick(place) })
watch(() => props.agents.length === 0, gone => { if (gone) hide() })

// ---------- Resting off screen ----------
let seen: IntersectionObserver | undefined
onMounted(() => {
  document.addEventListener('pointerdown', outside, true)
  window.addEventListener('scroll', replace, true)
  window.addEventListener('resize', replace)
  if ('IntersectionObserver' in window && root.value) {
    seen = new IntersectionObserver(([entry]) => { asleep.value = !entry!.isIntersecting })
    seen.observe(root.value)
  }
})
onBeforeUnmount(() => {
  clearTimeout(openTimer); clearTimeout(closeTimer); cancelAnimationFrame(frame)
  document.removeEventListener('pointerdown', outside, true)
  window.removeEventListener('scroll', replace, true)
  window.removeEventListener('resize', replace)
  seen?.disconnect()
})
</script>

<template>
  <span v-if="agents.length" ref="root" class="live" :class="[`as-${variant}`, { open, asleep }]" @keydown="keydown">
    <button
      ref="trigger" type="button" class="live-chip" :aria-expanded="open" :aria-controls="open ? id : undefined"
      :aria-label="`${summary}. Who works on what`" @click="toggle" @pointerenter="enter" @pointerleave="leave" @focusin="focusIn" @focusout="focusOut"
    >
      <span class="faces">
        <LiveBot v-for="(agent, i) in faces" :id="agent.principal_id" :key="agent.session_id ?? `${agent.since}${i}`" class="face" :index="i" :lead="i === 0" :size="variant === 'card' ? 26 : 22" />
        <span v-if="agents.length > 1" class="count mono">{{ agents.length }}</span>
      </span>
      <span v-if="more" class="more mono">+{{ more }}</span>
      <span v-if="variant === 'card'" class="words">
        <span class="name">{{ chip.name }}</span>
        <template v-if="chip.key"><span class="dot" /><span class="key mono">{{ chip.key }}</span></template>
        <template v-else-if="leadPhase"><span class="dot" /><span class="phase-word">{{ leadPhase }}</span></template>
        <span class="elapsed mono">{{ elapsed }}</span>
      </span>
      <span v-else-if="chip.key" class="key mono">{{ chip.key }}</span>
      <span v-else class="row-name">{{ chip.name }}</span>
      <span class="typing" aria-hidden="true"><i /><i /><i /></span>
    </button>
    <Teleport to="body">
      <div
        v-if="open" :id="id" ref="panel" class="live-pop floating pop" :class="{ above }" role="dialog" :aria-label="`Agents working on ${project.title}`"
        :style="{ transform: `translate(${x}px, ${y}px)` }" @pointerenter="enter" @pointerleave="leave" @focusout="focusOut" @keydown="keydown"
      >
        <p class="pop-head">
          <span class="pulse" aria-hidden="true" />
          <span>{{ agents.length }} {{ agents.length === 1 ? 'agent' : 'agents' }} working</span>
          <span class="pop-project">{{ project.title }}</span>
        </p>
        <ul class="pop-list">
          <li v-for="(agent, i) in agents" :key="agent.session_id ?? `${agent.since}${i}`" class="pop-agent">
            <component
              :is="agent.session_id ? RouterLink : 'div'" class="agent-line" :to="agent.session_id ? `/agents/${encodeURIComponent(agent.session_id)}` : undefined"
              :aria-label="agent.session_id ? `${who(agent)}, ${harnessLabel(agent.harness)}, ${phaseLabel(agent).toLowerCase()} for ${elapsedFor(agent, live.serverNow)}. Open the session` : undefined"
            >
              <LiveBot :id="agent.principal_id" :index="i" :size="30" />
              <span class="agent-text">
                <span class="agent-name">{{ who(agent) }}<span class="harness">{{ harnessLabel(agent.harness) }}</span></span>
                <span class="agent-meta"><span class="phase" :class="agent.phase">{{ phaseLabel(agent) }}</span><span class="sep" />for <time class="mono" :datetime="agent.since" :title="absoluteTime(agent.since)">{{ elapsedFor(agent, live.serverNow) }}</time><span class="sep" /><span class="since">since {{ clock(agent.since) }}</span></span>
              </span>
              <AppIcon v-if="agent.session_id" class="go" name="chevron-right" :size="14" />
            </component>
            <RouterLink v-if="agent.ticket && ticketHref(agent)" class="ticket-line" :to="ticketHref(agent)">
              <span class="ticket-key mono">{{ agent.ticket.key }}</span><span class="ticket-title">{{ agent.ticket.title }}</span>
            </RouterLink>
            <p v-else class="ticket-line none">{{ agent.ticket ? agent.ticket.key : 'No ticket bound' }}</p>
          </li>
        </ul>
      </div>
    </Teleport>
  </span>
</template>

<style scoped>
.live { position: relative; display: inline-flex; min-width: 0; max-width: 100%; }
.live-chip {
  position: relative; display: inline-flex; align-items: center; gap: 7px; min-width: 0; max-width: 100%; height: 32px; padding: 0 11px 0 3px;
  border: 0; border-radius: 999px; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line), 0 1px 2px rgba(14, 111, 108, .08);
  color: var(--ink); font: 500 12px/1 var(--font); cursor: pointer; -webkit-user-select: none; user-select: none;
}
.live-chip:hover, .live.open .live-chip { box-shadow: inset 0 0 0 1px var(--chip-teal-line), 0 6px 16px -8px rgba(14, 111, 108, .55); }
/* A finger's reach: the chip answers a little beyond its edge (44px tall, and wide on a phone row). */
.live-chip::before { content: ''; position: absolute; inset: -6px -2px; border-radius: 999px; }
.live-chip:focus-visible { outline: none; box-shadow: inset 0 0 0 1px var(--chip-teal-line), var(--focus-ring); }
.faces { position: relative; display: inline-flex; align-items: center; flex-shrink: 0; }
.face + .face { margin-left: -10px; }
.face:nth-child(1) { z-index: 3; } .face:nth-child(2) { z-index: 2; } .face:nth-child(3) { z-index: 1; }
/* How many there are, on the lead's shoulder: only where the other faces step aside (a phone row). */
.count { display: none; position: absolute; z-index: 4; right: -6px; bottom: -3px; min-width: 15px; height: 15px; padding: 0 4px; border-radius: 999px; background: var(--teal); color: var(--button-ink); box-shadow: 0 0 0 1.5px var(--surface-raised); font-size: 9.5px; font-weight: 700; line-height: 15px; text-align: center; }
.more { flex-shrink: 0; margin-left: -3px; font-size: 10.5px; font-weight: 600; color: var(--teal-ink); }
.words { display: inline-flex; align-items: center; gap: 6px; min-width: 0; }
.name, .row-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 650; letter-spacing: -.005em; }
.row-name { font-weight: 600; font-size: 11.5px; }
.dot { flex-shrink: 0; width: 3px; height: 3px; border-radius: 50%; background: var(--ink-3); }
.key { flex-shrink: 0; font-size: 11px; font-weight: 600; letter-spacing: .04em; color: var(--teal-ink); white-space: nowrap; }
.phase-word { flex-shrink: 0; color: var(--ink-2); }
.elapsed { flex-shrink: 0; font-size: 11px; color: var(--ink-3); }
/* Three dots take turns, like someone typing. */
.typing { display: inline-flex; align-items: center; gap: 2px; flex-shrink: 0; height: 10px; }
.typing i { width: 3px; height: 3px; border-radius: 50%; background: var(--teal); opacity: .5; }
.as-row .live-chip { height: 28px; gap: 6px; padding: 0 9px 0 3px; }
.as-row .face + .face { margin-left: -9px; }
/* A phone row keeps one robot, its count on its shoulder, in the row's right gutter. */
@media (max-width: 760px) {
  .as-row .key, .as-row .row-name, .as-row .typing, .as-row .more, .as-row .face + .face { display: none; }
  .as-row .count { display: block; }
  .as-row .live-chip { height: 32px; padding: 0 3px; }
  .as-row .live-chip::before { inset: -6px -8px; }
}
@container live-card (max-width: 300px) { .elapsed { display: none; } }
@media (prefers-reduced-motion: no-preference) {
  .typing i { animation: live-typing 1.2s ease-in-out infinite; }
  .typing i:nth-child(2) { animation-delay: .16s; }
  .typing i:nth-child(3) { animation-delay: .32s; }
  .live-chip { transition: box-shadow .15s ease, translate .15s ease; }
  .live-chip:hover { translate: 0 -1px; }
}
@keyframes live-typing { 0%, 60%, 100% { transform: translateY(0); opacity: .45; } 30% { transform: translateY(-2.5px); opacity: 1; } }
.live.asleep .typing i { animation-play-state: paused; }

/* ---------- The popover ---------- */
.live-pop { position: fixed; z-index: 75; top: 0; left: 0; width: min(330px, calc(100vw - 16px)); padding: 8px; }
@media (prefers-reduced-motion: no-preference) {
  .live-pop { animation: live-pop-in .16s cubic-bezier(.2, .7, .2, 1); }
  .live-pop:not(.above) { animation-name: live-pop-in-below; }
}
@keyframes live-pop-in { from { opacity: 0; margin-top: 4px; } to { opacity: 1; margin-top: 0; } }
@keyframes live-pop-in-below { from { opacity: 0; margin-top: -4px; } to { opacity: 1; margin-top: 0; } }
.pop-head { display: flex; align-items: center; gap: 8px; padding: 4px 8px 8px; font: 600 11px/1.2 var(--mono); letter-spacing: .08em; text-transform: uppercase; color: var(--ink-2); font-variant-ligatures: none; }
.pop-project { margin-left: auto; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 50%; font: 500 12px/1.2 var(--font); letter-spacing: 0; text-transform: none; color: var(--ink-3); }
.pulse { position: relative; flex-shrink: 0; width: 7px; height: 7px; border-radius: 50%; background: var(--ok); }
@media (prefers-reduced-motion: no-preference) {
  .pulse::after { content: ''; position: absolute; inset: 0; border-radius: 50%; background: inherit; animation: live-pulse 1.8s cubic-bezier(.2, .7, .2, 1) infinite; }
}
@keyframes live-pulse { from { transform: scale(1); opacity: .5; } to { transform: scale(2.6); opacity: 0; } }
.pop-list { display: grid; gap: 4px; margin: 0; padding: 0; list-style: none; }
.pop-agent { display: grid; gap: 2px; padding: 4px; border-radius: 10px; background: var(--surface-sunken); }
.agent-line { display: flex; align-items: center; gap: 10px; min-height: 44px; padding: 4px 6px; border-radius: 8px; color: var(--ink); text-decoration: none; }
a.agent-line:hover, .ticket-line[href]:hover { background: var(--row-hover); }
a.agent-line:focus-visible, .ticket-line:focus-visible { outline: none; box-shadow: var(--focus-ring); }
.agent-text { display: grid; gap: 3px; min-width: 0; flex: 1; }
.agent-name { display: flex; align-items: center; gap: 7px; min-width: 0; font-size: 13.5px; font-weight: 650; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.harness { flex-shrink: 0; padding: 2px 6px; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); font: 500 10.5px/1.2 var(--font); color: var(--ink-2); }
.agent-meta { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--ink-2); }
.phase { font-weight: 600; color: var(--ok); }
.phase.starting, .phase.stopping { color: var(--teal-ink); }
.sep { width: 3px; height: 3px; border-radius: 50%; background: var(--ink-3); }
.agent-meta time { color: var(--ink); font-size: 11.5px; }
.since { white-space: nowrap; color: var(--ink-3); }
.go { flex-shrink: 0; color: var(--ink-3); }
a.agent-line:hover .go { color: var(--teal-ink); }
.ticket-line { display: flex; align-items: baseline; gap: 8px; min-width: 0; margin: 0; padding: 6px 8px 7px 46px; border-radius: 8px; color: var(--ink-2); font-size: 12.5px; text-decoration: none; }
.ticket-key { flex-shrink: 0; font-size: 11px; font-weight: 600; letter-spacing: .04em; color: var(--teal-ink); }
.ticket-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ticket-line.none { color: var(--ink-3); font-size: 12px; }
@media (max-width: 760px) { .ticket-line { align-items: center; min-height: 44px; } }
</style>
