<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { getNode, type WorkNode } from '../../lib/api'
import { featureState } from '../../lib/journey'
import { useJourney } from '../../stores/journey'
import JourneyIcon from './JourneyIcon.vue'
import TicketDetails from './TicketDetails.vue'
import MarkdownBody from '../MarkdownBody.vue'
const props = defineProps<{ initialTicket?: string; canEdit: boolean }>()
const emit = defineEmits<{ close: []; openNode: [id: string] }>()
const store = useJourney()
const dialog = ref<HTMLDialogElement>(), rail = ref<HTMLElement>(), queryInput = ref<HTMLInputElement>()
const selectedId = ref(props.initialTicket || store.tickets[0]?.ticket_node_id || '')
const ticket = computed(() => store.tickets.find(t => t.ticket_node_id === selectedId.value))
const index = computed(() => store.tickets.findIndex(t => t.ticket_node_id === selectedId.value))
const node = ref<WorkNode>(), screens = ref<WorkNode[]>([]), screenIndex = ref(0), compareIndex = ref(0)
const comparePosition = ref(50)
const screen = computed(() => screens.value[screenIndex.value])
const detail = ref(true), compare = ref(false), zoom = ref(false), sheet = ref(false), search = ref(false), query = ref('')
const loading = ref(false), error = ref(''), leftFade = ref(false), rightFade = ref(false), dragging = ref(false)
const results = computed(() => store.tickets.filter(t => `${t.key} ${t.title} ${store.walker?.features.find(f => f.feature_node_id === t.feature_node_id)?.title || ''}`.toLocaleLowerCase().includes(query.value.toLocaleLowerCase())))
const editable = computed(() => props.canEdit && store.planning)
const feature = computed(() => store.walker?.features.find(f => f.feature_node_id === ticket.value?.feature_node_id))
let request = 0, observer: ResizeObserver | undefined, originalFocus: HTMLElement | null = null
let down: { x: number; scroll: number; pointer: number } | undefined, suppressClick = false
let velocity = 0, lastX = 0, lastTime = 0, frame = 0
const reduced = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches
function updateFades() {
  const el = rail.value
  if (!el) return
  leftFade.value = el.scrollLeft > 4; rightFade.value = el.scrollLeft < el.scrollWidth - el.clientWidth - 4
  const bounds = el.getBoundingClientRect()
  el.querySelectorAll<HTMLElement>('.epic-key').forEach(key => {
    const label = key.parentElement?.querySelector('.feature-label')?.getBoundingClientRect(), box = key.getBoundingClientRect()
    key.classList.toggle('gone', box.left < bounds.left + 28 || box.right > bounds.right - 28 || !!label && box.left < label.right + 8)
  })
}
async function center() {
  await nextTick()
  const el = rail.value, active = el?.querySelector<HTMLElement>('[data-selected="true"]')
  if (el && active) { const box = active.getBoundingClientRect(), parent = el.getBoundingClientRect(); el.scrollTo({ left: el.scrollLeft + box.left - parent.left - el.clientWidth / 2 + box.width / 2, behavior: reduced() ? 'instant' : 'smooth' }) }
  updateFades()
}
function select(id: string) {
  const wasSearch = search.value
  selectedId.value = id; search.value = false; query.value = ''; void center()
  if (wasSearch) void nextTick(() => dialog.value?.focus())
}
function step(direction: number, byFeature = false) {
  const all = store.tickets
  if (!all.length) return
  if (byFeature) {
    const starts = all.flatMap((t,i) => i === 0 || t.feature_node_id !== all[i-1]?.feature_node_id ? [i] : [])
    const group = starts.filter(i => i <= index.value).length - 1
    select(all[starts[(group + direction + starts.length) % starts.length]!]!.ticket_node_id)
  } else select(all[(index.value + direction + all.length) % all.length]!.ticket_node_id)
}
async function loadTicket() {
  const current = ticket.value, version = ++request
  node.value = undefined; screens.value = []; error.value = ''; screenIndex.value = 0; compare.value = false; zoom.value = false
  if (!current) return
  loading.value = true
  const data = await Promise.allSettled([getNode(current.ticket_node_id), ...(current.screen_node_ids ?? []).map(getNode)])
  if (version !== request) return
  const [ticketResult, ...screenResults] = data
  if (ticketResult?.status === 'fulfilled') node.value = ticketResult.value
  screens.value = screenResults.flatMap(r => r.status === 'fulfilled' ? [r.value] : [])
  if (data.some(r => r.status === 'rejected')) error.value = 'Some ticket details or linked screens could not be loaded.'
  compareIndex.value = screens.value.length > 1 ? 1 : 0; loading.value = false
}
watch(() => [selectedId.value, store.walker?.revision], loadTicket, { immediate: true })
watch(() => store.tickets, all => { if (!all.some(t => t.ticket_node_id === selectedId.value)) selectedId.value = all[0]?.ticket_node_id || '' })
watch(sheet, async open => { if (open) { await nextTick(); dialog.value?.querySelector<HTMLElement>('.walker-overlay .j-icon')?.focus() } })
watch(screenIndex, () => { compare.value = false; compareIndex.value = screens.value.findIndex((_,i) => i !== screenIndex.value) })
async function find() { search.value = !search.value; sheet.value = false; if (search.value) { await nextTick(); queryInput.value?.focus() } }
function closeOverlay() { search.value = false; sheet.value = false; dialog.value?.focus() }
function keys(event: KeyboardEvent) {
  if (event.metaKey || event.ctrlKey || event.altKey || event.isComposing) return
  const target = event.target as HTMLElement
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); if (search.value || sheet.value) closeOverlay(); else emit('close'); return }
  if (target.closest('input,textarea,select,[contenteditable="true"]')) return
  if (sheet.value || search.value) return
  const key = event.key.toLowerCase()
  if (key === 'arrowleft' || key === 'arrowright') step(key === 'arrowleft' ? -1 : 1, event.shiftKey)
  else if (key === 'arrowup' || key === 'arrowdown') { if (screens.value.length) screenIndex.value = (screenIndex.value + (key === 'arrowup' ? -1 : 1) + screens.value.length) % screens.value.length }
  else if (key === ' ') { if (target.closest('button,a')) return; if (ticket.value && editable.value) void store.toggleTicket(ticket.value.ticket_node_id) }
  else if (key === 'c') { if (screens.value.length > 1) compare.value = !compare.value }
  else if (key === 'z') zoom.value = !zoom.value
  else if (key === 'i') detail.value = !detail.value
  else if (key === '/') void find()
  else if (key === '?') sheet.value = !sheet.value
  else return
  event.preventDefault()
}
function pointerDown(event: PointerEvent) {
  const el = rail.value
  if (!el || event.button !== 0 || event.pointerType === 'touch' || el.scrollWidth <= el.clientWidth || (event.target as HTMLElement).closest('input,a,[role="checkbox"]')) return
  cancelAnimationFrame(frame); suppressClick = false; velocity = 0; lastX = event.clientX; lastTime = performance.now()
  down = { x: event.clientX, scroll: el.scrollLeft, pointer: event.pointerId }
}
function pointerMove(event: PointerEvent) {
  if (!down || !rail.value) return
  const dx = event.clientX - down.x
  if (!dragging.value && Math.abs(dx) < 5) return
  if (!dragging.value) rail.value.setPointerCapture(event.pointerId)
  dragging.value = true; suppressClick = true
  const now = performance.now()
  velocity = .7 * velocity + .3 * ((lastX - event.clientX) / Math.max(1, now - lastTime)); lastX = event.clientX; lastTime = now
  rail.value.scrollLeft = down.scroll - dx; updateFades(); event.preventDefault()
}
function pointerUp() {
  down = undefined
  if (!dragging.value) return
  dragging.value = false
  let speed = reduced() ? 0 : velocity * 16
  const coast = () => { const el = rail.value; if (!el || Math.abs(speed) < .4) return; const previous = el.scrollLeft; el.scrollLeft += speed; speed *= .93; updateFades(); if (el.scrollLeft !== previous) frame = requestAnimationFrame(coast) }
  frame = requestAnimationFrame(coast)
}
function suppress(event: MouseEvent) { if (suppressClick) { event.preventDefault(); event.stopPropagation(); suppressClick = false } }
function wheel(event: WheelEvent) {
  const el = rail.value
  if (el && el.scrollWidth > el.clientWidth && Math.abs(event.deltaY) > Math.abs(event.deltaX)) { el.scrollLeft += event.deltaY; event.preventDefault() }
}
onMounted(() => {
  originalFocus = document.activeElement as HTMLElement; dialog.value?.showModal(); dialog.value?.focus()
  observer = new ResizeObserver(updateFades); if (rail.value) observer.observe(rail.value)
  void center()
})
onBeforeUnmount(() => { request++; observer?.disconnect(); cancelAnimationFrame(frame); dialog.value?.close(); originalFocus?.focus() })
</script>
<template>
  <dialog ref="dialog" class="release-walker journey-dialog" aria-label="Release walker" tabindex="-1" @keydown="keys" @cancel.prevent="search || sheet ? closeOverlay() : $emit('close')">
    <header class="walker-header" :class="{ dragging }" :inert="search || sheet">
      <div class="release-count"><span class="eyebrow">{{ store.release?.title || 'Release' }}</span><b>{{ store.selected.length }} of {{ store.tickets.length }}</b></div>
      <div class="walker-arrows"><button class="j-icon" aria-label="Previous feature" title="Previous feature (Shift + Left)" :disabled="!ticket" @click="step(-1,true)"><JourneyIcon name="left" /></button><button class="j-icon" aria-label="Previous ticket" title="Previous ticket (Left)" :disabled="!ticket" @click="step(-1)"><JourneyIcon name="left" /></button></div>
      <div ref="rail" class="walker-rail" :class="{ 'fade-left': leftFade, 'fade-right': rightFade }" @scroll="updateFades" @pointerdown="pointerDown" @pointermove="pointerMove" @pointerup="pointerUp" @pointercancel="pointerUp" @lostpointercapture="pointerUp" @click.capture="suppress" @wheel="wheel" @dragstart.prevent>
        <section v-for="group in store.groups" :key="group.id" class="walker-group" :class="{ current: group.tickets.some(t => t.ticket_node_id === selectedId) }">
          <div class="feature-line"><span class="feature-label"><button v-if="group.tickets.length" class="j-check" role="checkbox" :aria-checked="featureState(group.tickets) === 'some' ? 'mixed' : featureState(group.tickets) === 'all'" :aria-label="`Select ${group.feature?.title || 'No feature'}: ${group.tickets.filter(t => t.included).length} of ${group.tickets.length}`" :disabled="!editable" @click="store.toggleFeature(group.id)"><JourneyIcon v-if="featureState(group.tickets) === 'all'" name="check" /><JourneyIcon v-else-if="featureState(group.tickets) === 'some'" name="minus" /></button><button class="feature-name" :disabled="!group.tickets.length" :title="group.feature?.title || 'No feature'" @click="select(group.tickets[0]!.ticket_node_id)">{{ group.feature?.title || 'No feature' }}</button></span><a v-if="group.feature" class="epic-key" :href="`?node=${encodeURIComponent(group.id)}`" @click.prevent="$emit('openNode',group.id)">{{ group.feature.epic_key }}<JourneyIcon name="external" /></a></div>
          <div class="ticket-chips"><span v-if="!group.tickets.length" class="no-tickets">No tickets</span><div v-for="item in group.tickets" :key="item.ticket_node_id" class="ticket-chip" :data-selected="item.ticket_node_id === selectedId" :class="{ deferred: !item.included }"><input type="checkbox" :checked="item.included" :disabled="!editable" :aria-label="`Include ${item.key}`" @click.prevent="store.toggleTicket(item.ticket_node_id)" /><button :aria-current="item.ticket_node_id === selectedId ? 'true' : undefined" :title="item.title" @click="select(item.ticket_node_id)">{{ item.key }}</button></div></div>
        </section>
      </div>
      <div class="walker-arrows"><button class="j-icon" aria-label="Next feature" title="Next feature (Shift + Right)" :disabled="!ticket" @click="step(1,true)"><JourneyIcon name="right" /></button><button class="j-icon" aria-label="Next ticket" title="Next ticket (Right)" :disabled="!ticket" @click="step(1)"><JourneyIcon name="right" /></button></div>
      <div class="walker-tools"><button class="j-icon" aria-label="Find a ticket" title="Find a ticket (/)" @click="find"><JourneyIcon name="search" /></button><button class="j-icon" aria-label="Compare screens" :aria-pressed="compare" :disabled="screens.length < 2" @click="compare = !compare"><JourneyIcon name="compare" /></button><button class="j-icon" aria-label="Zoom to 100 percent" :aria-pressed="zoom" @click="zoom = !zoom"><JourneyIcon name="zoom" /></button><button class="j-icon" aria-label="Ticket details" :aria-pressed="detail" @click="detail = !detail"><JourneyIcon name="info" /></button><button class="j-icon" aria-label="Shortcuts" @click="sheet = !sheet; search = false"><JourneyIcon name="keys" /></button><button class="j-icon" aria-label="Close walker" @click="$emit('close')"><JourneyIcon name="close" /></button></div>
    </header>
    <div v-if="store.error" class="walker-message error" role="alert">{{ store.error }} <button class="j-button" :disabled="store.loading || store.busy" @click="store.load()">Refresh</button></div>
    <div class="walker-body" :class="{ 'with-detail': detail && ticket }" :inert="search || sheet">
      <section class="screen-workspace" aria-label="Linked screens">
        <div class="screen-toolbar"><span v-if="ticket" class="j-meta">{{ ticket.key }} · {{ index + 1 }} / {{ store.tickets.length }}</span><span>{{ screen?.title || (loading ? 'Loading screens…' : 'No linked screen') }}</span><span class="j-meta">{{ zoom ? '100%' : 'Fit' }}</span><label v-if="compare" class="compare-select">Compare linked screen<select v-model="compareIndex"><option v-for="(other,i) in screens" :key="other.id" :value="i" :disabled="i === screenIndex">{{ other.key }} · {{ other.title }}</option></select></label></div>
        <div class="screen-canvas" :class="{ zoomed: zoom, comparing: compare }" :style="{ '--compare-position': `${comparePosition}%` }">
          <template v-if="screen"><article class="screen-page"><div class="eyebrow">{{ screen.key }} · {{ screen.state }}</div><h2>{{ screen.title }}</h2><MarkdownBody :body="screen.body" /></article><article v-if="compare && screens[compareIndex]" class="screen-page"><div class="eyebrow">{{ screens[compareIndex]!.key }} · {{ screens[compareIndex]!.state }}</div><h2>{{ screens[compareIndex]!.title }}</h2><MarkdownBody :body="screens[compareIndex]!.body" /></article></template>
          <div v-else class="screen-empty"><JourneyIcon name="expand" /><h2>{{ ticket ? 'No screen linked yet' : 'No tickets yet' }}</h2><p>{{ ticket ? 'This ticket has no available linked screen. Its description and evidence are in the details.' : 'Features stay empty until a breakdown is accepted.' }}</p><button v-if="error" class="j-button" @click="loadTicket">Retry details</button></div>
        </div>
        <label v-if="compare" class="compare-slider"><span class="j-meta">Comparison position</span><input v-model="comparePosition" type="range" min="0" max="100" /><span class="j-meta">{{ comparePosition }}%</span></label><div v-if="screens.length" class="screen-filmstrip" aria-label="Screen filmstrip"><button v-for="(item,i) in screens" :key="item.id" :aria-pressed="i === screenIndex" @click="screenIndex = i"><span class="screen-mini" aria-hidden="true">{{ item.body.slice(0,180) }}</span><span>{{ item.key }} · {{ item.title }}</span></button></div>
        <footer class="walker-footer"><label v-if="ticket"><input type="checkbox" :checked="ticket.included" :disabled="!editable" @click.prevent="store.toggleTicket(ticket.ticket_node_id)" />{{ ticket.included ? 'In this release' : 'Deferred to backlog' }}</label><span class="j-meta">Arrow keys · tickets · Shift + arrows · features</span></footer>
      </section>
      <TicketDetails v-if="detail && ticket" :ticket="ticket" :node="node" :feature="feature" :loading="loading" :error="error" @open-node="$emit('openNode',$event)" />
    </div>
    <section v-if="search" class="walker-overlay" aria-label="Find a ticket"><div class="overlay-card"><div class="j-card-head"><h2>Find a ticket</h2><button class="j-icon" aria-label="Close search" @click="closeOverlay"><JourneyIcon name="close" /></button></div><label class="j-label">Search tickets<input ref="queryInput" v-model="query" type="search" @keydown.enter.prevent="results[0] && select(results[0].ticket_node_id)" /></label><p v-if="!results.length">No matching tickets.</p><button v-for="item in results" :key="item.ticket_node_id" class="search-result" @click="select(item.ticket_node_id)"><span class="j-meta">{{ item.key }}</span><span>{{ item.title }}</span><span class="pick-dot" :class="{ included: item.included }" :aria-label="item.included ? 'In release' : 'Backlog'" /></button></div></section>
    <section v-if="sheet" class="walker-overlay" aria-label="Keyboard shortcuts" @click.self="closeOverlay"><div class="overlay-card"><div class="j-card-head"><h2>Shortcuts</h2><button class="j-icon" aria-label="Close shortcuts" @click="closeOverlay"><JourneyIcon name="close" /></button></div><dl class="shortcut-list"><dt><kbd>Left / Right</kbd></dt><dd>Previous / next ticket, wraps around</dd><dt><kbd>Shift + Left / Right</kbd></dt><dd>Previous / next feature</dd><dt><kbd>Up / Down</kbd></dt><dd>Screens of this ticket</dd><dt><kbd>Space</kbd></dt><dd>Include in release / defer to backlog</dd><dt><kbd>C</kbd></dt><dd>Compare linked screens</dd><dt><kbd>Z</kbd></dt><dd>Zoom to 100%</dd><dt><kbd>I</kbd></dt><dd>Show / hide details</dd><dt><kbd>/</kbd></dt><dd>Search tickets</dd><dt><kbd>?</kbd></dt><dd>This sheet</dd><dt><kbd>Esc</kbd></dt><dd>Close</dd></dl></div></section>
  </dialog>
