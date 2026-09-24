<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { ACTION_LONG, gateApprovals, offeredApproval } from '../../lib/journey'
import { useJourneyContext } from '../../lib/journeyContext'
import { plural, statusMeta } from '../../lib/work'
import { useJourney } from '../../stores/journey'
import AppIcon from '../AppIcon.vue'
import GateApprovals from './GateApprovals.vue'
import GateCard from './GateCard.vue'
import ReleaseTickets from './ReleaseTickets.vue'

// Build: the release's tickets with their state as the crew works on them, and
// the candidate gate once the build is ready. Sending the candidate back needs
// a reason the crew reads.
const ctx = useJourneyContext()
const store = useJourney()
const journey = computed(() => ctx.journey.value)
const walker = computed(() => ctx.data.walker.value.value)
const tickets = computed(() => (walker.value?.tickets ?? []).filter(t => t.included))
const stateOf = (id: string) => ctx.data.workById.value.get(id)?.state ?? ''
const done = computed(() => tickets.value.filter(t => statusMeta(stateOf(t.ticket_node_id)).closed).length)
const segments = computed(() => { const n = Math.min(24, Math.max(tickets.value.length, 1)); return Array.from({ length: n }, (_, i) => i < Math.round(done.value / Math.max(1, tickets.value.length) * n)) })
const next = computed(() => journey.value.next_action)
const building = computed(() => next.value.key === 'wait_for_build')
const candidate = computed(() => next.value.key === 'approve_candidate')
const approvals = computed(() => gateApprovals(ctx.approvals.value, 'candidate', journey.value.current_release_id))
const approval = computed(() => offeredApproval(ctx.approvals.value, journey.value, 'candidate', ctx.now.value))
const rejecting = ref(false)
const reason = ref('')
const reasonField = ref<HTMLTextAreaElement>()
async function startReject() { rejecting.value = true; reason.value = ''; await nextTick(); reasonField.value?.focus() }
async function reject() {
  if (!reason.value.trim()) return
  if (await ctx.act('reject_candidate', { approval: approval.value, reason: reason.value.trim(), done: 'The candidate goes back to the crew with your reason.' })) rejecting.value = false
}
</script>

<template>
  <div class="j-grid">
    <div class="j-col">
      <section class="j-card" aria-labelledby="build-tickets">
        <header class="j-card-head">
          <p id="build-tickets" class="eyebrow">Tickets · {{ tickets.length }}</p>
          <span v-if="tickets.length" class="j-count">{{ done }} of {{ tickets.length }} done</span>
          <button type="button" class="btn sm" :disabled="!tickets.length" aria-keyshortcuts="w" @click="ctx.walk()"><AppIcon name="expand" :size="13" />Full screen</button>
        </header>
        <div v-if="!ctx.release.value" class="j-empty"><strong>No release is being built</strong><span>The build starts from the planned release.</span></div>
        <p v-else-if="ctx.data.walker.status.value === 'loading'" class="skeleton list-skel" role="status" aria-label="Loading the release" />
        <div v-else-if="!tickets.length" class="j-empty"><strong>No tickets in this release</strong></div>
        <ReleaseTickets v-else :plan="ctx.plan" :editable="false" only="included" :project-key="ctx.project.value.routeKey" :work-by-id="ctx.data.workById.value" @walk="t => ctx.walk(t.key)" @open="ctx.open" />
      </section>
    </div>
    <div class="j-col">
      <section v-if="building" class="j-card" aria-labelledby="build-progress">
        <header class="j-card-head"><p id="build-progress" class="eyebrow">In progress</p></header>
        <h3>Building {{ ctx.releaseLabel.value }}</h3>
        <div class="j-segs" role="img" :aria-label="`${done} of ${tickets.length} tickets done`"><i v-for="(on, i) in segments" :key="i" :class="{ on }" /></div>
        <p class="j-note">{{ done }} of {{ plural(tickets.length, 'ticket') }} done. {{ ACTION_LONG.wait_for_build }}</p>
      </section>
      <GateCard
        v-if="candidate" eyebrow="Decision" title="Approve the release candidate"
        :action="{ label: ctx.next.value.label, disabled: ctx.next.value.disabled, busy: ctx.next.value.busy, tip: ctx.next.value.tip }" @act="ctx.runNext()"
      >
        <p>All {{ plural(tickets.length, 'ticket') }} of {{ ctx.releaseLabel.value }} are built. {{ ACTION_LONG.approve_candidate }}</p>
        <GateApprovals gate="candidate" :approvals="approvals" :on="ctx.releaseLabel.value" :can-decide="ctx.canAct.value" :now="ctx.now.value" :me="ctx.me.value" />
        <p v-if="!approval" class="j-note">An agent asks for the candidate gate; it appears here for you to approve.</p>
        <div v-if="ctx.canAct.value" class="reject">
          <button v-if="!rejecting" type="button" class="btn sm ghost" :disabled="!approval || store.busy" @click="startReject">Send it back</button>
          <form v-else class="reason" @submit.prevent="reject">
            <label for="reject-reason">What should change?</label>
            <textarea id="reject-reason" ref="reasonField" v-model="reason" class="field" rows="2" maxlength="2048" @keydown.esc.stop="rejecting = false" />
            <div class="reason-actions"><button type="button" class="btn sm ghost" @click="rejecting = false">Cancel</button><button type="submit" class="btn sm deny" :disabled="!reason.trim() || store.busy">Send back</button></div>
          </form>
        </div>
      </GateCard>
      <GateCard v-else-if="!building && journey.stages.find(s => s.key === 'build')?.state === 'done'" eyebrow="Done" :title="`${ctx.releaseLabel.value} candidate`" tone="record">
        <p>The candidate is approved. <button type="button" class="linkish" @click="ctx.view('deploy')">Deploy <AppIcon name="arrow" :size="12" /></button></p>
      </GateCard>
      <GateCard v-else-if="!building" eyebrow="Later" title="Not yet" tone="record"><p>Agents build the release; you approve the candidate.</p></GateCard>
    </div>
  </div>
</template>

<style scoped>
.list-skel { height: 160px; border-radius: 10px; }
.reject { display: grid; }
.reject > .btn { justify-self: start; }
.reason { display: grid; gap: 6px; }
.reason label { font-size: 12.5px; color: var(--ink-2); }
.reason textarea { height: auto; padding: 8px 10px; }
.reason-actions { display: flex; justify-content: flex-end; gap: 6px; }
.btn.deny { background: var(--danger-bg); color: var(--danger); box-shadow: inset 0 0 0 1px var(--danger-line); }
.linkish { display: inline-flex; align-items: center; gap: 4px; padding: 0; border: 0; background: transparent; color: var(--teal-ink); font-weight: 600; cursor: pointer; }
.linkish:focus-visible { border-radius: 4px; box-shadow: var(--focus-ring); }
</style>
