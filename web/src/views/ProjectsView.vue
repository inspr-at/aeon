<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useProjects, type Project } from '../stores/projects'
import { useSession } from '../stores/session'
import { absoluteTime, highlight, plural, relativeTime } from '../lib/work'
import AppIcon from '../components/AppIcon.vue'
import FloatingPanel from '../components/work/FloatingPanel.vue'
import StatusIcon from '../components/work/StatusIcon.vue'

type SortKey = 'activity' | 'name' | 'open' | 'progress'
const SORTS: { value: SortKey; label: string }[] = [
  { value: 'activity', label: 'Last activity' },
  { value: 'name', label: 'Name' },
  { value: 'open', label: 'Open tickets' },
  { value: 'progress', label: 'Progress' },
]

const store = useProjects()
const session = useSession()
const route = useRoute()
const router = useRouter()
const term = ref('')
const showArchived = ref(false)
const search = ref<HTMLInputElement>()
const list = ref<HTMLElement>()
const sortAnchor = ref<HTMLElement | null>(null)
const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | undefined

const sort = computed<SortKey>(() => SORTS.some(s => s.value === route.query.sort) ? route.query.sort as SortKey : 'activity')
const sortLabel = computed(() => SORTS.find(s => s.value === sort.value)!.label)
function percent(project: Project) { return project.total ? Math.round((project.done / project.total) * 100) : 0 }
const compare: Record<SortKey, (a: Project, b: Project) => number> = {
  activity: (a, b) => Date.parse(b.last_activity) - Date.parse(a.last_activity),
  name: (a, b) => a.title.localeCompare(b.title, 'en', { sensitivity: 'base' }),
  open: (a, b) => (b.open + b.in_progress) - (a.open + a.in_progress),
  progress: (a, b) => percent(b) - percent(a) || b.total - a.total,
}
const archivedCount = computed(() => store.projects.filter(project => project.archived).length)
const visible = computed(() => {
  const needle = term.value.trim().toLowerCase()
  return store.projects
    .filter(project => showArchived.value || !project.archived)
    .filter(project => !needle || `${project.routeKey} ${project.title} ${project.description}`.toLowerCase().includes(needle))
    .sort((a, b) => compare[sort.value](a, b) || compare.activity(a, b))
})
const active = computed(() => store.projects.filter(project => !project.archived))
const openTotal = computed(() => active.value.reduce((sum, project) => sum + project.open + project.in_progress, 0))
function to(project: Project) { return `/p/${encodeURIComponent(project.routeKey)}` }
function chooseSort(value: SortKey) {
  sortAnchor.value = null
  void router.replace({ query: { ...route.query, sort: value === 'activity' ? undefined : value } })
}
function closeSort(restore: boolean) {
  const anchor = sortAnchor.value
  sortAnchor.value = null
  if (restore) anchor?.focus()
}

function rows() { return [...(list.value?.querySelectorAll<HTMLAnchorElement>('.project-row') ?? [])] }
function move(step: number) {
  const items = rows()
  if (!items.length) return
  const index = items.indexOf(document.activeElement as HTMLAnchorElement)
  const next = index === -1 ? (step > 0 ? 0 : items.length - 1) : Math.max(0, Math.min(items.length - 1, index + step))
  items[next].focus()
  items[next].scrollIntoView({ block: 'nearest' })
}
function typing(target: EventTarget | null) {
  return target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
}
function keydown(event: KeyboardEvent) {
  if (event.metaKey || event.ctrlKey || event.altKey || document.querySelector('dialog[open], .floating')) return
  if (typing(event.target)) {
    if (event.key === 'ArrowDown' && event.target === search.value) { event.preventDefault(); move(1) }
    if (event.key === 'Escape' && event.target === search.value && term.value) { event.preventDefault(); term.value = '' }
    return
  }
  if (event.key === 'j' || event.key === 'ArrowDown') { event.preventDefault(); move(1) }
  else if (event.key === 'k' || event.key === 'ArrowUp') { event.preventDefault(); move(-1) }
  else if (event.key === '/') { event.preventDefault(); search.value?.focus() }
  else if (event.key === 'Enter' && document.activeElement === document.getElementById('main')) { const first = rows()[0]; if (first) void router.push(first.getAttribute('href')!) }
}
async function clearSearch() { term.value = ''; await nextTick(); search.value?.focus() }
onMounted(() => {
  void store.load()
  window.addEventListener('keydown', keydown)
  clock = setInterval(() => { now.value = Date.now() }, 60_000)
})
onBeforeUnmount(() => { window.removeEventListener('keydown', keydown); clearInterval(clock) })
</script>