</template>
<style scoped>
.release-walker { position:fixed; inset:0; width:100vw; max-width:none; height:100dvh; max-height:none; margin:0; padding:0; border:0; border-radius:0; background:var(--canvas); color:var(--ink); overflow:hidden; }
.release-walker[open] { display:flex; flex-direction:column; }.release-walker::backdrop { background:var(--canvas); }
.walker-header { display:flex; align-items:center; gap:8px; flex:none; height:90px; padding:12px 16px; border-bottom:1px solid var(--line); background:var(--glass); }
.release-count { display:flex; flex-direction:column; flex:none; max-width:130px; padding-right:8px; }.release-count .eyebrow { overflow:hidden; white-space:nowrap; text-overflow:ellipsis; }.release-count b { font-size:14px; }.walker-arrows { display:grid; gap:4px; flex:none; }.walker-arrows .j-icon { width:26px; height:26px; }
.walker-rail { display:flex; gap:24px; min-width:0; flex:1; overflow:auto hidden; scrollbar-width:none; user-select:none; touch-action:pan-x; cursor:grab; padding:4px 28px; }.walker-rail::-webkit-scrollbar { display:none; }.dragging .walker-rail,.dragging .walker-rail * { cursor:grabbing; }.fade-left { mask-image:linear-gradient(90deg,transparent,#000 28px); }.fade-right { mask-image:linear-gradient(270deg,transparent,#000 28px); }.fade-left.fade-right { mask-image:linear-gradient(90deg,transparent,#000 28px,#000 calc(100% - 28px),transparent); }
.walker-group { flex:none; min-width:220px; }.feature-line { display:flex; align-items:center; gap:20px; height:22px; font-size:12px; color:var(--ink-2); }.feature-label { display:flex; gap:7px; align-items:center; min-width:0; }.feature-name { border:0; background:none; padding:0; max-width:210px; text-align:left; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; font:inherit; }.feature-name:hover { color:var(--teal); }.current .feature-name { color:var(--teal-ink); font-weight:600; }.epic-key { display:inline-flex; gap:4px; align-items:center; margin-left:auto; font:10px var(--mono); white-space:nowrap; }.epic-key svg { width:11px; height:11px; }.epic-key.gone { visibility:hidden; }.feature-line .j-check { width:16px; height:16px; }.feature-line .j-check svg { width:12px; height:12px; }
.ticket-chips { display:flex; gap:6px; margin-top:8px; }.ticket-chip { display:flex; align-items:center; gap:4px; border:1px solid var(--line-2); background:var(--glass); border-radius:999px; padding:3px 9px; height:28px; }.ticket-chip[data-selected="true"] { background:var(--aqua-3); border-color:var(--teal); color:var(--teal-ink); box-shadow:0 0 12px var(--glass-rim); }.ticket-chip button { background:none; border:0; padding:0 3px; font:11px var(--mono); white-space:nowrap; }.ticket-chip input { width:13px; height:13px; margin:0; accent-color:var(--teal); }.ticket-chip.deferred { border-style:dashed; color:var(--ink-2); }.no-tickets { display:inline-flex; align-items:center; height:28px; border:1px dashed var(--line-2); border-radius:999px; padding:0 12px; font-size:12px; color:var(--ink-2); }.walker-tools { display:flex; gap:3px; flex:none; }.walker-tools .j-icon { width:30px; height:34px; }
.walker-body { display:grid; grid-template-columns:minmax(0,1fr); min-height:0; flex:1; }.walker-body.with-detail { grid-template-columns:minmax(0,1fr) 310px; }.screen-workspace { min-width:0; min-height:0; display:flex; flex-direction:column; }.screen-toolbar { min-height:42px; padding:8px 22px; display:flex; align-items:center; gap:16px; border-bottom:1px solid var(--line); font-size:12px; }.screen-toolbar>span:nth-child(2) { flex:1; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.screen-canvas { flex:1; min-height:0; overflow:auto; padding:28px; background:radial-gradient(ellipse at center,var(--aqua-3),var(--canvas) 75%); display:flex; gap:18px; align-items:flex-start; justify-content:center; }.screen-page { background:var(--surface); border:1px solid var(--glass-edge); box-shadow:var(--shadow); border-radius:8px; padding:24px; width:min(100%,760px); min-height:100%; flex-shrink:0; overflow-wrap:anywhere; }.screen-page h2 { margin:12px 0; font-size:22px; }.screen-canvas.comparing { display:grid; grid-template-columns:minmax(0,1fr); align-content:start; }.comparing .screen-page { grid-area:1 / 1; width:100%; }.comparing .screen-page:nth-child(2) { clip-path:inset(0 0 0 var(--compare-position)); background:var(--surface); }.compare-slider { display:flex; align-items:center; gap:12px; padding:8px 20px; border-top:1px solid var(--line); }.compare-slider input { flex:1; accent-color:var(--teal); }.compare-select { display:flex; align-items:center; gap:6px; font-size:11px; }.compare-select select { width:150px; background:var(--surface); color:var(--ink); border:1px solid var(--line-2); border-radius:6px; padding:4px; }.zoomed { justify-content:flex-start; }.zoomed .screen-page { width:960px; }.screen-empty { align-self:center; text-align:center; display:flex; align-items:center; flex-direction:column; gap:12px; max-width:440px; }.screen-empty>svg { width:36px; height:36px; color:var(--ink-3); }.screen-empty h2 { font-size:20px; font-weight:400; }.screen-empty p { font-size:13px; }
.screen-filmstrip { display:flex; justify-content:safe center; gap:10px; overflow:auto; flex:none; padding:10px 18px; border-top:1px solid var(--line); }.screen-filmstrip button { width:140px; flex:none; padding:6px; border:1px solid var(--line); border-radius:8px; background:var(--glass); display:flex; flex-direction:column; gap:6px; text-align:left; }.screen-filmstrip button[aria-pressed="true"] { border-color:var(--teal); background:var(--aqua-3); }.screen-filmstrip button>span:last-child { font:10px var(--mono); width:100%; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }.screen-mini { display:block; overflow:hidden; font:5px/1.5 var(--font); white-space:pre-wrap; padding:8px; width:100%; height:40px; background:var(--surface); border-radius:3px; }.screen-mini span { height:2px; background:var(--line-2); }.screen-mini span:last-child { width:55%; }
.walker-footer { display:flex; align-items:center; justify-content:space-between; gap:10px; padding:10px 20px; border-top:1px solid var(--line); flex:none; font-size:12px; }.walker-footer label { display:flex; align-items:center; gap:7px; }.walker-overlay { position:absolute; inset:90px 0 0; background:color-mix(in srgb,var(--canvas) 65%,transparent); display:flex; justify-content:center; align-items:flex-start; padding:24px; backdrop-filter:blur(8px); }.overlay-card { background:var(--surface); border:1px solid var(--glass-rim); box-shadow:var(--shadow); border-radius:var(--radius); padding:20px; width:min(620px,100%); max-height:100%; overflow:auto; }.search-result { display:flex; align-items:center; gap:14px; width:100%; text-align:left; padding:12px 0; border:0; border-bottom:1px solid var(--line); background:none; font-size:13px; }.search-result span:nth-child(2) { flex:1; }.search-result:hover { color:var(--teal); }.pick-dot { width:8px; height:8px; border:1px solid var(--ink-3); border-radius:50%; }.pick-dot.included { background:var(--teal); border-color:var(--teal); }.shortcut-list { display:grid; grid-template-columns:auto 1fr; gap:13px 24px; font-size:13px; }.shortcut-list dd { margin:0; color:var(--ink-2); }kbd { font:11px var(--mono); background:var(--surface-2); border:1px solid var(--line-2); padding:3px 6px; border-radius:4px; }.walker-message { flex:none; padding:8px 20px; }
@media(max-width:800px) { .walker-body.with-detail { grid-template-columns:minmax(0,1fr) 250px; }.release-count { display:none; }.walker-header { padding-inline:8px; }.walker-tools { flex-wrap:wrap; max-width:100px; }.screen-canvas { padding:12px; }.walker-footer .j-meta { display:none; } }
@media(max-width:560px) { .walker-body.with-detail { grid-template-columns:minmax(0,1fr); }.walker-body.with-detail .screen-workspace { display:none; }.walker-body.with-detail :deep(.ticket-details) { border-left:0; }.shortcut-list { grid-template-columns:1fr; gap:8px; }.shortcut-list dd { margin-bottom:8px; } }
</style>
