<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { can, myWorkspaceRole, permissionsAvailable } from '../../lib/authz'
import { useSession } from '../../stores/session'
import AppIcon from '../AppIcon.vue'
import SettingsCard from './SettingsCard.vue'

// The workspace itself: its name and my role in it. Who is in it, their roles,
// invites and agent keys live under Access.
const session = useSession()
const role = computed(() => myWorkspaceRole()?.name ?? (permissionsAvailable.value ? 'No workspace role' : '—'))
</script>

<template>
  <div class="section">
    <SettingsCard title="Workspace" icon="folder" anchor="workspace">
      <template #lead>The workspace you are signed in to.</template>
      <dl class="set-facts">
        <div><dt>Name</dt><dd>{{ session.identity?.tenant.name }}</dd></div>
        <div><dt>Your role</dt><dd>{{ role }}</dd></div>
      </dl>
    </SettingsCard>
    <SettingsCard v-if="can('members.read')" title="People and agents" icon="users" anchor="members">
      <template #lead>Members, invites, roles, project access and agent keys have their own place.</template>
      <template #aside><RouterLink class="btn sm" to="/settings/access">Open Access<AppIcon name="arrow" :size="13" /></RouterLink></template>
    </SettingsCard>
  </div>
</template>

<style scoped>
.section { display: grid; gap: 14px; }
</style>
