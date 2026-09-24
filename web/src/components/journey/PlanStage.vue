<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { ACTION_LONG, gateApprovals, hours, offeredApproval, RELEASE_STATE_LABEL } from '../../lib/journey'
import { useJourneyContext } from '../../lib/journeyContext'
import { plural, statusMeta } from '../../lib/work'
import { useJourney } from '../../stores/journey'
import AppIcon from '../AppIcon.vue'
import GateApprovals from './GateApprovals.vue'
import GateCard from './GateCard.vue'
import ReleaseList from './ReleaseList.vue'
import ReleaseTickets from './ReleaseTickets.vue'

// Plan: the tickets of the release, grouped by feature. While the current
// release is planning, ticked tickets form it and unticked ones stay in the
// backlog; the walker shows them one by one with their screens. Other releases
// read the same way, without the boxes.
const ctx = useJourneyContext()
const store = useJourney()
const journey = computed(() => ctx.journey.value)
const walker = computed(() => ctx.data.walker.value.value)
const status = computed(() => ctx.data.walker.status.value)
const stats = computed(() => ctx.plan.stats.value)
const planning = computed(() => journey.value.next_action.key === 'start_build')
const approvals = computed(() => gateApprovals(ctx.approvals.value, 'build', journey.value.current_release_id))
const approval = computed(() => offeredApproval(ctx.approvals.value, journey.value, 'build', ctx.now.value))
const releases = computed(() => ctx.data.releases.value)
const epicsWithout = computed(() => stats.value.emptyFeatures.map(g => g.feature!.key))
const reqRevision = computed(() => journey.value.requirements_revision)
</script>

