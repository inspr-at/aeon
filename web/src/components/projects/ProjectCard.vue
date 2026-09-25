<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import type { Project } from '../../stores/projects'
import { absoluteTime, highlight, relativeTime } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import PeopleStack from './PeopleStack.vue'
import ProgressRing from './ProgressRing.vue'
import StatCount from './StatCount.vue'

// One project as a card: key, name, a line of description, progress as a ring,
// Open, Doing and Done with their icons in one straight line, who was active
// lately and when. The card is a link; its … button opens the same menu as a row.
defineProps<{ project: Project; term: string; now: number; to: string; label: string; selected: boolean; dragging: boolean; menuOpen: boolean }>()
const emit = defineEmits<{ menu: [anchor: HTMLElement] }>()
</script>

<template>
  <li class="card" :class="{ selected, dragging, menu: menuOpen, archived: project.archived }" :data-project-id="project.id" draggable="true">
    <RouterLink class="card-link item-link" :to="to" :aria-label="label" draggable="false">
      <span class="card-top">
        <span class="key-badge"><template v-for="(part, i) in highlight(project.routeKey, term)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span>
        <span v-if="project.frozen" class="chip state-chip frozen">Frozen</span>
        <span v-else-if="project.state === 'deleted'" class="chip state-chip">Deleted</span>
      </span>
      <span class="card-name"><template v-for="(part, i) in highlight(project.title, term)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span>
      <span class="card-desc">{{ project.description || 'No description' }}</span>
      <span class="card-mid">
        <span class="ring-wrap" :data-tip="`${project.done.toLocaleString('en-GB')} of ${(project.total - project.cancelled).toLocaleString('en-GB')} done${project.cancelled ? ` · ${project.cancelled} cancelled` : ''}`">
          <ProgressRing :percent="project.percent" :empty="!project.total" :size="56" />
        </span>
        <span class="counts">
          <StatCount kind="open" :value="project.open" label />
          <StatCount kind="doing" :value="project.in_progress" label />
          <StatCount kind="done" :value="project.done" label />
        </span>
      </span>
      <span class="card-foot">
        <PeopleStack :people="project.people" :size="22" />
        <span v-if="!project.people.length" class="nobody">No one lately</span>
        <time class="activity" :datetime="project.last_activity" :data-tip="absoluteTime(project.last_activity)">{{ relativeTime(project.last_activity, { now, long: true }) }}</time>
      </span>
    </RouterLink>
    <button
      type="button" class="icon-btn sm flat card-more" :aria-label="`Actions for ${project.routeKey} ${project.title}`" aria-haspopup="menu" :aria-expanded="menuOpen"
      data-tip="Actions · Shift F10" @click="emit('menu', $event.currentTarget as HTMLElement)"
    ><AppIcon name="more" :size="15" /></button>
  </li>
</template>

<style scoped>
.card {
  position: relative; display: flex; min-width: 0; border-radius: var(--radius); border: 1px solid var(--glass-edge);
  background: linear-gradient(165deg, var(--surface-raised-2), var(--glass) 60%); box-shadow: var(--shadow);
  backdrop-filter: blur(18px) saturate(1.15); -webkit-backdrop-filter: blur(18px) saturate(1.15);
}
@media (hover: hover) { .card:hover { box-shadow: var(--shadow), 0 16px 32px -22px rgba(16, 35, 39, .45); } }
@media (hover: hover) and (prefers-reduced-motion: no-preference) {
  .card { transition: box-shadow .18s ease, transform .18s ease; }
  .card:hover { transform: translateY(-1px); }
}
.card:has(.card-link:focus-visible) { box-shadow: var(--focus-ring), var(--shadow); }
.card.selected { background: linear-gradient(165deg, var(--surface-raised-2), var(--glass) 60%), var(--row-selected); box-shadow: 0 0 0 1.5px var(--teal), var(--shadow); }
.card.selected:has(.card-link:focus-visible) { box-shadow: 0 0 0 1.5px var(--teal), var(--focus-ring), var(--shadow); }
.card.dragging { opacity: .45; }
.card.archived .card-link { opacity: .72; }
.card-link { display: flex; flex-direction: column; flex: 1; min-width: 0; padding: 16px 18px 14px; border-radius: inherit; color: var(--ink); text-decoration: none; }
.card-link:focus-visible { box-shadow: none; }
.card-top { display: flex; align-items: center; gap: 8px; min-height: 22px; padding-right: 30px; }
.state-chip { height: 18px; padding: 0 7px; font-size: 10px; text-transform: uppercase; letter-spacing: .08em; }
.state-chip.frozen { color: var(--gold-ink); box-shadow: inset 0 0 0 1px rgba(214, 155, 49, .45); }
.card-name { margin-top: 12px; font-size: 16px; font-weight: 650; letter-spacing: -.01em; line-height: 1.3; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.card-desc { margin-top: 3px; font-size: 13px; color: var(--ink-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.card-mid { display: flex; align-items: center; gap: 20px; margin-top: 18px; }
.ring-wrap { display: inline-flex; }
.counts { display: grid; gap: 7px; width: min(100%, 164px); }
.counts :deep(.stat-count) { gap: 8px; }
.card-foot { display: flex; align-items: center; gap: 10px; min-height: 28px; margin-top: 18px; padding-top: 12px; border-top: 1px solid var(--line); }
.nobody { font-size: 12px; color: var(--ink-3); }
.activity { margin-left: auto; font-size: 12.5px; color: var(--ink-2); white-space: nowrap; }
.card-more { position: absolute; top: 12px; right: 12px; width: 30px; height: 30px; color: var(--ink-3); opacity: 0; }
.card:hover .card-more, .card:focus-within .card-more, .card.menu .card-more { opacity: 1; }
.card-more:hover, .card.menu .card-more { color: var(--teal-ink); }
@media (hover: none) { .card-more { opacity: 1; } }
@media (max-width: 600px) { .card-link { padding: 14px 16px 12px; } .card-more { top: 8px; right: 8px; width: 36px; height: 36px; } }
</style>
