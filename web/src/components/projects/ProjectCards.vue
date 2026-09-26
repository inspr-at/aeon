<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import type { Project } from '../../stores/projects'
import GroupHeader from './GroupHeader.vue'
import ProjectCard from './ProjectCard.vue'
import type { ProjectSection } from './ProjectList.vue'

// The projects as cards, grouped like the list: each group under its header,
// its cards in a grid that reflows to one column on a phone. Cards glide to
// their new places when the order changes (a sort, a filter, a drag).
defineProps<{
  sections: ProjectSection[]; headers: boolean; term: string; now: number; loading: boolean; arranging?: boolean
  selected: Set<string>; dragging: Set<string>; dropOn: string | null; caretBefore: string | null
  rowMenu: string | null; groupMenu: string | null; renaming: string | null; validateName: (name: string, except: string) => string
  to: (project: Project) => string; label: (project: Project) => string
}>()
const emit = defineEmits<{
  toggle: [group: string]; step: [group: string, delta: -1 | 1]; groupMenu: [group: string, anchor: HTMLElement]
  rename: [group: string, name: string | null]; rowMenu: [project: Project, anchor: HTMLElement]
}>()
</script>

<template>
  <div class="cards-view" :class="{ arranging }">
    <div v-if="loading" class="card-grid" role="status" aria-label="Loading projects">
      <div v-for="index in 6" :key="index" class="ghost-card">
        <span class="skeleton key-skel" /><span class="skeleton name-skel" /><span class="skeleton line-skel" />
        <span class="ghost-mid"><span class="skeleton ring-skel" /><span class="ghost-counts"><span class="skeleton" /><span class="skeleton" /><span class="skeleton" /></span></span>
      </div>
    </div>
    <TransitionGroup v-else-if="!headers" tag="ul" name="card" class="card-grid" aria-label="Projects">
      <template v-for="section in sections" :key="section.group.id">
        <ProjectCard
          v-for="project in section.items" :key="project.id" :project="project" :term="term" :now="now" :to="to(project)" :label="label(project)"
          :selected="selected.has(project.id)" :dragging="dragging.has(project.id)" :menu-open="rowMenu === project.id" @menu="anchor => emit('rowMenu', project, anchor)"
        />
      </template>
    </TransitionGroup>
    <ul v-else class="card-sections" aria-label="Projects">
      <li v-for="section in sections" :key="section.group.id" class="card-section" :class="{ 'drop-on': dropOn === section.group.id }" :data-group-drop="section.group.id">
        <GroupHeader
          variant="cards" :group="section.group" :count="section.total" :collapsed="section.collapsed" :controls="`cards-${section.group.id}`"
          :renaming="renaming === section.group.id" :menu-open="groupMenu === section.group.id" :caret="caretBefore === section.group.id" draggable
          :validate="name => validateName(name, section.group.id)"
          @toggle="emit('toggle', section.group.id)" @step="delta => emit('step', section.group.id, delta)"
          @menu="anchor => emit('groupMenu', section.group.id, anchor)" @rename="name => emit('rename', section.group.id, name)"
        />
        <TransitionGroup v-if="!section.collapsed && section.items.length" :id="`cards-${section.group.id}`" tag="ul" name="card" class="card-grid" :aria-label="section.group.name">
          <ProjectCard
            v-for="project in section.items" :key="project.id" :project="project" :term="term" :now="now" :to="to(project)" :label="label(project)"
            :selected="selected.has(project.id)" :dragging="dragging.has(project.id)" :menu-open="rowMenu === project.id" @menu="anchor => emit('rowMenu', project, anchor)"
          />
        </TransitionGroup>
        <p v-else-if="!section.collapsed" :id="`cards-${section.group.id}`" class="group-empty">{{ term ? 'No project in this group matches.' : 'No projects here yet. Drag a card in, or press m on a project.' }}</p>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.cards-view { min-width: 0; }
.card-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 300px), 1fr)); gap: 16px; margin: 0; padding: 0; list-style: none; }
.card-sections { display: grid; gap: 22px; margin: 0; padding: 0; list-style: none; }
.card-section { position: relative; display: grid; gap: 10px; margin: -8px -10px; padding: 8px 10px 10px; border-radius: 20px; }
.card-section.drop-on { background: var(--row-hover); box-shadow: inset 0 0 0 1.5px var(--chip-teal-line); }
@media (prefers-reduced-motion: no-preference) { .card-move { transition: transform .24s cubic-bezier(.2, .75, .3, 1); } }
/* While a card is carried, the others stay still under the pointer. */
.arranging :deep(.card) { transform: none; }
.arranging :deep(.card-more), .arranging :deep(.card-grip) { opacity: 0; }
.group-empty { margin: 0; padding: 18px 20px; border-radius: var(--radius); box-shadow: inset 0 0 0 1px var(--line-2); font-size: 13px; color: var(--ink-3); text-align: center; }
.ghost-card { display: grid; gap: 10px; min-height: 212px; padding: 18px; border-radius: var(--radius); border: 1px solid var(--glass-edge); background: var(--glass-2); box-shadow: var(--shadow); }
.key-skel { width: 64px; height: 20px; border-radius: 6px; }
.name-skel { width: 52%; height: 12px; margin-top: 6px; }
.line-skel { width: 78%; height: 8px; opacity: .7; }
.ghost-mid { display: flex; align-items: center; gap: 20px; margin-top: 10px; }
.ring-skel { width: 56px; height: 56px; border-radius: 50%; }
.ghost-counts { display: grid; gap: 10px; width: 140px; }
@media (max-width: 600px) { .card-grid { gap: 12px; } .card-sections { gap: 18px; } .card-section { margin: -6px -6px; padding: 6px 6px 8px; } }
</style>