<template>
  <section class="projects-page" aria-labelledby="projects-title">
    <header class="page-head">
      <p class="eyebrow">{{ session.identity?.tenant.name ?? 'Workspace' }}</p>
      <h1 id="projects-title">Projects</h1>
      <p v-if="store.loaded" class="summary">{{ plural(active.length, 'project') }} · {{ plural(openTotal, 'open ticket') }}</p>
      <p v-else class="summary"><span class="skeleton summary-skeleton" /></p>
    </header>

    <div class="projects-card glass-card">
      <div class="projects-toolbar">
        <label class="search-field project-search">
          <AppIcon name="search" :size="14" />
          <input ref="search" v-model="term" class="field" type="search" placeholder="Filter projects" aria-label="Filter projects" autocomplete="off" spellcheck="false" />
          <kbd v-if="!term" class="keycap slash" aria-hidden="true">/</kbd>
        </label>
        <label class="switch archived-switch">
          <input v-model="showArchived" type="checkbox" />
          <span>Archived</span>
          <span v-if="archivedCount" class="mono count">{{ archivedCount }}</span>
        </label>
        <span class="spacer" />
        <button type="button" class="btn sm sort-btn" aria-haspopup="menu" :aria-expanded="!!sortAnchor" @click="sortAnchor = sortAnchor ? null : ($event.currentTarget as HTMLElement)">
          <span class="sort-by">Sort</span>{{ sortLabel }}<AppIcon name="chevron" :size="12" class="chev" />
        </button>
        <FloatingPanel v-if="sortAnchor" :anchor="sortAnchor" :width="200" align="end" label="Sort projects" @close="closeSort">
          <p class="eyebrow menu-title">Sort by</p>
          <div role="menu" aria-label="Sort projects by">
            <button v-for="option in SORTS" :key="option.value" type="button" role="menuitemradio" class="menu-item" :aria-checked="sort === option.value" :data-autofocus="sort === option.value ? '' : undefined" @click="chooseSort(option.value)">
              <span>{{ option.label }}</span><AppIcon v-if="sort === option.value" name="check" :size="14" class="tick" />
            </button>
          </div>
        </FloatingPanel>
      </div>

      <div v-if="store.error && !store.loaded" class="state" role="alert">
        <AppIcon name="alert" :size="20" />
        <h2>Projects could not be loaded</h2>
        <p>{{ store.error }}</p>
        <button class="btn" type="button" @click="store.load(true)"><AppIcon name="refresh" :size="14" />Try again</button>
      </div>
      <div v-else-if="store.loaded && !visible.length" class="state">
        <AppIcon name="folder" :size="20" />
        <h2>{{ term ? `No project matches “${term}”` : 'No projects yet' }}</h2>
        <p>{{ term ? 'Check the spelling or search by project key.' : 'Projects you create or import appear here.' }}</p>
        <button v-if="term" class="btn" type="button" @click="clearSearch">Clear filter</button>
      </div>
      <!-- One grid for header and rows: the key column is exactly as wide as the widest badge. -->
      <div v-else class="project-grid">
        <div class="projects-head" aria-hidden="true">
          <span class="head-project">Project</span><span class="num">Open</span><span class="num">In progress</span><span class="num">Done</span><span>Progress</span><span class="right">Last activity</span><span />
        </div>
        <div v-if="!store.loaded" class="project-list" role="status" aria-label="Loading projects">
          <div v-for="index in 8" :key="index" class="project-row ghost">
            <span class="skeleton key-skel" />
            <span class="title-skel"><span class="skeleton" /><span class="skeleton short" /></span>
            <span class="skeleton num-skel" /><span class="skeleton num-skel" /><span class="skeleton num-skel" />
            <span class="skeleton bar-skel" /><span class="skeleton time-skel" /><span />
          </div>
        </div>
        <ul v-else ref="list" class="project-list" aria-label="Projects">
          <li v-for="project in visible" :key="project.id">
            <RouterLink class="project-row" :class="{ archived: project.archived }" :to="to(project)" :aria-label="`${project.routeKey} ${project.title}, ${project.open + project.in_progress} open, ${project.in_progress} in progress, ${project.done} done`">
              <span class="key-badge">
                <template v-for="(part, i) in highlight(project.routeKey, term)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template>
              </span>
              <span class="project-text">
                <span class="project-name">
                  <span class="name"><template v-for="(part, i) in highlight(project.title, term)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span>
                  <span v-if="project.frozen" class="chip state-chip frozen">Frozen</span>
                  <span v-else-if="project.state === 'deleted'" class="chip state-chip">Deleted</span>
                  <span v-else-if="project.archived" class="chip state-chip">Archived</span>
                </span>
                <span class="project-desc">{{ project.description || 'No description' }}</span>
              </span>
              <span class="stat" :class="{ zero: !project.open }"><StatusIcon state="new" :size="11" />{{ project.open.toLocaleString('en-GB') }}</span>
              <span class="stat" :class="{ zero: !project.in_progress }"><StatusIcon state="in_progress" :size="11" />{{ project.in_progress.toLocaleString('en-GB') }}</span>
              <span class="stat" :class="{ zero: !project.done }"><StatusIcon state="done" :size="11" />{{ project.done.toLocaleString('en-GB') }}</span>
              <span class="progress" :data-tip="`${project.done.toLocaleString('en-GB')} of ${project.total.toLocaleString('en-GB')} done`">
                <span class="bar"><i :style="{ width: `${percent(project)}%` }" /></span>
                <span class="mono pct">{{ project.total ? `${percent(project)}%` : '—' }}</span>
              </span>
              <time class="activity" :datetime="project.last_activity" :data-tip="absoluteTime(project.last_activity)">{{ relativeTime(project.last_activity, { now, long: true }) }}</time>
              <AppIcon name="chevron-right" :size="14" class="go" />
              <span class="stats-line" aria-hidden="true">
                <span><StatusIcon state="new" :size="10" />{{ project.open.toLocaleString('en-GB') }} open</span>
                <span><StatusIcon state="in_progress" :size="10" />{{ project.in_progress.toLocaleString('en-GB') }} in progress</span>
                <span><StatusIcon state="done" :size="10" />{{ project.done.toLocaleString('en-GB') }} done</span>
              </span>
            </RouterLink>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>

