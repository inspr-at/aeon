<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { listPrincipals, type Principal } from '../../lib/business'
import { keyState, listAgentKeys, statusOf, type AgentKey } from '../../lib/settings'
import { absoluteTime, relativeTime } from '../../lib/work'
import { useSession } from '../../stores/session'
import AppIcon from '../AppIcon.vue'
import Avatar from '../Avatar.vue'
import SettingsCard from './SettingsCard.vue'

// The workspace as it is today, read-only: who is in it and which agent keys
// exist. Managing members and keys arrives later; nothing here pretends to.
const session = useSession()
const members = ref<Principal[] | null>(null)
const membersState = ref<'loading' | 'ready' | 'closed' | 'error'>('loading')
const keys = ref<AgentKey[] | null>(null)
const keysError = ref('')
// People who work here, customer contacts who can sign in, and agents; the
// workspace's own system principal is not a member.
const isCustomer = (p: Principal) => p.roles.length > 0 && p.roles.every(role => role === 'customer')
const people = computed(() => (members.value ?? []).filter(p => p.kind === 'person' && !isCustomer(p)))
const customers = computed(() => (members.value ?? []).filter(p => p.kind === 'person' && isCustomer(p)))
const agentsIn = computed(() => (members.value ?? []).filter(p => p.kind === 'agent' && !p.roles.includes('system')))
const ROLE: Record<string, string> = { admin: 'Admin', member: 'Member', viewer: 'Viewer', customer: 'Customer' }
const roleText = (roles: string[]) => roles.map(r => ROLE[r] ?? r).join(' · ') || 'No role'
const STATE: Record<string, string> = { active: 'Active', expired: 'Expired', revoked: 'Revoked' }

async function loadMembers() {
  membersState.value = 'loading'
  try { members.value = await listPrincipals(); membersState.value = 'ready' }
  // The directory answers only while a Business part is enabled.
  catch (e) { membersState.value = statusOf(e) === 403 ? 'closed' : 'error' }
}
async function loadKeys() {
  keysError.value = ''
  try { keys.value = await listAgentKeys() } catch { keysError.value = 'The agent keys could not be loaded.' }
}
onMounted(() => { void loadMembers(); void loadKeys() })
</script>

<template>
  <div class="section">
    <SettingsCard title="Workspace" icon="folder" anchor="workspace">
      <template #lead>The workspace you are signed in to.</template>
      <dl class="set-facts">
        <div><dt>Name</dt><dd>{{ session.identity?.tenant.name }}</dd></div>
        <div><dt>Your role</dt><dd>{{ roleText(session.identity?.principal.roles ?? []) }}</dd></div>
      </dl>
    </SettingsCard>

    <SettingsCard title="Members" icon="users" anchor="members">
      <template #lead>People and agents who work in this workspace.</template>
      <div v-if="membersState === 'loading'" class="set-skeleton" role="status" aria-label="Loading members"><span class="skeleton" /><span class="skeleton" /><span class="skeleton" /></div>
      <p v-else-if="membersState === 'closed'" class="set-note"><AppIcon name="info" :size="14" />Members show here only while a Business part is on, and none is on in this workspace. A member list of its own arrives with workspace administration.</p>
      <p v-else-if="membersState === 'error'" class="set-note error" role="alert"><AppIcon name="alert" :size="14" />The members could not be loaded.<button type="button" class="btn sm" @click="loadMembers">Try again</button></p>
      <template v-else>
        <p class="group-h">People <span class="count">{{ people.length }}</span></p>
        <ul class="people">
          <li v-for="p in people" :key="p.id"><Avatar :id="p.id" :name="p.name" :size="28" /><span class="person-name">{{ p.name }}</span><span class="role">{{ roleText(p.roles) }}</span></li>
        </ul>
        <template v-if="customers.length">
          <p class="group-h">Customer contacts <span class="count">{{ customers.length }}</span></p>
          <ul class="people">
            <li v-for="p in customers" :key="p.id"><Avatar :id="p.id" :name="p.name" :size="28" /><span class="person-name">{{ p.name }}</span><span class="role">Customer</span></li>
          </ul>
        </template>
        <template v-if="agentsIn.length">
          <p class="group-h">Agents <span class="count">{{ agentsIn.length }}</span></p>
          <ul class="people">
            <li v-for="p in agentsIn" :key="p.id"><Avatar :id="p.id" :name="p.name" kind="agent" :size="28" /><span class="person-name mono">{{ p.name }}</span><span class="role">Agent</span></li>
          </ul>
        </template>
        <p class="set-note"><AppIcon name="info" :size="14" />Inviting people and changing their roles arrives here next.</p>
      </template>
    </SettingsCard>

    <SettingsCard title="Agent keys" icon="key" anchor="agent-keys">
      <template #lead>Keys agents use to work in this workspace. Only a key's prefix is ever shown again.</template>
      <div v-if="!keys && !keysError" class="set-skeleton" role="status" aria-label="Loading agent keys"><span class="skeleton" /><span class="skeleton" /></div>
      <p v-else-if="keysError" class="set-note error" role="alert"><AppIcon name="alert" :size="14" />{{ keysError }}<button type="button" class="btn sm" @click="loadKeys">Try again</button></p>
      <p v-else-if="!keys!.length" class="set-note"><AppIcon name="info" :size="14" />No agent keys yet.</p>
      <div v-else class="table-wrap">
        <table class="keys-table">
          <thead><tr><th scope="col">Name</th><th scope="col">Key</th><th scope="col">Scopes</th><th scope="col">Last used</th><th scope="col">Status</th></tr></thead>
          <tbody>
            <tr v-for="key in keys!" :key="key.id" :class="keyState(key)">
              <th scope="row">{{ key.name }}<span class="sub">Created {{ relativeTime(key.created_at, { long: true }) }}</span></th>
              <td class="mono">aeon_{{ key.prefix }}_…</td>
              <td><span v-if="!key.scopes.length" class="muted">All</span><span v-for="scope in key.scopes" :key="scope" class="scope mono">{{ scope }}</span></td>
              <td><time v-if="key.last_used_at" :datetime="key.last_used_at" :data-tip="absoluteTime(key.last_used_at)">{{ relativeTime(key.last_used_at, { long: true }) }}</time><span v-else class="muted">Never</span></td>
              <td><span class="state" :class="keyState(key)">{{ STATE[keyState(key)] }}</span><span v-if="key.expires_at && keyState(key) === 'active'" class="sub">until {{ absoluteTime(key.expires_at) }}</span></td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="keys" class="set-note"><AppIcon name="info" :size="14" />Creating and revoking keys here arrives next.</p>
    </SettingsCard>
  </div>
