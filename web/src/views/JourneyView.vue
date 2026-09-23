<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
// AEON-39 handoff: coordinator registers JourneyView on /projects/:projectId
// and /projects/:projectId/journey/:stage. Bare project uses the server default.
// #aithema receives projectId/refresh for AIT-34. No cmd/aeon module is needed.
// web/tests/journey.spec.ts adds these routes in its mocked browser harness.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useJourney } from '../stores/journey'
import { useSession } from '../stores/session'
import { stages, profiles, stageLabel, journeyPath, type Stage, type Profile } from '../lib/journey'
import JourneyBar from '../components/journey/JourneyBar.vue'
import GateCard from '../components/journey/GateCard.vue'
import InspirePanel from '../components/journey/InspirePanel.vue'
import ShapePanel from '../components/journey/ShapePanel.vue'
import RequirementsPanel from '../components/journey/RequirementsPanel.vue'
import PlanPanel from '../components/journey/PlanPanel.vue'
import BuildPanel from '../components/journey/BuildPanel.vue'
import DeployPanel from '../components/journey/DeployPanel.vue'
import AccessPanel from '../components/journey/AccessPanel.vue'
import LivePanel from '../components/journey/LivePanel.vue'
import ReleaseWalker from '../components/journey/ReleaseWalker.vue'
import JourneyIcon from '../components/journey/JourneyIcon.vue'
import MarkdownBody from '../components/MarkdownBody.vue'
import { getNode, type WorkNode } from '../lib/api'
const route = useRoute(), router = useRouter(), store = useJourney(), session = useSession()
const person = computed(() => session.identity?.principal.kind === 'person')
const blocked = computed(() => !person.value || store.busy || store.loading || store.stale)
const stage = computed<Stage>(() => stages.includes(route.params.stage as Stage) ? route.params.stage as Stage : store.journey?.stage ?? 'inspire')
const state = computed(() => store.journey?.stages.find(s => s.key === stage.value))
const panels = { inspire: InspirePanel, shape: ShapePanel, requirements: RequirementsPanel, plan: PlanPanel, build: BuildPanel, deploy: DeployPanel, access: AccessPanel, live: LivePanel }
const titles: Record<Stage,string> = { inspire: 'Conversation and sources', shape: 'Shape the idea', requirements: 'Agree what matters', plan: 'Plan the release', build: 'Build the release', deploy: 'Ready for the real world', access: 'The right access', live: 'Out in the world' }
const owners: Record<Stage,string> = { inspire:'Aithema', shape:'Aithema', requirements:'Aithema', plan:'Paimos', build:'Paimos', deploy:'Pharos', access:'Janus', live:'INSPR' }
const gateId = computed(() => state.value?.gate_approval_id || (store.journey?.next_action.stage === stage.value ? store.journey.next_action.approval_request_id : undefined))
const approval = computed(() => store.approvals.find(a => a.id === gateId.value && a.resource_kind === 'node' && (a.resource_id === store.projectId || a.resource_id === store.journey?.current_release_id)))
const walkerOpen = ref(false), initialTicket = ref<string>(), commandDialog = ref<HTMLDialogElement>(), inspectDialog = ref<HTMLDialogElement>()
const inspected = ref<WorkNode>(), inspectError = ref(''), inspecting = ref(false), command = ref(false)
let inspectRequest = 0, previousFocus: HTMLElement | null = null, timer: ReturnType<typeof setInterval> | undefined
async function canonicalRoute() {
  if (store.journey && !stages.includes(route.params.stage as Stage)) await router.replace(journeyPath(store.projectId, store.journey.stage))
}
watch(() => String(route.params.projectId || ''), async id => { walkerOpen.value = false; await store.load(id); await canonicalRoute() }, { immediate:true })
watch(() => route.params.stage, canonicalRoute)
function walk(ticket?: string) { initialTicket.value = ticket; walkerOpen.value = true }
async function openNode(id: string) {
  const version = ++inspectRequest
  previousFocus = document.activeElement as HTMLElement; inspected.value = undefined; inspectError.value = ''; inspecting.value = true
  inspectDialog.value?.showModal()
  try { const value = await getNode(id); if (version === inspectRequest) inspected.value = value }
  catch (e) { if (version === inspectRequest) inspectError.value = e instanceof Error ? e.message : 'Could not load this record.' }
  finally { if (version === inspectRequest) inspecting.value = false }
}
function closeNode() { inspectRequest++; inspectDialog.value?.close(); previousFocus?.focus() }
async function nextAction() {
  const next = store.journey?.next_action
  if (!next || blocked.value || !next.available || next.key === 'wait_for_build') return
  if (stage.value !== next.stage) { await router.push(journeyPath(store.projectId,next.stage)); return }
  if (next.key === 'continue_intake') { await nextTick(); document.querySelector<HTMLElement>('[data-aithema-conversation]')?.focus(); return }
  if (next.key === 'decide') { document.querySelector<HTMLButtonElement>('[role="tablist"][aria-label="Shape"] [role="tab"]:last-child')?.click(); return }
  command.value = true; await nextTick(); commandDialog.value?.showModal()
}
async function execute() {
  const next = store.journey?.next_action
  if (!next || blocked.value || !next.available || next.key === 'continue_intake' || next.key === 'decide' || next.key === 'wait_for_build') return
  const ok = next.key === 'approve_requirements' ? await store.agree() : await store.action(next.key)
  commandDialog.value?.close(); command.value = false
  if (ok && store.journey) await router.push(journeyPath(store.projectId,store.journey.stage))
}
function closeCommand() { commandDialog.value?.close(); command.value = false }
function changeProfile(event: Event) { void store.profile((event.target as HTMLSelectElement).value as Profile) }
onMounted(() => { timer = setInterval(() => { if (!document.hidden && !store.busy && !store.loading && !store.stale && !command.value && !walkerOpen.value) void store.load() },30_000) })
onBeforeUnmount(() => { if (timer) clearInterval(timer); inspectRequest++; store.dispose() })
</script>
<template>
  <section class="journey-face" aria-label="Project journey workspace">
    <header class="journey-heading"><div><p class="eyebrow">{{ stageLabel(stage) }} · {{ state?.state || 'Loading' }} · {{ owners[stage] }}</p><h1>{{ titles[stage] }}</h1></div><div class="profile-control"><div class="j-label"><label for="journey-profile">Profile</label><select id="journey-profile" :value="store.journey?.profile" :disabled="blocked || !store.journey" @change="changeProfile"><option v-for="(profile,key) in profiles" :key="key" :value="key">{{ profile.label }}</option></select></div><span v-if="store.journey" class="j-note" :title="profiles[store.journey.profile].line">{{ profiles[store.journey.profile].line }}</span></div><button class="j-icon" aria-label="Refresh journey" :disabled="store.loading || store.busy" @click="store.load()"><JourneyIcon name="refresh" /></button></header>
    <div v-if="store.error" class="journey-message error" role="alert">{{ store.error }} <button class="j-button" :disabled="store.loading || store.busy" @click="store.load()">Refresh</button></div><div v-else-if="store.notice" class="journey-message" role="status">{{ store.notice }}</div>
    <div v-if="store.loading && !store.journey" class="journey-empty" role="status">Loading project journey…</div><div v-else-if="!store.journey" class="journey-empty"><h2>Journey unavailable</h2><p>Refresh to try this project again.</p></div>
    <div v-else class="stage-workspace" :aria-busy="store.loading || store.busy">
      <div v-if="state?.state === 'later'" class="j-columns one"><section class="j-card"><div class="eyebrow">Later</div><h2>{{ stageLabel(stage) }} comes next in its turn.</h2><p>{{ store.journey.next_action.reason || 'The current stage and its next action are shown below.' }}</p><RouterLink class="j-button" :to="journeyPath(store.projectId,store.journey.stage)">Back to {{ stageLabel(store.journey.stage) }}</RouterLink></section></div>
      <component :is="panels[stage]" v-else :can-edit="person && !store.loading && !store.stale" @walk="walk" @open-node="openNode">
        <template v-if="stage === 'inspire'"><slot name="aithema" :project-id="store.projectId" :refresh="store.load" /></template>
        <template v-else><GateCard v-if="gateId || (state?.state === 'current' && store.journey.next_action.key !== 'wait_for_build')" :title="store.journey.next_action.stage === stage ? store.journey.next_action.label : `${stageLabel(stage)} gate`" :approval="approval" :requested="!!gateId" :busy="blocked" :can-decide="person" :independent="store.journey.profile === 'enterprise' && stage === 'build'" @decide="store.decide" /><section v-if="store.journey.next_action.stage === stage && store.journey.next_action.reason" class="j-card"><div class="eyebrow">Next action</div><p>{{ store.journey.next_action.reason }}</p></section></template>
      </component>
    </div>
    <JourneyBar v-if="store.journey" :journey="store.journey" :viewed="stage" :title="store.project?.title || 'Project'" :disabled="blocked" @action="nextAction" />
    <ReleaseWalker v-if="walkerOpen && store.walker" :initial-ticket="initialTicket" :can-edit="person" @close="walkerOpen = false" @open-node="openNode" />
    <dialog ref="commandDialog" class="journey-dialog command-dialog" aria-label="Confirm journey action" @cancel.prevent="closeCommand"><template v-if="command && store.journey"><div class="eyebrow">{{ stageLabel(store.journey.next_action.stage) }}</div><h2>{{ store.journey.next_action.label }}</h2><p>{{ store.journey.next_action.reason || 'Apply this action to the current project revision.' }}</p><p class="j-note">The server checks the current revision and any required approval before recording the change.</p><div class="j-actions"><button class="j-button" :disabled="store.busy" @click="closeCommand">Cancel</button><button class="j-button primary" :disabled="blocked" @click="execute">Confirm action</button></div></template></dialog>
    <dialog ref="inspectDialog" class="journey-dialog inspect-dialog" aria-label="Node record" @cancel.prevent="closeNode"><div class="j-card-head"><h2>{{ inspected?.key || 'Record' }}</h2><button class="j-icon" aria-label="Close record" @click="closeNode"><JourneyIcon name="close" /></button></div><p v-if="inspecting" role="status">Loading record…</p><p v-if="inspectError" class="error" role="alert">{{ inspectError }}</p><template v-if="inspected"><h2>{{ inspected.title }}</h2><p class="j-meta">{{ inspected.state }} · updated {{ new Date(inspected.updated_at).toLocaleString() }}</p><MarkdownBody :body="inspected.body" /></template></dialog>
  </section>
