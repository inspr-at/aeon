<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Project } from '../../stores/projects'
import type { GroupDef } from '../../lib/projectGroups'
import { PROJECT_COLUMN_BY_ID, projectColumnTrack, type ProjectColumnId } from '../../lib/projectColumns'
import GroupHeader from './GroupHeader.vue'
import ProjectRow from './ProjectRow.vue'

export interface ProjectSection { group: GroupDef; items: Project[]; total: number; collapsed: boolean }

// The projects as one grid: header and rows share the columns, so the key column
// is exactly as wide as the widest badge and Open, Doing and Done line up from
// row to row. With groups, each group is a foldable section under its header.
// Selection, dragging and the keyboard belong to the page; this shows them.
const props = defineProps<{
  sections: ProjectSection[]; headers: boolean; columns: ProjectColumnId[]; term: string; now: number; loading: boolean
  selected: Set<string>; dragging: Set<string>; dropOn: string | null; caretBefore: string | null
  rowMenu: string | null; groupMenu: string | null; renaming: string | null; validateName: (name: string, except: string) => string
  to: (project: Project) => string; label: (project: Project) => string
}>()
const emit = defineEmits<{
  toggle: [group: string]; step: [group: string, delta: -1 | 1]; groupMenu: [group: string, anchor: HTMLElement]
  rename: [group: string, name: string | null]; rowMenu: [project: Project, anchor: HTMLElement]
}>()
const root = ref<HTMLElement>()
const template = computed(() => ['max-content', 'minmax(0, 1fr)', ...props.columns.map(projectColumnTrack), '30px'].join(' '))
const stat = (id: ProjectColumnId) => id === 'open' || id === 'doing' || id === 'done'
const flat = computed(() => props.headers ? [] : props.sections.flatMap(section => section.items))
defineExpose({ root })
</script>

<template>
  <div ref="root" class="project-grid" :style="{ gridTemplateColumns: template }">
    <div class="projects-head" aria-hidden="true">
      <span class="head-project">Project</span>
      <span v-for="id in columns" :key="id" :class="[`h-${id}`, { stat: stat(id), right: id === 'activity' }]" :data-tip="PROJECT_COLUMN_BY_ID.get(id)!.tip">{{ PROJECT_COLUMN_BY_ID.get(id)!.label }}</span>
      <span />
    </div>
    <div v-if="loading" class="project-list" role="status" aria-label="Loading projects">
      <div v-for="index in 8" :key="index" class="ghost">
        <span class="skeleton key-skel" />
        <span class="title-skel"><span class="skeleton" /><span class="skeleton short" /></span>
        <span v-for="id in columns" :key="id" class="skeleton" :class="stat(id) ? 'num-skel' : id === 'progress' ? 'bar-skel' : 'time-skel'" />
        <span />
      </div>
    </div>
    <ul v-else-if="!headers" class="project-list" aria-label="Projects">
      <ProjectRow
        v-for="project in flat" :key="project.id" :project="project" :columns="columns" :term="term" :now="now" :to="to(project)" :label="label(project)"
        :selected="selected.has(project.id)" :dragging="dragging.has(project.id)" :menu-open="rowMenu === project.id" show-archived
        @menu="anchor => emit('rowMenu', project, anchor)"
      />
    </ul>
    <ul v-else class="project-list" aria-label="Projects">
      <li v-for="section in sections" :key="section.group.id" class="group-section" :class="{ 'drop-on': dropOn === section.group.id }" :data-group-drop="section.group.id">
        <GroupHeader
          variant="list" :group="section.group" :count="section.total" :collapsed="section.collapsed" :controls="`rows-${section.group.id}`"
          :renaming="renaming === section.group.id" :menu-open="groupMenu === section.group.id" :caret="caretBefore === section.group.id" draggable
          :validate="name => validateName(name, section.group.id)"
          @toggle="emit('toggle', section.group.id)" @step="delta => emit('step', section.group.id, delta)"
          @menu="anchor => emit('groupMenu', section.group.id, anchor)" @rename="name => emit('rename', section.group.id, name)"
        />
        <ul v-if="!section.collapsed && section.items.length" :id="`rows-${section.group.id}`" class="group-rows" :aria-label="section.group.name">
          <ProjectRow
            v-for="project in section.items" :key="project.id" :project="project" :columns="columns" :term="term" :now="now" :to="to(project)" :label="label(project)"
            :selected="selected.has(project.id)" :dragging="dragging.has(project.id)" :menu-open="rowMenu === project.id"
            @menu="anchor => emit('rowMenu', project, anchor)"
          />
        </ul>
        <p v-else-if="!section.collapsed" :id="`rows-${section.group.id}`" class="group-empty">{{ term ? 'No project in this group matches.' : 'No projects here yet. Drag one in, or press m on a project.' }}</p>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.project-grid { display: grid; column-gap: 20px; }
.projects-head, .project-list, .group-section, .group-rows, .ghost { display: grid; grid-template-columns: subgrid; grid-column: 1 / -1; align-items: center; }
.projects-head { height: 34px; padding: 0 20px; border-bottom: 1px solid var(--line); font: 500 10.5px/1 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; white-space: nowrap; }
.head-project { grid-column: 1 / 3; }
.projects-head .right { text-align: right; }
.project-list { margin: 0; padding: 6px 0; list-style: none; }
.group-rows { margin: 0; padding: 0 0 6px; list-style: none; }
.group-section { position: relative; margin: 0 6px; border-radius: 12px; }
.group-section + .group-section { margin-top: 2px; }
.group-section :deep(.project-item) { margin: 0; }
.group-section :deep(.group-head) { grid-column: 1 / -1; }
.group-section.drop-on { background: var(--row-hover); box-shadow: inset 0 0 0 1.5px var(--chip-teal-line); }
.group-empty { grid-column: 1 / -1; margin: 0 0 6px; padding: 4px 20px 14px 46px; font-size: 13px; color: var(--ink-3); }
.ghost { min-height: 60px; margin: 0 6px; padding: 8px 14px; pointer-events: none; }
.key-skel { width: 60px; height: 20px; border-radius: 6px; }
.title-skel { display: grid; gap: 8px; }
.title-skel .skeleton { width: 46%; }
.title-skel .short { width: 72%; height: 8px; opacity: .7; }
.num-skel { width: 44px; justify-self: end; }
.bar-skel { height: 6px; }
.time-skel { width: 60px; justify-self: end; }
@media (max-width: 760px) {
  .project-grid, .project-list, .group-section, .group-rows { display: block; }
  .projects-head { display: none; }
  .project-list { padding: 4px 0; }
  .group-section { margin: 0 4px; }
  .ghost { display: grid; grid-template-columns: auto 1fr; gap: 10px; margin: 0 4px; }
  .ghost > .skeleton:not(.key-skel) { display: none; }
  .group-empty { padding: 2px 16px 12px 40px; }
}
</style>