<style scoped>
.projects-page { width: min(1180px, 100%); margin: 0 auto; padding: 30px 28px 40px; }
.page-head { margin-bottom: 20px; }
.page-head h1 { margin-top: 6px; }
.summary { margin-top: 6px; font-size: 13.5px; color: var(--ink-2); min-height: 20px; }
.summary-skeleton { display: inline-block; width: 220px; }
.projects-card { overflow: clip; }
.projects-toolbar { display: flex; align-items: center; gap: 16px; padding: 14px 16px; border-bottom: 1px solid var(--line); }
.project-search { width: 280px; }
.project-search .field { padding-right: 32px; }
.project-search .field::-webkit-search-cancel-button { display: none; }
.slash { position: absolute; right: 9px; pointer-events: none; }
@media (hover: none) { .slash { display: none; } }
.archived-switch .count { font-size: 11px; color: var(--ink-3); }
.spacer { flex: 1; }
.sort-btn { gap: 6px; color: var(--ink); }
.sort-by { font: 500 10px/1 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.chev { color: var(--ink-3); }
.menu-title { padding: 6px 10px 4px; }
.menu-item { display: flex; align-items: center; justify-content: space-between; gap: 10px; width: 100%; height: 32px; padding: 0 10px; border: 0; border-radius: 8px; background: transparent; color: var(--ink); font-size: 13.5px; text-align: left; }
.menu-item:hover { background: var(--row-hover); }
.menu-item:focus-visible { background: var(--row-selected); box-shadow: inset 0 0 0 1px var(--glass-rim); }
.menu-item:active { background: var(--row-selected); }
.tick { color: var(--teal); }

.project-grid { display: grid; grid-template-columns: max-content minmax(0, 1fr) max-content max-content max-content 150px max-content 14px; column-gap: 18px; }
.projects-head, .project-list, .project-list > li, .project-row { display: grid; grid-template-columns: subgrid; grid-column: 1 / -1; align-items: center; }
.projects-head { height: 34px; padding: 0 20px; border-bottom: 1px solid var(--line); font: 500 10.5px/1 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; white-space: nowrap; }
.head-project { grid-column: 1 / 3; }
.num, .right { text-align: right; }
.project-list { margin: 0; padding: 6px 0; list-style: none; }
.project-row { position: relative; min-height: 60px; margin: 0 6px; padding: 8px 14px; border-radius: 10px; color: var(--ink); text-decoration: none; }
.project-row:hover { background: var(--row-hover); }
.project-row:active { background: var(--row-selected); }
.project-row:focus-visible { background: var(--row-selected); box-shadow: inset 3px 0 0 var(--row-accent), 0 0 0 1px var(--glass-rim); }
.project-row.archived { opacity: .72; }
.key-badge { justify-self: start; }
.project-text { display: grid; gap: 1px; min-width: 0; }
.project-name { display: flex; align-items: center; gap: 8px; min-width: 0; }
.name { font-size: 14.5px; font-weight: 650; letter-spacing: -.005em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.project-desc { font-size: 13px; color: var(--ink-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.state-chip { height: 18px; padding: 0 7px; font-size: 10px; text-transform: uppercase; letter-spacing: .08em; }
.state-chip.frozen { color: var(--gold-ink); box-shadow: inset 0 0 0 1px rgba(214, 155, 49, .45); }
/* Icon and number read as one right-aligned group. */
.stat { display: inline-flex; align-items: center; justify-content: flex-end; gap: 6px; font: 500 13px/1 var(--mono); font-variant-numeric: tabular-nums; font-variant-ligatures: none; color: var(--ink); white-space: nowrap; }
.stat.zero { color: var(--ink-3); }
.stats-line { display: none; }
.progress { display: flex; align-items: center; gap: 10px; }
.progress .bar { flex: 1; }
.pct { width: 36px; font-size: 12px; color: var(--ink-2); text-align: right; }
.activity { font-size: 12.5px; color: var(--ink-2); text-align: right; white-space: nowrap; }
.go { color: var(--ink-3); opacity: 0; transform: translateX(-4px); }
.project-row:hover .go, .project-row:focus-visible .go { opacity: 1; transform: none; color: var(--teal); }
.ghost { pointer-events: none; }
.key-skel { width: 60px; height: 20px; border-radius: 6px; }
.title-skel { display: grid; gap: 8px; }
.title-skel .skeleton { width: 46%; }
.title-skel .short { width: 72%; height: 8px; opacity: .7; }
.num-skel { width: 34px; justify-self: end; }
.bar-skel { height: 6px; }
.time-skel { width: 60px; justify-self: end; }
.state { display: grid; justify-items: center; gap: 8px; padding: 64px 24px; text-align: center; color: var(--ink-2); }
.state > svg { color: var(--teal); margin-bottom: 4px; }
.state h2 { color: var(--ink); font-size: 17px; }
.state p { font-size: 13.5px; }
.state .btn { margin-top: 10px; }
@media (prefers-reduced-motion: no-preference) {
  .go { transition: opacity .15s ease, transform .15s ease; }
}
@media (max-width: 1080px) {
  .project-grid { grid-template-columns: max-content minmax(0, 1fr) max-content max-content max-content 120px 0 14px; }
  .projects-head > :nth-child(6), .activity { visibility: hidden; }
}
@media (max-width: 760px) {
  .projects-page { padding: 18px 12px 28px; }
  .page-head { margin-bottom: 14px; padding: 0 4px; }
  .projects-toolbar { flex-wrap: wrap; gap: 10px 14px; padding: 12px; }
  .project-search { width: 100%; }
  .project-search .field { height: 44px; font-size: 16px; }
  .archived-switch, .sort-btn { min-height: 44px; }
  .sort-btn { height: 44px; }
  .project-grid, .project-list, .project-list > li { display: block; }
  .projects-head { display: none; }
  .project-list { padding: 4px 0; }
  .project-row {
    display: grid; grid-template-columns: auto minmax(0, 1fr) auto;
    grid-template-areas: "key key time" "text text text" "bar bar bar" "stats stats stats";
    row-gap: 6px; margin: 0 4px; padding: 12px 10px; min-height: 44px;
  }
  .project-row .key-badge { grid-area: key; }
  .project-text { grid-area: text; }
  .activity { display: block; visibility: visible; grid-area: time; font-size: 12px; }
  .progress { grid-area: bar; }
  .project-row > .stat { display: none; }
  .name { white-space: normal; }
  .project-desc { white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
  .go { display: none; }
  .stats-line { grid-area: stats; display: flex; flex-wrap: wrap; gap: 4px 14px; font: 500 11.5px/1.4 var(--mono); color: var(--ink-2); font-variant-ligatures: none; }
  .stats-line > span { display: inline-flex; align-items: center; gap: 5px; }
}
</style>