</template>
<style>
/* Owned, namespaced face styles. Shared tokens, shell and router stay intact. */
.journey-face { height:100%; min-height:0; display:flex; flex-direction:column; overflow:hidden; }
.journey-heading { display:flex; gap:24px; align-items:center; padding:22px 28px 18px; flex:none; }.journey-heading h1 { font-size:30px; margin-top:5px; }.journey-heading>div:first-child { flex:1; min-width:0; }.profile-control { width:285px; }.profile-control .j-label { flex-direction:row; align-items:center; gap:12px; margin:0; }.profile-control .j-note { display:block; font-size:10px; margin-top:6px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.stage-workspace { flex:1; min-height:0; overflow:hidden; padding:0 28px 22px; }.j-columns { display:grid; grid-template-columns:minmax(0,1.45fr) minmax(0,1fr); gap:22px; height:100%; min-height:0; }.j-columns.one { grid-template-columns:minmax(0,1fr); max-width:1000px; margin-inline:auto; overflow:auto; }.j-scroll { min-height:0; min-width:0; overflow:auto; padding:1px 4px 20px; scrollbar-gutter:stable; }.j-stack { display:flex; flex-direction:column; gap:18px; }.j-card { background:linear-gradient(155deg,var(--surface),var(--glass)); border:1px solid var(--glass-edge); border-radius:var(--radius); box-shadow:var(--shadow); padding:20px 22px; min-width:0; height:fit-content; }.j-card+.j-card { margin-top:18px; }.j-stack>.j-card+.j-card { margin-top:0; }.j-card h2 { font-size:19px; font-weight:500; margin-bottom:10px; }.j-card p { font-size:13px; }.j-card-head { display:flex; align-items:center; justify-content:space-between; gap:16px; margin-bottom:12px; min-width:0; }.j-card-head h2 { margin:0; }
:is(.journey-face,.journey-dialog) .j-button { border:1px solid var(--line-2); background:var(--glass); color:var(--ink); border-radius:999px; padding:7px 14px; min-height:34px; display:inline-flex; align-items:center; justify-content:center; gap:7px; font:600 12px var(--font); cursor:pointer; white-space:nowrap; }.j-button:hover { background:var(--aqua-3); box-shadow:0 0 12px var(--glass-rim); }.j-button:disabled { cursor:not-allowed; }.j-button.primary { background:var(--teal); color:var(--button-ink); }.j-actions { display:flex; gap:10px; flex-wrap:wrap; margin-top:16px; }.j-icon { display:inline-grid; place-items:center; width:34px; height:34px; padding:0; border:1px solid transparent; border-radius:8px; background:transparent; color:var(--ink-2); flex:none; }.j-icon:hover,.j-icon[aria-pressed="true"] { background:var(--aqua-3); color:var(--teal-ink); }.j-icon:disabled { cursor:not-allowed; }.j-text { border:0; background:none; color:var(--teal); font:inherit; padding:0; text-align:left; }.j-text:hover { text-decoration:underline; }.j-check { display:inline-grid; place-items:center; width:19px; height:19px; padding:0; background:var(--surface); border:1px solid var(--line-2); border-radius:4px; color:var(--teal); flex:none; }.j-check[aria-checked="true"],.j-check[aria-checked="mixed"] { border-color:var(--teal); background:var(--aqua-3); }.j-check:disabled { cursor:not-allowed; }
.j-label { display:flex; flex-direction:column; gap:6px; font:11px/1.4 var(--mono); color:var(--ink-2); margin:14px 0; }.j-label :is(input,textarea,select) { width:100%; min-width:0; border:1px solid var(--line-2); border-radius:8px; background:var(--surface); color:var(--ink); padding:9px 11px; font:13px/1.5 var(--font); }.j-label textarea { resize:vertical; min-height:62px; }.j-label select { cursor:pointer; }.j-note { color:var(--ink-2); font-size:12px; margin-top:12px; }.j-meta { font:10px/1.5 var(--mono); color:var(--ink-2); }.j-kv { display:grid; grid-template-columns:105px minmax(0,1fr); gap:8px 12px; margin:16px 0; font-size:12px; }.j-kv dt { font:10px/1.5 var(--mono); color:var(--ink-2); }.j-kv dd { margin:0; overflow-wrap:anywhere; }.j-tabs { display:flex; gap:8px; border-bottom:1px solid var(--line); padding-bottom:14px; margin-bottom:18px; }.j-tabs [aria-selected="true"] { background:var(--aqua-3); color:var(--teal-ink); }
.journey-message { flex:none; display:flex; align-items:center; gap:12px; font-size:12px; padding:8px 28px; }.journey-empty { flex:1; display:flex; flex-direction:column; align-items:center; justify-content:center; gap:14px; }.journey-dialog { color:var(--ink); background:var(--surface); border:1px solid var(--glass-rim); border-radius:var(--radius); padding:24px; box-shadow:var(--shadow); }.journey-dialog::backdrop { background:color-mix(in srgb,var(--canvas) 75%,transparent); backdrop-filter:blur(8px); }.command-dialog { width:min(520px,90vw); }.command-dialog p { margin-top:14px; font-size:13px; }.inspect-dialog { width:min(800px,90vw); max-height:85dvh; overflow:auto; }.inspect-dialog h2 { font-size:22px; }.inspect-dialog .j-meta { margin:12px 0; }
@media(max-width:850px) { .journey-heading { padding:18px; gap:12px; }.journey-heading h1 { font-size:24px; }.profile-control { width:180px; }.profile-control .j-note { display:none; }.stage-workspace { padding:0 14px 14px; }.j-columns { grid-template-columns:minmax(0,1fr); overflow:auto; gap:16px; align-content:start; }.j-columns>.j-scroll { overflow:visible; }.j-card { padding:18px; } }
@media(max-width:520px) { .journey-heading { flex-wrap:wrap; }.journey-heading>div:first-child { flex-basis:100%; }.profile-control { width:auto; flex:1; }.journey-heading h1 { font-size:22px; } }
@media(prefers-reduced-motion:reduce) { .journey-face *, .journey-dialog * { scroll-behavior:auto!important; transition:none!important; animation:none!important; } }
</style>
