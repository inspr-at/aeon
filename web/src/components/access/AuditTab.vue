<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { AUDIT_CATEGORIES, auditSentence, categoryOf, getAudit, type AuditCategory, type AuditEvent } from '../../lib/access'
import { absoluteTime, relativeTime } from '../../lib/work'
import { useAccess } from '../../stores/access'
import { useProjects } from '../../stores/projects'
import AppIcon, { type IconName } from '../AppIcon.vue'
import Avatar from '../Avatar.vue'
import { problem } from './accessText'

// The access log: every change to roles, access, invites, people's sign-in and
// agent keys, newest first, as sentences. Filter by kind of change, by person,
// or by any word in it.
const access = useAccess()
const projects = useProjects()
const events = ref<AuditEvent[]>([])
const complete = ref(true)
const state = ref<'loading' | 'ready' | 'error'>('loading')
const error = ref('')
const kinds = ref(new Set<AuditCategory>())
const who = ref('')
const term = ref('')
const names = {
  principal: (id: string | null | undefined) => (id ? access.names.get(id) : undefined) ?? 'someone who is no longer here',
  project: (id: string | null | undefined) => (id ? projects.byId(id)?.title : undefined) ?? '',
  role: (id: string | null | undefined) => (id ? access.roleById.get(id)?.name : undefined) ?? 'a role that was deleted',
}
const rows = computed(() => events.value.map(event => ({ event, category: categoryOf(event.type), ...auditSentence(event, names) })))
const people = computed(() => [...new Map(rows.value.flatMap(r => [[r.event.actor.principal_id, r.actor] as const, ...(r.event.subject ? [[r.event.subject.principal_id, r.event.subject.name || r.subject] as const] : [])])).entries()].sort((a, b) => a[1].localeCompare(b[1])))
const shown = computed(() => rows.value.filter(r => (!kinds.value.size || (r.category && kinds.value.has(r.category)))
  && (!who.value || r.event.actor.principal_id === who.value || r.event.subject?.principal_id === who.value)
  && (!term.value.trim() || `${r.actor} ${r.text}`.toLowerCase().includes(term.value.trim().toLowerCase()))))
const days = computed(() => {
  const out: { day: string; label: string; items: typeof shown.value }[] = []
  for (const row of shown.value) {
    const date = new Date(row.event.at)
    const day = date.toDateString()
    let bucket = out.find(d => d.day === day)
    if (!bucket) { bucket = { day, label: dayLabel(date), items: [] }; out.push(bucket) }
    bucket.items.push(row)
  }
  return out
})
function dayLabel(date: Date) {
  const today = new Date(); const yesterday = new Date(Date.now() - 86_400_000)
  if (date.toDateString() === today.toDateString()) return 'Today'
  if (date.toDateString() === yesterday.toDateString()) return 'Yesterday'
  return date.toLocaleDateString('en-GB', { weekday: 'short', day: 'numeric', month: 'short', year: date.getFullYear() === today.getFullYear() ? undefined : 'numeric' })
}
const ICON: Record<AuditCategory, IconName> = { roles: 'shield', bindings: 'users', invites: 'send', lifecycle: 'user', keys: 'key' }
function toggle(kind: AuditCategory) { const set = new Set(kinds.value); if (set.has(kind)) set.delete(kind); else set.add(kind); kinds.value = set }
function clear() { kinds.value = new Set(); who.value = ''; term.value = '' }
const filtered = computed(() => kinds.value.size > 0 || !!who.value || !!term.value.trim())
async function load() {
  state.value = 'loading'
  try { const log = await getAudit(); events.value = log.items; complete.value = log.complete; state.value = 'ready' }
  catch (e) { error.value = problem(e, 'The access log could not be loaded'); state.value = 'error' }
}
onMounted(() => { void load(); void projects.load() })
</script>

