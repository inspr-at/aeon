<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listNodes, searchNodes, type ListItem, type WorkNode } from '../lib/api'
import { run } from '../lib/commands'
import { actionResults, assemble, keyPrefixOf, keyQuery, projectResults, recentResults, ticketResults, type ActionResult, type Group, type Result, type TicketResult } from '../lib/palette'
import { recents } from '../lib/recents'
import { dark, toggleTheme } from '../lib/theme'
import { toast } from '../lib/toast'
import { kinds } from '../lib/useTicket'
import { highlight, statusMeta } from '../lib/work'
import { useProjects } from '../stores/projects'
import AppIcon, { type IconName } from './AppIcon.vue'
import BizIcon, { type BizIconName } from './business/BizIcon.vue'
import { useBusiness } from '../stores/business'
import StatusIcon from './work/StatusIcon.vue'

// One input for tickets, projects and actions. Inside a project the search is scoped
// to it (a chip that Backspace on an empty input removes); an empty input shows what
// was opened recently. Stale requests are cancelled; results never jump while typing.
const props = defineProps<{ canWrite: boolean }>()
const route = useRoute()
const router = useRouter()
const projects = useProjects()
const business = useBusiness()
const dialog = ref<HTMLDialogElement>()
const input = ref<HTMLInputElement>()
const list = ref<HTMLElement>()
const term = ref('')
const scope = ref<string | null>(null)
const active = ref(0)
const loading = ref(false)
const failed = ref('')
const listed = ref<ListItem[]>([])
const hits = ref<WorkNode[]>([])
const workKinds = ref(new Map<string, string>())
const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)
// Phones get a shorter placeholder and a Cancel button in place of the Esc keycap.
const narrowQuery = window.matchMedia('(max-width: 600px)')
const narrow = ref(narrowQuery.matches)
const onNarrow = (event: MediaQueryListEvent) => { narrow.value = event.matches }
narrowQuery.addEventListener('change', onNarrow)
let opener: HTMLElement | null = null
let timer: ReturnType<typeof setTimeout> | undefined
let controller: AbortController | null = null
let searched = ''

const routeProject = computed(() => typeof route.params.projectKey === 'string' ? projects.byRouteKey(route.params.projectKey) : undefined)
const scopeProject = computed(() => scope.value ? projects.byRouteKey(scope.value) : undefined)
const query = computed(() => term.value.trim())