</template>

<style scoped>
.section { display: grid; gap: 14px; }
.group-h { display: flex; align-items: baseline; gap: 6px; margin: 4px 0 6px; font: 600 10.5px/1.5 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); }
.group-h + .people { margin-bottom: 12px; }
.count { letter-spacing: 0; }
.people { display: grid; margin: 0; padding: 0; list-style: none; }
.people li { display: grid; grid-template-columns: 30px minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 42px; border-top: 1px solid var(--line); }
.person-name { font-size: 13.5px; color: var(--ink); overflow-wrap: anywhere; }
.person-name.mono { font-size: 12.5px; }
.role { font-size: 12.5px; color: var(--ink-2); white-space: nowrap; }
.table-wrap { overflow-x: auto; margin: 0 -4px; padding: 0 4px; }
.keys-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.keys-table th, .keys-table td { padding: 9px 10px 9px 0; text-align: left; vertical-align: top; border-top: 1px solid var(--line); }
.keys-table thead th { padding-top: 0; border-top: 0; font: 500 10.5px/1.4 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--ink-3); }
.keys-table tbody th { font-weight: 600; color: var(--ink); }
.sub { display: block; margin-top: 2px; font-size: 11.5px; font-weight: 400; color: var(--ink-3); }
.scope { display: inline-flex; align-items: center; height: 20px; margin: 0 4px 4px 0; padding: 0 7px; border-radius: 6px; background: var(--surface-2); color: var(--ink-2); font-size: 11px; }
.muted { color: var(--ink-3); }
.state { display: inline-flex; align-items: center; height: 20px; padding: 0 8px; border-radius: 999px; background: var(--surface-2); color: var(--ink-2); font-size: 11.5px; font-weight: 600; }
.state.active { background: rgba(47, 122, 90, .12); color: color-mix(in oklab, var(--ok), var(--ink) 35%); }
tr.revoked, tr.expired { color: var(--ink-2); }
@media (max-width: 600px) {
  /* Phones: each key reads as a small card, not a table that scrolls sideways. */
  .keys-table thead { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); }
  .keys-table, .keys-table tbody, .keys-table tr, .keys-table th, .keys-table td { display: block; }
  .keys-table tr { padding: 8px 0; border-top: 1px solid var(--line); }
  .keys-table th, .keys-table td { padding: 2px 0; border: 0; }
}
</style>
