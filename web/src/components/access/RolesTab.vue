<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { brand } from '../../lib/brand'
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { can } from '../../lib/authz'
import { useAccess } from '../../stores/access'
import AppIcon from '../AppIcon.vue'
import RoleEditor from './RoleEditor.vue'

// Roles: the built-in ones (read-only, copyable) and the workspace's own. A role
// opens at /settings/access/roles/<id>; "new" starts one, from ?from=<id> when
// it duplicates another.
const props = defineProps<{ detail?: string }>()
const route = useRoute()
const access = useAccess()
const manage = computed(() => can('roles.manage'))
const builtin = computed(() => access.roles.filter(r => r.builtin))
const custom = computed(() => access.roles.filter(r => !r.builtin))
const editing = computed(() => props.detail ? (props.detail === 'new' ? 'new' : access.roleById.get(props.detail) ?? null) : null)
const from = computed(() => typeof route.query.from === 'string' ? access.roleById.get(route.query.from) ?? null : null)
const baseName = (id: string | null) => id ? access.roleById.get(id)?.name ?? 'a deleted role' : ''
const people = (n: number) => n ? `Used by ${n}` : 'Unused'
const perms = (n: number) => n === 1 ? '1 permission' : `${n} permissions`
</script>

<template>
  <RoleEditor v-if="editing" :key="editing === 'new' ? `new-${from?.id ?? ''}` : editing.id" :role="editing === 'new' ? null : editing" :from="from" />
  <div v-else-if="detail" class="gone">
    <p>This role no longer exists.</p>
    <RouterLink class="btn sm" to="/settings/access/roles"><AppIcon name="arrow-left" :size="13" />All roles</RouterLink>
  </div>
  <div v-else class="roles-tab">
    <div class="list-tools">
      <p class="lead">A role is a named set of permissions. Built-in roles come with {{ brand.short_name }}; duplicate one to make your own.</p>
      <RouterLink v-if="manage" class="btn primary sm" to="/settings/access/roles/new"><AppIcon name="plus" :size="13" />New role</RouterLink>
    </div>
    <section aria-labelledby="builtin-h">
      <h3 id="builtin-h" class="group-h">Built in <span class="count mono">{{ builtin.length }}</span></h3>
      <ul class="roles">
        <li v-for="role in builtin" :key="role.id">
          <RouterLink class="role" :to="`/settings/access/roles/${role.id}`">
            <span class="r-icon" aria-hidden="true"><AppIcon name="shield" :size="14" /></span>
            <span class="r-text"><span class="r-name">{{ role.name }}</span><span class="r-desc">{{ role.description }}</span></span>
            <span class="r-meta mono">{{ perms(role.permissions.length) }}</span>
            <span class="r-meta">{{ people(role.member_count) }}</span>
            <AppIcon name="chevron-right" :size="14" class="go" />
          </RouterLink>
        </li>
      </ul>
    </section>
    <section aria-labelledby="custom-h">
      <h3 id="custom-h" class="group-h">Custom <span class="count mono">{{ custom.length }}</span></h3>
      <ul v-if="custom.length" class="roles">
        <li v-for="role in custom" :key="role.id">
          <RouterLink class="role" :to="`/settings/access/roles/${role.id}`">
            <span class="r-icon custom" aria-hidden="true"><AppIcon name="sliders" :size="14" /></span>
            <span class="r-text"><span class="r-name">{{ role.name }}</span><span class="r-desc">{{ role.description || (role.based_on ? `Based on ${baseName(role.based_on)}` : 'No description') }}</span></span>
            <span class="r-meta mono">{{ perms(role.permissions.length) }}</span>
            <span class="r-meta">{{ people(role.member_count) }}</span>
            <AppIcon name="chevron-right" :size="14" class="go" />
          </RouterLink>
        </li>
      </ul>
      <p v-else class="empty">No custom roles yet. Open a built-in role and duplicate it to start one.</p>
    </section>
  </div>
</template>

<style scoped>
.roles-tab { display: grid; grid-template-columns: minmax(0, 1fr); gap: 16px; }
.list-tools { display: flex; align-items: center; gap: 12px; }
.lead { flex: 1; font-size: 13px; line-height: 1.5; color: var(--ink-2); }
.group-h { display: flex; align-items: baseline; gap: 6px; margin: 0 0 4px; font: 600 10.5px/1.5 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); }
.count { letter-spacing: 0; }
.roles { display: grid; margin: 0; padding: 0; list-style: none; }
.role { display: grid; grid-template-columns: 30px minmax(0, 1fr) auto 90px 16px; align-items: center; gap: 12px; min-height: 56px; margin: 0 -10px; padding: 6px 10px; border-radius: 10px; color: var(--ink); text-decoration: none; }
.roles li + li .role { box-shadow: 0 -1px 0 var(--line); }
@media (hover: hover) { .role:hover { background: var(--row-hover); } .role:hover .go { color: var(--teal-ink); } }
.role:focus-visible { box-shadow: var(--focus-ring); }
.r-icon { display: grid; place-items: center; width: 30px; height: 30px; border-radius: 9px; background: var(--surface-2); color: var(--ink-2); }
.r-icon.custom { background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.r-text { display: grid; gap: 1px; min-width: 0; }
.r-name { font-size: 13.5px; font-weight: 600; }
.r-desc { font-size: 12.5px; color: var(--ink-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.r-meta { font-size: 12px; color: var(--ink-3); white-space: nowrap; text-align: right; }
.go { color: var(--ink-3); }
.empty, .gone p { padding: 10px 0; font-size: 13px; color: var(--ink-3); }
.gone { display: grid; justify-items: start; gap: 8px; }
@media (max-width: 760px) {
  .list-tools { flex-direction: column; align-items: stretch; }
  .role { grid-template-columns: 30px minmax(0, 1fr) 16px; }
  .r-meta { display: none; }
  .r-desc { white-space: normal; }
  .list-tools .btn { height: 44px; justify-content: center; }
}
</style>
