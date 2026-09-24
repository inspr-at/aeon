<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import { ACTION_LONG, gateApprovals, offeredApproval, PLUGIN_GATE } from '../../lib/journey'
import { useJourneyContext } from '../../lib/journeyContext'
import AppIcon from '../AppIcon.vue'
import GateApprovals from './GateApprovals.vue'
import GateCard from './GateCard.vue'
import HandoffList from './HandoffList.vue'

// Deploy: Pharos applies the release after your approval. Its deploy step has
// gates of its own; launch admission among them fails closed on this server (no
// launch-checks provider is installed), so a deployment cannot be admitted yet.
const ctx = useJourneyContext()
const journey = computed(() => ctx.journey.value)
const next = computed(() => journey.value.next_action)
const pharos = computed(() => ctx.plugins.value.find(p => p.id === 'pharos') ?? null)
const deployGates = computed(() => pharos.value?.workflow_steps?.find(s => s.key === 'deploy')?.gates ?? [])
const handoffs = computed(() => ctx.data.handoffs.value.value.filter(h => h.stage === 'deploy'))
const deployed = computed(() => handoffs.value.some(h => h.operation === 'deploy' && h.state === 'succeeded'))
const deciding = computed(() => next.value.key === 'approve_deploy' || next.value.key === 'retry_deploy')
const approvals = computed(() => gateApprovals(ctx.approvals.value, 'deploy', journey.value.current_release_id))
const approval = computed(() => offeredApproval(ctx.approvals.value, journey.value, 'deploy', ctx.now.value))
const state = computed(() => journey.value.stages.find(s => s.key === 'deploy')?.state ?? 'later')
</script>

<template>
  <div class="j-grid">
    <div class="j-col">
      <section class="j-card" aria-labelledby="deploy-target">
        <header class="j-card-head"><p id="deploy-target" class="eyebrow">Target · Pharos</p><span class="j-chip" :class="pharos?.installation?.enabled ? 'ok' : 'gold'">{{ pharos ? pharos.installation?.enabled ? 'Enabled' : 'Not enabled' : 'Not installed' }}</span></header>
        <dl class="j-kv">
          <dt>Release</dt><dd>{{ ctx.release.value ? `${ctx.releaseLabel.value} · ${ctx.release.value.key}` : 'None is ready to deploy' }}</dd>
          <dt>Plugin</dt><dd :class="{ faint: !pharos }">{{ pharos ? `Pharos deploys and verifies (${pharos.owner})` : 'Pharos is not installed on this server' }}</dd>
          <dt>Checks</dt>
          <dd>
            <ul class="j-checks">
              <li v-for="gate in deployGates" :key="gate">
                <AppIcon :name="gate === 'launch_admission' ? 'close' : gate === 'person_decision' ? 'user' : 'info'" :size="13" :class="gate === 'launch_admission' ? 'bad' : 'info'" />
                <span>{{ PLUGIN_GATE[gate] ?? gate.replace(/_/g, ' ') }}<template v-if="gate === 'launch_admission'"> · closed on this server</template><template v-else-if="gate === 'person_decision'"> · the deployment gate below</template></span>
              </li>
              <li v-if="!deployGates.length"><AppIcon name="info" :size="13" class="info" /><span>Pharos lists its checks once it is installed.</span></li>
            </ul>
          </dd>
        </dl>
      </section>
      <section class="j-card" aria-labelledby="deploy-handoffs">
        <header class="j-card-head"><p id="deploy-handoffs" class="eyebrow">Handoffs · deploy and verify</p></header>
        <HandoffList :handoffs="handoffs" :now="ctx.now.value" empty="No deployment was handed to Pharos yet. After you approve the deployment, Pharos reports each attempt here." />
      </section>
    </div>
    <div class="j-col">
      <GateCard eyebrow="Blocked" title="Launch admission is closed" tone="blocked">
        <p>Pharos launch checks are unavailable: this server has no launch-checks provider, so launch admission fails closed and no release can be admitted for deployment.</p>
        <p class="j-note">Approving the deployment still records your decision; the host applies the release once admission can succeed.</p>
      </GateCard>
      <GateCard
        v-if="deciding" eyebrow="Decision" :title="next.key === 'retry_deploy' ? 'The host did not apply it' : 'Approve deployment'"
        :action="{ label: ctx.next.value.label, disabled: ctx.next.value.disabled, busy: ctx.next.value.busy, tip: ctx.next.value.tip }" @act="ctx.runNext()"
      >
        <p>{{ ACTION_LONG[next.key] }}</p>
        <p v-if="!next.available && next.reason" class="j-note">{{ next.reason }}</p>
        <GateApprovals gate="deploy" :approvals="approvals" :on="ctx.releaseLabel.value" :can-decide="ctx.canAct.value" :now="ctx.now.value" :me="ctx.me.value" />
        <p v-if="!approval" class="j-note">An agent asks for the deployment gate; it appears here for you to approve.</p>
      </GateCard>
      <GateCard v-else-if="state === 'done' || deployed" eyebrow="Deployed" :title="ctx.releaseLabel.value" tone="record"><p>Pharos applied and verified the release.</p></GateCard>
      <GateCard v-else eyebrow="Later" title="Not yet" tone="record"><p>The host applies the release after your approval.</p></GateCard>
    </div>
  </div>
</template>