<template>
  <div class="j-grid">
    <div class="j-col">
      <!-- No release shown: the journey has none open yet. -->
      <section v-if="!ctx.release.value" class="j-card" aria-labelledby="plan-none">
        <header class="j-card-head"><p id="plan-none" class="eyebrow">Tickets · the release</p></header>
        <div class="j-empty">
          <span class="j-empty-icon"><AppIcon name="layers" :size="16" /></span>
          <strong>No release is open</strong>
          <span>{{ journey.next_action.key === 'start_build' && journey.next_action.reason ? journey.next_action.reason : 'Agreeing the requirements opens release 1; its tickets are chosen here.' }}</span>
          <span v-if="releases.length">Earlier releases of this project are listed below; open one to walk through its tickets.</span>
        </div>
      </section>
      <section v-else class="j-card" aria-labelledby="plan-tickets">
        <header class="j-card-head">
          <p id="plan-tickets" class="eyebrow">{{ ctx.editable.value ? 'Tickets · ticked ones form the release' : `Tickets · ${ctx.releaseLabel.value}` }}</p>
          <span v-if="walker" class="j-count">{{ ctx.editable.value ? `${stats.inRelease} in release · ${stats.backlog} in backlog` : plural(stats.inRelease, 'ticket') }}</span>
          <button type="button" class="btn sm" :disabled="!walker?.tickets.length" data-tip="Walk through the tickets with their screens · w" aria-keyshortcuts="w" @click="ctx.walk()"><AppIcon name="expand" :size="13" />Full screen</button>
        </header>
        <p v-if="status === 'loading'" class="skeleton list-skel" role="status" aria-label="Loading the release" />
        <div v-else-if="status === 'error'" class="j-empty" role="alert"><strong>This release could not be loaded</strong><span>{{ ctx.data.walker.error.value }}</span>
          <button type="button" class="btn sm" @click="ctx.data.loadWalker(ctx.release.value!.id, true)"><AppIcon name="refresh" :size="13" />Try again</button>
        </div>
        <div v-else-if="walker && !walker.tickets.length" class="j-empty"><strong>No tickets yet</strong><span>Agreeing the requirements generates the tickets; tickets added by hand join the backlog.</span></div>
        <ReleaseTickets v-else-if="walker" :plan="ctx.plan" :editable="ctx.editable.value" :project-key="ctx.project.value.routeKey" :work-by-id="ctx.data.workById.value" @walk="t => ctx.walk(t.key)" @open="ctx.open" />
      </section>
      <section v-if="releases.length" class="j-card" aria-labelledby="plan-releases">
        <header class="j-card-head"><p id="plan-releases" class="eyebrow">Releases · {{ releases.length }}</p></header>
        <ReleaseList :releases="releases" :current-id="journey.current_release_id" :selected-id="ctx.release.value?.id ?? null" :now="ctx.now.value" :limit="6" @select="r => ctx.selectRelease(r.key)" />
      </section>
    </div>
    <div class="j-col">
      <GateCard
        v-if="planning && ctx.current.value" eyebrow="Decision" :title="ctx.releaseLabel.value"
        :action="{ label: ctx.next.value.label, disabled: ctx.next.value.disabled, busy: ctx.next.value.busy, tip: ctx.next.value.tip }" @act="ctx.runNext()"
      >
        <div class="j-stats">
          <div class="j-stat"><b>{{ stats.inRelease }}</b><span>{{ stats.inRelease === 1 ? 'ticket' : 'tickets' }}</span></div>
          <div class="j-stat" :class="{ warn: stats.unestimated }"><b>{{ stats.hours ? hours(stats.hours) : '—' }}</b><span>{{ stats.unestimated ? `${stats.unestimated} not estimated` : 'estimate' }}</span></div>
          <div class="j-stat"><b>{{ stats.backlog }}</b><span>in the backlog</span></div>
        </div>
        <ul class="j-checks">
          <li v-if="epicsWithout.length"><AppIcon name="alert" :size="13" class="warn" /><span>{{ epicsWithout.join(', ') }} {{ epicsWithout.length === 1 ? 'has' : 'have' }} no ticket in this release</span></li>
          <li v-else-if="walker?.features.length"><AppIcon name="check" :size="13" class="ok" /><span>Every feature has a ticket in this release</span></li>
          <li v-if="stats.unestimated"><AppIcon name="alert" :size="13" class="warn" /><span>{{ plural(stats.unestimated, 'ticket') }} in the release {{ stats.unestimated === 1 ? 'has' : 'have' }} no estimate</span></li>
          <li><AppIcon name="info" :size="13" class="info" /><span>Starting the build agrees requirements revision {{ reqRevision }}</span></li>
          <li v-if="!journey.next_action.available && journey.next_action.reason"><AppIcon name="alert" :size="13" class="warn" /><span>{{ journey.next_action.reason }}</span></li>
        </ul>
        <GateApprovals gate="build" :approvals="approvals" :on="ctx.releaseLabel.value" :can-decide="ctx.canAct.value" :now="ctx.now.value" :me="ctx.me.value" />
        <p v-if="!approval" class="j-note">An agent asks for the build gate on {{ ctx.releaseLabel.value }}; it appears here for you to approve.</p>
        <p class="j-note">{{ ACTION_LONG.start_build }}</p>
      </GateCard>
      <GateCard v-else-if="planning" eyebrow="Decision" title="Start build" tone="blocked">
        <p>{{ journey.next_action.reason || 'No release is open.' }}</p>
        <p class="j-note">The build starts from the current release. Until one is open, earlier releases can be read but not changed.</p>
      </GateCard>
      <GateCard v-if="ctx.release.value && !(planning && ctx.current.value)" :eyebrow="ctx.current.value ? 'Planned' : 'Release'" :title="ctx.releaseLabel.value" tone="record">
        <dl class="j-kv">
          <dt>Version</dt><dd :class="{ faint: !ctx.release.value.version }">{{ ctx.release.value.version ?? 'Set when it goes live' }}</dd>
          <dt>Key</dt><dd class="mono">{{ ctx.release.value.key }}</dd>
          <dt>State</dt><dd>{{ walker && ctx.current.value ? RELEASE_STATE_LABEL[walker.state] : statusMeta(ctx.release.value.state).label }}</dd>
          <dt>Tickets</dt><dd>{{ walker ? plural(stats.inRelease, 'ticket') : '…' }}</dd>
        </dl>
        <p v-if="!ctx.current.value" class="j-note">An earlier release: its tickets are shown as they were planned.</p>
        <p v-if="journey.stage !== 'plan'"><button type="button" class="linkish" @click="ctx.view(journey.stage)">Where the journey is now <AppIcon name="arrow" :size="12" /></button></p>
      </GateCard>
      <GateCard v-if="!planning && !ctx.release.value" eyebrow="Later" title="Not yet" tone="record"><p>Choosing the tickets of the release. Agreeing the requirements opens release 1.</p></GateCard>
      <p v-if="store.busy || ctx.plan.saving.value" class="saving" role="status">Saving the plan…</p>
    </div>
  </div>
</template>

<style scoped>
.list-skel { height: 180px; border-radius: 10px; }
.saving { font-size: 12px; color: var(--ink-3); }
.linkish { display: inline-flex; align-items: center; gap: 4px; padding: 0; border: 0; background: transparent; color: var(--teal-ink); font-weight: 600; cursor: pointer; }
.linkish:focus-visible { border-radius: 4px; box-shadow: var(--focus-ring); }
</style>
