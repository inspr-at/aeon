<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { brand } from '../../lib/brand'
import { computed, ref } from 'vue'
import { INVITE_LABEL, creatorName, inviteRoles, type Invite, type InviteStatus } from '../../lib/access'
import { can } from '../../lib/authz'
import { confirmAction } from '../../lib/confirm'
import { toast } from '../../lib/toast'
import { absoluteTime, relativeTime } from '../../lib/work'
import { useAccess } from '../../stores/access'
import AppIcon from '../AppIcon.vue'
import BizIcon from '../business/BizIcon.vue'
import StatusChip from './StatusChip.vue'
import type { InvitePrefill } from './InviteSheet.vue'
import { problem } from './accessText'

// Invites: pending, accepted, expired and revoked, newest first. A pending one
// can be revoked (its link stops working at once); an expired or revoked one can
// be sent again, as a new invite with a new link.
const emit = defineEmits<{ invite: [prefill?: InvitePrefill] }>()
const access = useAccess()
const manage = computed(() => can('members.manage'))
type Filter = InviteStatus | 'all'
const filter = ref<Filter>('pending')
const FILTERS: Filter[] = ['pending', 'accepted', 'expired', 'revoked', 'all']
const count = (f: Filter) => f === 'all' ? access.invites.length : access.invites.filter(i => i.status === f).length
const shown = computed(() => access.invites.filter(i => filter.value === 'all' || i.status === filter.value).sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at)))
const TONE: Record<InviteStatus, 'wait' | 'ok' | 'muted' | 'off'> = { pending: 'wait', accepted: 'ok', expired: 'muted', revoked: 'off' }
function when(invite: Invite) {
  if (invite.status === 'pending') return `Link works until ${absoluteTime(invite.expires_at)}`
  if (invite.status === 'expired') return `Expired ${relativeTime(invite.expires_at, { long: true })}`
  return ''
}
async function revoke(invite: Invite) {
  const ok = await confirmAction({ title: `Revoke the invite for ${invite.email}?`, points: ['The join link stops working at once.', 'Nobody is signed out; they never joined.', 'You can invite them again later with a new link.'], confirmLabel: 'Revoke invite', danger: true })
  if (!ok) return
  try { await access.revokeInvite(invite.id); toast(`The invite for ${invite.email} is revoked`) }
  catch (e) { toast(problem(e, 'The invite stays open'), { tone: 'error' }) }
}
function again(invite: Invite) {
  emit('invite', { email: invite.email, workspaceRoleId: invite.workspace_role?.id ?? null, projectRoles: invite.project_roles.map(r => ({ project_id: r.project_id, role_id: r.role.id })) })
}
</script>

<template>
  <div class="invites-tab">
    <div class="list-tools">
      <div class="seg" role="radiogroup" aria-label="Invites to show">
        <button v-for="f in FILTERS" :key="f" type="button" role="radio" :aria-checked="filter === f" @click="filter = f">{{ f === 'all' ? 'All' : INVITE_LABEL[f] }}<span class="n mono">{{ count(f) }}</span></button>
      </div>
      <span class="spacer" />
      <button v-if="manage" type="button" class="btn primary sm" @click="emit('invite')"><AppIcon name="plus" :size="13" />Invite people</button>
    </div>
    <p class="note"><AppIcon name="info" :size="14" /><span>{{ brand.short_name }} never sends email. Each invite gives you a join link, shown once, to send yourself.</span></p>
    <ul v-if="shown.length" class="invites" aria-label="Invites">
      <li v-for="invite in shown" :key="invite.id" class="invite" :class="invite.status">
        <span class="mail-icon" aria-hidden="true"><BizIcon name="mail" :size="15" /></span>
        <span class="text">
          <span class="email">{{ invite.email }}</span>
          <span class="roles">{{ inviteRoles(invite) }}</span>
          <span class="meta">Invited by {{ creatorName(invite, access.names) }} {{ relativeTime(invite.created_at, { long: true }) }}<template v-if="when(invite)"> · {{ when(invite) }}</template></span>
        </span>
        <StatusChip class="state" :tone="TONE[invite.status]" :label="INVITE_LABEL[invite.status]" />
        <span class="acts">
          <button v-if="manage && invite.status === 'pending'" type="button" class="btn sm" :aria-label="`Revoke the invite for ${invite.email}`" @click="revoke(invite)">Revoke</button>
          <button v-else-if="manage && (invite.status === 'expired' || invite.status === 'revoked')" type="button" class="btn sm ghost" :aria-label="`Invite ${invite.email} again`" @click="again(invite)"><AppIcon name="refresh" :size="12" />Invite again</button>
        </span>
      </li>
    </ul>
    <p v-else class="empty">{{ filter === 'pending' ? 'No invite is waiting.' : filter === 'all' ? 'No invites yet.' : `No ${INVITE_LABEL[filter as InviteStatus].toLowerCase()} invites.` }}</p>
  </div>
</template>

<style scoped>
.invites-tab { display: grid; grid-template-columns: minmax(0, 1fr); gap: 12px; }
.list-tools { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.spacer { flex: 1; }
.n { margin-left: 6px; font-size: 11px; color: var(--ink-3); }
.note { display: grid; grid-template-columns: 14px 1fr; gap: 8px; padding: 9px 12px; border-radius: 10px; background: var(--surface-2); font-size: 12.5px; line-height: 1.45; color: var(--ink-2); }
.note svg { margin-top: 2px; color: var(--ink-3); }
.invites { display: grid; margin: 0; padding: 0; list-style: none; }
.invite { display: grid; grid-template-columns: 32px minmax(0, 1fr) auto 130px; align-items: center; gap: 12px; padding: 10px 0; border-bottom: 1px solid var(--line); }
.mail-icon { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 50%; background: var(--surface-2); color: var(--ink-2); }
.text { display: grid; gap: 1px; min-width: 0; }
.email { font-size: 13.5px; font-weight: 600; color: var(--ink); overflow-wrap: anywhere; }
.roles { font-size: 12.5px; color: var(--ink); }
.meta { font-size: 12px; color: var(--ink-3); }
.invite.revoked .email, .invite.expired .email { color: var(--ink-2); }
.acts { display: flex; justify-content: flex-end; }
.empty { padding: 18px 0; font-size: 13px; color: var(--ink-3); }
@media (max-width: 760px) {
  /* Phones: the filters wrap onto a second row instead of hiding off the edge. */
  .seg { width: 100%; flex-wrap: wrap; border-radius: 16px; }
  .seg button { flex: 1 1 auto; white-space: nowrap; }
  .invite { grid-template-columns: 32px minmax(0, 1fr) auto; grid-template-areas: "icon text text" ". state acts"; row-gap: 8px; }
  .mail-icon { grid-area: icon; }
  .text { grid-area: text; }
  .state { grid-area: state; justify-self: start; }
  .acts { grid-area: acts; }
}
@media (max-width: 600px) { .list-tools .btn.primary { width: 100%; height: 44px; } .seg button { height: 40px; } .acts .btn { height: 44px; } }
</style>