<template>
  <div class="audit-tab">
    <div class="filters">
      <div class="kinds" role="group" aria-label="Kinds of change">
        <button v-for="c in AUDIT_CATEGORIES" :key="c.id" type="button" class="kind" :aria-pressed="kinds.has(c.id)" @click="toggle(c.id)"><AppIcon :name="ICON[c.id]" :size="13" />{{ c.label }}</button>
      </div>
      <select v-model="who" class="field who" aria-label="Changes by or about">
        <option value="">Anyone</option>
        <option v-for="[id, name] in people" :key="id" :value="id">{{ name }}</option>
      </select>
      <label class="search-field find">
        <AppIcon name="search" :size="14" />
        <input v-model="term" class="field" type="search" placeholder="Find in the log" aria-label="Find in the access log" autocomplete="off" spellcheck="false" />
      </label>
      <button v-if="filtered" type="button" class="btn sm ghost" @click="clear">Clear filters</button>
    </div>
    <div v-if="state === 'loading'" class="set-skeleton" role="status" aria-label="Loading the access log"><span class="skeleton" /><span class="skeleton" /><span class="skeleton" /></div>
    <p v-else-if="state === 'error'" class="set-note error" role="alert"><AppIcon name="alert" :size="14" />{{ error }}<button type="button" class="btn sm" @click="load">Try again</button></p>
    <template v-else>
      <p class="summary" role="status">{{ filtered ? `${shown.length} of ${rows.length} changes` : `${rows.length} changes` }}{{ complete ? '' : ' (the most recent; older ones are not shown)' }}</p>
      <section v-for="day in days" :key="day.day" class="day" :aria-label="day.label">
        <h3 class="day-h">{{ day.label }}</h3>
        <ol class="events">
          <li v-for="row in day.items" :key="row.event.id" class="event">
            <span class="e-icon" aria-hidden="true"><AppIcon :name="row.category ? ICON[row.category] : 'history'" :size="13" /></span>
            <Avatar :id="row.event.actor.principal_id" :name="row.actor" :size="22" />
            <p class="e-text"><b>{{ row.actor }}</b> {{ row.text }}</p>
            <time class="e-time" :datetime="row.event.at" :data-tip="absoluteTime(row.event.at)">{{ relativeTime(row.event.at) }}</time>
          </li>
        </ol>
      </section>
      <p v-if="!shown.length" class="empty">{{ filtered ? 'No change matches these filters.' : 'No access changes yet.' }}</p>
    </template>
  </div>
</template>

<style scoped>
.audit-tab { display: grid; grid-template-columns: minmax(0, 1fr); gap: 12px; }
.filters { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 10px; }
.kinds { display: flex; flex-wrap: wrap; gap: 6px; }
.kind { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 11px; border: 0; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); color: var(--ink-2); font-size: 12.5px; font-weight: 600; }
.kind svg { color: var(--ink-3); }
@media (hover: hover) { .kind:hover { color: var(--ink); background: var(--row-hover); } }
.kind[aria-pressed="true"] { background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.kind[aria-pressed="true"] svg { color: var(--teal-ink); }
.kind:focus-visible { box-shadow: var(--focus-ring); }
.who { width: 190px; height: 32px; appearance: auto; }
.find { width: 220px; }
.summary { font-size: 12.5px; color: var(--ink-3); }
.day { display: grid; gap: 2px; }
.day-h { margin: 6px 0 2px; font: 600 10.5px/1.5 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); }
.events { display: grid; margin: 0; padding: 0; list-style: none; }
.event { display: grid; grid-template-columns: 26px 22px minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 44px; padding: 6px 0; border-bottom: 1px solid var(--line); }
.e-icon { display: grid; place-items: center; width: 26px; height: 26px; border-radius: 8px; background: var(--surface-2); color: var(--ink-2); }
.e-text { font-size: 13px; line-height: 1.45; color: var(--ink-2); }
.e-text b { color: var(--ink); font-weight: 600; }
.e-time { font-size: 12px; color: var(--ink-3); white-space: nowrap; }
.empty { padding: 12px 0; font-size: 13px; color: var(--ink-3); }
@media (max-width: 600px) {
  .kinds { width: 100%; }
  .kind { height: 44px; }
  .who, .find { width: 100%; }
  .who, .find .field { height: 44px; font-size: 16px; }
  .event { grid-template-columns: 26px minmax(0, 1fr); grid-template-areas: "icon text" ". time"; row-gap: 2px; }
  .event > :nth-child(2) { display: none; }
  .e-icon { grid-area: icon; align-self: start; }
  .e-text { grid-area: text; }
  .e-time { grid-area: time; }
}
</style>