// ---------- Actions ----------
const actions = computed<ActionResult[]>(() => {
  const here = routeProject.value
  const out: ActionResult[] = []
  if (here && props.canWrite) out.push({ type: 'action', id: 'new-ticket', label: `New ticket in ${here.routeKey}`, hint: here.title, icon: 'plus', keys: ['n'] })
  if (here) {
    const outline = route.query.view === 'outline'
    out.push({ type: 'action', id: outline ? 'go-list' : 'go-outline', label: outline ? 'Go to List' : 'Go to Outline', hint: here.title, icon: outline ? 'list' : 'outline' })
  }
  if (route.path !== '/') out.push({ type: 'action', id: 'go-projects', label: 'Go to Projects', icon: 'folder' })
  if (!route.path.startsWith('/agents')) out.push({ type: 'action', id: 'go-agents', label: 'Go to Agents', hint: 'Sessions, approvals and pacing', icon: 'agent' })
  // Business: log time (on the open ticket, when one is open) and the overview.
  if (business.open.hours && business.staff) {
    const ticket = typeof route.params.ticketKey === 'string' ? route.params.ticketKey.toUpperCase() : ''
    out.push({ type: 'action', id: 'log-time', label: ticket ? `Log time on ${ticket}` : 'Log time', hint: 'Hours this week', icon: 'clock', keys: route.path.startsWith('/business/hours') ? ['l'] : undefined })
  }
  if (business.anyOpen && route.path !== '/business') out.push({ type: 'action', id: 'go-business', label: 'Go to Business', hint: 'Hours and rates', icon: 'briefcase' })
  out.push({ type: 'action', id: 'theme', label: dark.value ? 'Switch to light theme' : 'Switch to dark theme', icon: dark.value ? 'sun' : 'moon' })
  out.push({ type: 'action', id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', keys: ['?'] })
  return out
})

// ---------- Results ----------
function projectFor(key: string) {
  const project = projects.byRouteKey(keyPrefixOf(key))
  return project ? project.routeKey : null
}
const groups = computed<Group[]>(() => assemble(query.value, {
  recent: recentResults(recents, scope.value),
  tickets: ticketResults(searched, listed.value, hits.value, workKinds.value, projectFor, scope.value),
  projects: scope.value ? [] : projectResults(query.value, projects.projects.map(p => ({ id: p.id, routeKey: p.routeKey, title: p.title, description: p.description, archived: p.archived }))),
  actions: actionResults(query.value, actions.value),
}))
const flat = computed(() => groups.value.flatMap(group => group.items))
const showSkeleton = computed(() => loading.value && !!query.value && !listed.value.length && !hits.value.length)
const empty = computed(() => !!query.value && !loading.value && !failed.value && searched === query.value && !flat.value.length)
watch(flat, () => { if (active.value >= flat.value.length) active.value = 0 })

async function search() {
  const q = query.value
  controller?.abort()
  if (!q) { listed.value = []; hits.value = []; loading.value = false; searched = ''; return }
  controller = new AbortController()
  const signal = controller.signal
  loading.value = true; failed.value = ''
  const within = scopeProject.value?.id
  const key = keyQuery(q)
  try {
    if (!workKinds.value.size) workKinds.value = new Map((await kinds()).filter(k => ['ticket', 'task', 'epic'].includes(k.slug)).map(k => [k.id, k.slug]))
    // A key prefix ("PHAROS-29") is a key lookup; words also go to the hybrid search.
    const [page, found] = await Promise.all([
      listNodes({ q, kind: ['ticket', 'task', 'epic'], within, sort: key ? 'key' : '-updated_at', limit: 8 }, { signal }),
      key ? Promise.resolve({ items: [] }) : searchNodes(q, { limit: 12 }, { signal }).catch(() => ({ items: [] })),
    ])
    if (signal.aborted) return
    listed.value = page.items
    hits.value = found.items.map(hit => hit.node)
    searched = q
    active.value = 0
  } catch (e) {
    if (signal.aborted) return
    failed.value = e instanceof Error ? e.message : 'Search is unavailable'
  } finally {
    if (!signal.aborted) loading.value = false
  }
}
watch([term, scope], () => {
  clearTimeout(timer)
  if (!query.value) { void search(); active.value = 0; return }
  loading.value = true
  timer = setTimeout(() => void search(), 120)
})

// ---------- Open, close, choose ----------
async function open() {
  if (dialog.value?.open) { input.value?.select(); return }
  opener = document.activeElement as HTMLElement
  scope.value = routeProject.value?.routeKey ?? null
  term.value = ''; listed.value = []; hits.value = []; failed.value = ''; active.value = 0; searched = ''
  void projects.load()
  dialog.value?.showModal()
  await nextTick()
  input.value?.focus()
}
function close() { controller?.abort(); dialog.value?.close(); opener?.focus({ preventScroll: true }) }
function hrefOf(result: Result): string | null {
  if (result.type === 'project') return `/p/${encodeURIComponent(result.key)}`
  if (result.type === 'ticket' && result.projectKey) return `/p/${encodeURIComponent(result.projectKey)}/${encodeURIComponent(result.key)}`
  return null
}
async function resolveTicket(result: TicketResult): Promise<string | null> {
  if (result.projectKey) return hrefOf(result)
  try {
    const page = await listNodes({ q: result.key, limit: 10 })
    const owner = page.items.find(item => item.key === result.key)?.project
    const project = owner ? projects.byId(owner.id) : undefined
    return project ? `/p/${encodeURIComponent(project.routeKey)}/${encodeURIComponent(result.key)}` : null
  } catch { return null }
}
function act(id: string) {
  const here = routeProject.value
  if (id === 'new-ticket' && here) run({ name: 'new-ticket', projectKey: here.routeKey })
  else if (id === 'go-outline' || id === 'go-list') {
    const { view: _view, ...rest } = route.query
    void router.push({ path: here ? `/p/${encodeURIComponent(here.routeKey)}` : route.path, query: id === 'go-outline' ? { ...rest, view: 'outline' } : rest })
  } else if (id === 'go-projects') void router.push('/')
  else if (id === 'go-agents') void router.push('/agents')
  else if (id === 'go-business') void router.push('/business')
  else if (id === 'log-time') {
    const ticket = typeof route.params.ticketKey === 'string' ? route.params.ticketKey.toUpperCase() : ''
    void router.push({ path: '/business/hours', query: ticket ? { log: '1', ticket } : { log: '1' } })
  }
  else if (id === 'theme') toggleTheme()
  else if (id === 'shortcuts') run({ name: 'shortcuts' })
}
async function choose(result: Result | undefined, newTab = false) {
  if (!result) return
  if (result.type === 'action') { close(); act(result.id); return }
  const href = result.type === 'ticket' ? await resolveTicket(result) : hrefOf(result)
  if (!href) { toast(`${result.key} is not part of a project, so it has no list to open in.`); return }
  if (newTab) { window.open(href, '_blank', 'noopener'); return }
  close()
  await router.push(href)
}

// ---------- Keyboard ----------
function move(step: number) {
  if (!flat.value.length) return
  active.value = (active.value + step + flat.value.length) % flat.value.length
  void nextTick(() => list.value?.querySelector(`[data-index="${active.value}"]`)?.scrollIntoView({ block: 'nearest' }))
}
function jumpGroup(step: number) {
  const starts: number[] = []
  let index = 0
  for (const group of groups.value) { starts.push(index); index += group.items.length }
  if (starts.length < 2) return
  const current = starts.reduce((found, start, i) => active.value >= start ? i : found, 0)
  active.value = starts[(current + step + starts.length) % starts.length]
  void nextTick(() => list.value?.querySelector(`[data-index="${active.value}"]`)?.scrollIntoView({ block: 'nearest' }))
}
function keydown(event: KeyboardEvent) {
  const ctrl = event.ctrlKey && !event.metaKey
  // Ctrl+J/K/N/P move like arrows while typing; they stop here so Ctrl+K does not reopen the palette.
  if (ctrl && ['j', 'k', 'n', 'p'].includes(event.key)) event.stopPropagation()
  if (event.key === 'ArrowDown' || (ctrl && (event.key === 'j' || event.key === 'n'))) { event.preventDefault(); move(1) }
  else if (event.key === 'ArrowUp' || (ctrl && (event.key === 'k' || event.key === 'p'))) { event.preventDefault(); move(-1) }
  else if (event.key === 'Tab') { event.preventDefault(); jumpGroup(event.shiftKey ? -1 : 1) }
  else if (event.key === 'Enter') { event.preventDefault(); void choose(flat.value[active.value], event.metaKey || event.ctrlKey) }
  else if (event.key === 'Backspace' && !term.value && scope.value) { event.preventDefault(); scope.value = null }
}
function backdrop(event: MouseEvent) { if (event.target === dialog.value) close() }
function indexOf(result: Result) { return flat.value.indexOf(result) }
onBeforeUnmount(() => { clearTimeout(timer); controller?.abort(); narrowQuery.removeEventListener('change', onNarrow) })
defineExpose({ open })
const iconOf = (result: Result): BizIconName => result.type === 'action' ? result.icon as BizIconName : result.type === 'project' ? 'folder' : result.kind === 'epic' ? 'epic' : result.kind === 'task' ? 'task' : 'ticket'
</script>

<template>
  <dialog ref="dialog" class="palette" aria-label="Search and commands" @cancel.prevent="close" @click="backdrop">
    <div class="sheet">
      <label class="input-row">
        <AppIcon name="search" :size="18" class="lead" />
        <span v-if="scope" class="scope-chip">
          <span class="scope-in">in</span><span class="scope-key">{{ scope }}</span>
          <button type="button" class="scope-x" :aria-label="`Search everywhere, not only in ${scope}`" data-tip="Search everywhere · Backspace" @click="scope = null; input?.focus()"><AppIcon name="close" :size="11" /></button>
        </span>
        <input
          ref="input" v-model="term" class="palette-input" role="combobox" aria-controls="palette-results" aria-autocomplete="list" :aria-expanded="true"
          :aria-activedescendant="flat.length ? `palette-item-${active}` : undefined" :aria-label="scope ? `Search in ${scope}` : 'Search tickets, projects and actions'"
          :placeholder="narrow ? (scope ? `Search ${scopeProject?.title ?? scope}` : 'Search everything') : scope ? `Search ${scopeProject?.title ?? scope}, or a key like ${scope}-12` : 'Search tickets, projects and actions'" autocomplete="off" spellcheck="false" @keydown="keydown"
        />
        <span v-if="loading && query" class="spinner" aria-hidden="true" />
        <kbd class="keycap esc" aria-hidden="true">esc</kbd>
      </label>
      <button type="button" class="cancel" @click="close">Cancel</button>

      <div id="palette-results" ref="list" class="results" role="listbox" :aria-label="query ? 'Results' : 'Recent and actions'" :class="{ stale: loading && !showSkeleton }">
        <div v-if="showSkeleton" class="skeleton-lines" aria-hidden="true">
          <span v-for="i in 5" :key="i" class="line"><span class="skeleton dot" /><span class="skeleton key-sk" /><span class="skeleton" :style="{ width: `${34 + ((i * 29) % 40)}%` }" /></span>
        </div>
        <div v-else-if="failed" class="state" role="alert">
          <AppIcon name="alert" :size="18" class="state-icon danger" />
          <p><strong>Search is not answering.</strong> {{ failed }}</p>
          <button type="button" class="btn sm" @click="search()">Try again</button>
        </div>
        <div v-else-if="empty" class="state">
          <AppIcon name="search" :size="18" class="state-icon" />
          <p><strong>Nothing matches “{{ query }}”{{ scope ? ` in ${scope}` : '' }}.</strong></p>
          <p class="hint">Try a key like <kbd class="keycap wide">{{ scope ?? 'PHAROS' }}-296</kbd> or a few words from a title.</p>
          <button v-if="scope" type="button" class="btn sm" @click="scope = null; input?.focus()">Search everywhere</button>
        </div>
        <template v-else>
          <section v-for="group in groups" :key="group.id" class="group" role="group" :aria-label="group.label">
            <p class="group-label eyebrow" aria-hidden="true">{{ group.label }}</p>
            <div
              v-for="result in group.items" :id="`palette-item-${indexOf(result)}`" :key="result.id" class="item" :class="[result.type, { active: indexOf(result) === active }]"
              role="option" :aria-selected="indexOf(result) === active" :data-index="indexOf(result)" @pointermove="active = indexOf(result)" @click="choose(result, $event.metaKey || $event.ctrlKey)"
            >
              <template v-if="result.type === 'ticket'">
                <StatusIcon :state="result.state" :size="13" class="st" :data-tip="statusMeta(result.state).label" />
                <span class="key"><template v-for="(part, i) in highlight(result.key, group.id === 'recent' || !keyQuery(query) ? '' : query)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span>
                <AppIcon :name="iconOf(result) as IconName" :size="13" class="kind" :class="result.kind" />
                <span class="title"><template v-for="(part, i) in highlight(result.title, group.id === 'recent' ? '' : query)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span>
                <span v-if="result.projectKey && result.projectKey !== scope" class="project-chip">{{ result.projectKey }}</span>
              </template>
              <template v-else-if="result.type === 'project'">
                <span class="project-mark"><AppIcon name="folder" :size="13" /></span>
                <span class="key-badge"><template v-for="(part, i) in highlight(result.key, group.id === 'recent' ? '' : query)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span>
                <span class="title">{{ result.title }}</span>
                <span v-if="result.description" class="desc">{{ result.description }}</span>
              </template>
              <template v-else>
                <span class="action-mark"><BizIcon :name="iconOf(result)" :size="14" /></span>
                <span class="title">{{ result.label }}</span>
                <span v-if="result.hint" class="desc">{{ result.hint }}</span>
                <span v-if="result.keys" class="item-keys" aria-hidden="true"><kbd v-for="k in result.keys" :key="k" class="keycap">{{ k }}</kbd></span>
              </template>
              <AppIcon v-if="indexOf(result) === active" name="enter" :size="13" class="enter" />
            </div>
          </section>
          <p v-if="!query && !groups.some(g => g.id === 'recent')" class="first-hint">Tickets and projects you open show up here.</p>
        </template>
      </div>

      <footer class="foot">
        <span><kbd class="keycap"><AppIcon name="arrow-up" /></kbd><kbd class="keycap"><AppIcon name="arrow-down" /></kbd> move</span>
        <span><kbd class="keycap"><AppIcon name="enter" /></kbd> open</span>
        <span><kbd class="keycap">{{ mac ? '⌘' : 'Ctrl' }}</kbd><kbd class="keycap"><AppIcon name="enter" /></kbd> new tab</span>
        <span><kbd class="keycap">tab</kbd> next group</span>
        <span v-if="scope" class="scope-hint"><kbd class="keycap wide">⌫</kbd> all projects</span>
      </footer>
    </div>
  </dialog>
</template>

<style scoped>
.palette { width: min(680px, calc(100vw - 24px)); max-width: none; margin: 11dvh auto auto; padding: 0; border: 0; background: transparent; color: var(--ink); overflow: visible; }
.palette::backdrop { background: var(--palette-scrim); backdrop-filter: blur(3px) saturate(1.05); -webkit-backdrop-filter: blur(3px) saturate(1.05); }
.sheet {
  display: flex; flex-direction: column; border-radius: 18px; border: 1px solid var(--glass-edge); overflow: hidden;
  /* Near-opaque: the list behind must never read through the results. */
  background: linear-gradient(165deg, var(--surface-raised), var(--surface-raised-2) 70%) var(--surface-raised); box-shadow: var(--shadow-pop), var(--shadow);
  backdrop-filter: blur(22px) saturate(1.2); -webkit-backdrop-filter: blur(22px) saturate(1.2);
}
@media (prefers-reduced-motion: no-preference) {
  .palette[open] .sheet { animation: palette-in .18s cubic-bezier(.2, .7, .2, 1); }
  .palette[open]::backdrop { animation: scrim-in .18s ease; }
  @keyframes palette-in { from { opacity: 0; transform: translateY(-6px) scale(.985); } to { opacity: 1; transform: none; } }
  @keyframes scrim-in { from { opacity: 0; } to { opacity: 1; } }
}
.input-row { display: flex; align-items: center; gap: 10px; height: 60px; padding: 0 14px 0 18px; border-bottom: 1px solid var(--line); }
.lead { color: var(--ink-3); }
.palette-input { flex: 1; min-width: 0; height: 100%; border: 0; background: transparent; color: var(--ink); font-size: 16.5px; }
.palette-input:focus { box-shadow: none; }
.palette-input::placeholder { color: var(--ink-3); }
.scope-chip { display: inline-flex; align-items: center; gap: 5px; flex-shrink: 0; height: 26px; padding: 0 3px 0 9px; border-radius: 999px; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.scope-in { font-size: 12px; color: var(--ink-2); }
.scope-key { font: 600 11.5px/1 var(--mono); letter-spacing: .04em; font-variant-ligatures: none; }
.scope-x { display: grid; place-items: center; width: 20px; height: 20px; padding: 0; border: 0; border-radius: 50%; background: transparent; color: var(--teal-ink); }
.scope-x:hover { background: rgba(14, 111, 108, .12); }
.scope-x:focus-visible { box-shadow: var(--focus-ring); }
.esc { flex-shrink: 0; }
.sheet { position: relative; }
.cancel { display: none; }
.spinner { width: 14px; height: 14px; flex-shrink: 0; border-radius: 50%; border: 1.8px solid var(--line-2); border-top-color: var(--teal); }
@media (prefers-reduced-motion: no-preference) { .spinner { animation: spin .8s linear infinite; } @keyframes spin { to { transform: rotate(360deg); } } }
/* A steady height: results change in place rather than resizing the sheet. */
.results { height: min(420px, 56dvh); overflow: auto; padding: 6px; overscroll-behavior: contain; }
.results.stale .group { opacity: .6; }
@media (prefers-reduced-motion: no-preference) { .results .group { transition: opacity .12s ease; } }
.group + .group { margin-top: 4px; }
.group-label { padding: 8px 12px 4px; margin: 0; }
.item { display: flex; align-items: center; gap: 10px; min-height: 38px; padding: 0 12px; border-radius: 10px; cursor: pointer; font-size: 14px; }
.item.active { background: var(--row-selected); box-shadow: inset 0 0 0 1px var(--glass-rim); }
.key { flex-shrink: 0; width: 96px; overflow: hidden; text-overflow: ellipsis; font: 500 11.5px/1 var(--mono); color: var(--ink-2); font-variant-ligatures: none; }
.item.active .key { color: var(--teal-ink); }
.kind { flex-shrink: 0; color: var(--ink-3); }
.kind.epic { color: var(--gold); }
.title { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.desc { flex: 1 1 0; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12.5px; color: var(--ink-3); }
.item.ticket .title { flex: 1 1 auto; }
.project-chip { flex-shrink: 0; height: 20px; padding: 0 8px; border-radius: 6px; background: var(--code-bg); color: var(--ink-2); font: 500 10.5px/20px var(--mono); letter-spacing: .04em; font-variant-ligatures: none; }
.project-mark, .action-mark { display: grid; place-items: center; flex-shrink: 0; width: 22px; height: 22px; border-radius: 7px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); color: var(--ink-2); }
.item.active .action-mark, .item.active .project-mark { color: var(--teal-ink); box-shadow: inset 0 0 0 1px var(--chip-teal-line); }
.item .key-badge { flex-shrink: 0; }
.item-keys { display: inline-flex; gap: 3px; margin-left: auto; }
.enter { flex-shrink: 0; margin-left: 4px; color: var(--ink-3); }
.item-keys + .enter { margin-left: 8px; }
.item:not(.ticket) .desc + .enter, .item:not(.ticket) .title + .enter { margin-left: auto; }
.first-hint { padding: 10px 12px; font-size: 12.5px; color: var(--ink-3); }
.skeleton-lines { display: grid; gap: 2px; padding: 8px 6px; }
.line { display: flex; align-items: center; gap: 12px; height: 38px; padding: 0 6px; }
.line .dot { width: 13px; height: 13px; border-radius: 50%; flex-shrink: 0; }
.line .key-sk { width: 76px; flex-shrink: 0; }
.state { display: grid; justify-items: center; align-content: center; gap: 8px; height: 100%; padding: 24px; text-align: center; font-size: 13.5px; color: var(--ink-2); }
.state strong { color: var(--ink); font-weight: 600; }
.state .hint { display: inline-flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 5px; }
.state-icon { color: var(--teal); }
.state-icon.danger { color: var(--danger); }
.keycap.wide { padding: 0 6px; }
.foot { display: flex; flex-wrap: wrap; gap: 6px 16px; padding: 10px 18px; border-top: 1px solid var(--line); font-size: 12px; color: var(--ink-3); }
.foot span { display: inline-flex; align-items: center; gap: 4px; }
.scope-hint { margin-left: auto; }
@media (max-width: 600px) {
  .palette { width: calc(100vw - 16px); margin-top: 8px; }
  .input-row { height: 56px; padding: 0 10px 0 14px; }
  .esc { display: none; }
  .results { height: min(460px, 62dvh); }
  .item { min-height: 44px; }
  .desc { display: none; }
  .foot { display: none; }
  .input-row { padding-right: 76px; }
  .cancel { display: grid; place-items: center; position: absolute; top: 6px; right: 6px; height: 44px; padding: 0 12px; border: 0; border-radius: 10px; background: transparent; color: var(--teal-ink); font-weight: 600; font-size: 14.5px; }
  .cancel:focus-visible { box-shadow: var(--focus-ring); }
  /* Two lines on a phone: key, kind and project above; the title gets the full width. */
  .item.ticket {
    display: grid; grid-template-columns: 14px auto 14px minmax(0, 1fr) auto; grid-template-areas: "st key kind . chip" ". ttl ttl ttl ttl";
    align-items: center; column-gap: 8px; row-gap: 4px; padding: 9px 12px;
  }
  .item.ticket .st { grid-area: st; }
  .item.ticket .key { grid-area: key; width: auto; }
  .item.ticket .kind { grid-area: kind; }
  .item.ticket .project-chip { grid-area: chip; }
  .item.ticket .title { grid-area: ttl; white-space: normal; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; line-height: 1.35; }
  .item.ticket .enter { display: none; }
}
</style>
