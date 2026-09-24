<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Parked (AEON-70, 2026-09-24): quotes and organisations will be ported from Markus's current classic Paimos quote builder; this file is not routed or linked. -->
<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { createQuote, createRelation, type Quote } from '../../lib/business'
import { useBusiness } from '../../stores/business'
import { useProjects } from '../../stores/projects'
import AppIcon from './BizIcon.vue'
import PickerMenu, { type PickOption } from './PickerMenu.vue'

// A quote belongs to one customer organisation and one project. When the two are
// not linked yet, creating the quote first links the organisation as the
// project's customer (a normal graph link anyone can see and remove).
const emit = defineEmits<{ created: [quote: Quote] }>()
const business = useBusiness()
const projects = useProjects()
const dialog = ref<HTMLDialogElement>()
const titleInput = ref<HTMLInputElement>()
const title = ref('')
const orgId = ref('')
const projectId = ref('')
const busy = ref(false)
const error = ref('')
const picker = ref<{ kind: 'org' | 'project'; anchor: HTMLElement } | null>(null)
let opener: HTMLElement | null = null

const org = computed(() => business.organisation(orgId.value))
const project = computed(() => projects.byId(projectId.value))
const linkedProjects = computed(() => business.links.get(orgId.value)?.projects ?? [])
const linked = computed(() => !!projectId.value && linkedProjects.value.includes(projectId.value))
const orgOptions = computed<PickOption[]>(() => business.organisations.map(o => ({ value: o.id, label: o.title, note: typeof o.fields.website === 'string' ? o.fields.website : undefined, icon: 'building' })))
const projectOptions = computed<PickOption[]>(() => projects.projects.filter(p => !p.archived || linkedProjects.value.includes(p.id))
  .sort((a, b) => Number(linkedProjects.value.includes(b.id)) - Number(linkedProjects.value.includes(a.id)) || a.title.localeCompare(b.title))
  .map(p => ({ value: p.id, label: p.title, badge: p.routeKey, hint: linkedProjects.value.includes(p.id) ? 'customer' : undefined })))
const ready = computed(() => !!title.value.trim() && !!orgId.value && !!projectId.value)

async function open(preset: { orgId?: string; projectId?: string } = {}) {
  opener = document.activeElement as HTMLElement
  title.value = ''; orgId.value = preset.orgId ?? ''; projectId.value = preset.projectId ?? ''; error.value = ''; busy.value = false
  void business.loadCRM(); void projects.load()
  if (orgId.value) void business.loadLinks([orgId.value])
  dialog.value?.showModal()
  await nextTick(); titleInput.value?.focus()
}
function close() { picker.value = null; dialog.value?.close(); opener?.focus({ preventScroll: true }) }
function openPicker(kind: 'org' | 'project', event: MouseEvent) { picker.value = picker.value?.kind === kind ? null : { kind, anchor: event.currentTarget as HTMLElement } }
function closePicker(restore: boolean) { const anchor = picker.value?.anchor; picker.value = null; if (restore) anchor?.focus() }
async function chooseOrg(option: PickOption) {
  orgId.value = option.value; closePicker(true)
  await business.loadLinks([option.value])
  const only = business.links.get(option.value)?.projects ?? []
  if (!projectId.value && only.length === 1) projectId.value = only[0]
}
async function createOrg(name: string) {
  try {
    const node = await business.createCRMNode('organisation', name)
    business.upsertNode('organisations', node)
    business.setLinks(node.id, { contacts: [], projects: [], quotes: [] })
    orgId.value = node.id
  } catch (e) { error.value = e instanceof Error ? e.message : 'The organisation was not created.' }
  closePicker(true)
}
function chooseProject(option: PickOption) { projectId.value = option.value; closePicker(true) }
async function submit() {
  if (!ready.value || busy.value) return
  busy.value = true; error.value = ''
  try {
    if (!linked.value) {
      await createRelation(orgId.value, projectId.value, 'customer_of')
      const links = business.links.get(orgId.value) ?? { contacts: [], projects: [], quotes: [] }
      business.setLinks(orgId.value, { ...links, projects: [...links.projects, projectId.value] })
    }
    const quote = await createQuote({ title: title.value.trim(), project_node_id: projectId.value, customer_org_node_id: orgId.value })
    business.upsertQuote(quote)
    const links = business.links.get(orgId.value)
    if (links) business.setLinks(orgId.value, { ...links, quotes: [...links.quotes, quote.quote_node_id] })
    close()
    emit('created', quote)
  } catch (e) { error.value = e instanceof Error ? e.message : 'The quote was not created.' }
  finally { busy.value = false }
}
function backdrop(event: MouseEvent) { if (event.target === dialog.value) close() }
defineExpose({ open })
</script>

<template>
  <dialog id="new-quote-dialog" ref="dialog" class="create" aria-labelledby="new-quote-title" @cancel.prevent="picker ? closePicker(true) : close()" @click="backdrop">
    <form class="create-card" @submit.prevent="submit">
      <header>
        <h2 id="new-quote-title">New quote</h2>
        <button type="button" class="icon-btn sm" aria-label="Close" @click="close"><AppIcon name="close" :size="14" /></button>
      </header>
      <label class="row">
        <span class="label">Title</span>
        <input ref="titleInput" v-model="title" class="field" placeholder="What the offer is about" maxlength="512" autocomplete="off" />
      </label>
      <div class="row">
        <span class="label" aria-hidden="true">Customer</span>
        <button type="button" class="pick" :class="{ unset: !org }" :aria-label="`Customer: ${org?.title ?? 'choose an organisation'}`" aria-haspopup="dialog" :aria-expanded="picker?.kind === 'org'" @click="openPicker('org', $event)">
          <AppIcon name="building" :size="14" /><span>{{ org?.title ?? 'Choose an organisation' }}</span><AppIcon name="chevron" :size="12" class="chev" />
        </button>
      </div>
      <div class="row">
        <span class="label" aria-hidden="true">Project</span>
        <button type="button" class="pick" :class="{ unset: !project }" :aria-label="`Project: ${project?.title ?? 'choose a project'}`" aria-haspopup="dialog" :aria-expanded="picker?.kind === 'project'" @click="openPicker('project', $event)">
          <span v-if="project" class="key-badge">{{ project.routeKey }}</span><AppIcon v-else name="folder" :size="14" /><span>{{ project?.title ?? 'Choose a project' }}</span><AppIcon name="chevron" :size="12" class="chev" />
        </button>
      </div>
      <p v-if="org && project && !linked" class="note"><AppIcon name="link" :size="13" />Creating it also links {{ org.title }} as the customer of {{ project.title }}.</p>
      <p v-if="error" class="error-line" role="alert"><AppIcon name="alert" :size="13" />{{ error }}</p>
      <footer>
        <span class="hint">The first version comes next: lines, rates and terms.</span>
        <button type="button" class="btn" @click="close">Cancel</button>
        <button type="submit" class="btn primary" :disabled="!ready || busy">{{ busy ? 'Creating…' : 'Create quote' }}</button>
      </footer>
    </form>
    <PickerMenu v-if="picker?.kind === 'org'" :anchor="picker.anchor" title="Customer" :options="orgOptions" :current="orgId" placeholder="Find or add an organisation…" to="#new-quote-dialog" :create-label="term => `Add organisation “${term}”`" @choose="chooseOrg" @create="createOrg" @close="closePicker" />
    <PickerMenu v-if="picker?.kind === 'project'" :anchor="picker.anchor" title="Project" :options="projectOptions" :current="projectId" placeholder="Find a project…" to="#new-quote-dialog" @choose="chooseProject" @close="closePicker" />
  </dialog>
</template>

<style scoped>
.create { width: min(520px, calc(100vw - 24px)); padding: 0; border: 0; background: transparent; color: var(--ink); overflow: visible; }
.create::backdrop { background: var(--scrim); backdrop-filter: blur(2px); }
.create-card { display: grid; gap: 14px; padding: 20px 22px 18px; border-radius: var(--radius); border: 1px solid var(--glass-edge); background: linear-gradient(165deg, var(--surface-raised), var(--surface-raised-2)); box-shadow: var(--shadow-pop), var(--shadow); }
header { display: flex; align-items: center; justify-content: space-between; }
h2 { font-size: 18px; }
.row { display: grid; grid-template-columns: 92px minmax(0, 1fr); align-items: center; gap: 12px; }
.label { font: 500 10.5px/1 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.pick { display: flex; align-items: center; gap: 8px; min-width: 0; height: 34px; padding: 0 10px 0 11px; border: 1px solid var(--glass-edge); border-radius: var(--radius-s); background: var(--field-bg); box-shadow: var(--field-inset), 0 0 0 1px var(--line); color: var(--ink); font-size: 13.5px; text-align: left; }
.pick span:not(.key-badge) { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pick svg { color: var(--ink-3); flex-shrink: 0; }
.pick.unset { color: var(--ink-3); }
.pick:hover { box-shadow: var(--field-inset), 0 0 0 1px var(--glass-rim); }
.pick:focus-visible { box-shadow: var(--focus-ring); }
.pick .key-badge { height: 20px; padding: 0 6px; font-size: 10.5px; }
.note { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-radius: 10px; background: var(--aqua-wash); color: var(--teal-ink); font-size: 12.5px; }
.error-line { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-radius: 10px; background: var(--danger-bg); color: var(--danger); font-size: 13px; }
footer { display: flex; align-items: center; justify-content: flex-end; gap: 8px; margin-top: 4px; }
.hint { flex: 1; font-size: 12px; color: var(--ink-3); }
@media (max-width: 600px) {
  .row { grid-template-columns: minmax(0, 1fr); gap: 6px; }
  .pick, .row .field { height: 44px; font-size: 16px; }
  footer { flex-wrap: wrap; }
  .hint { flex-basis: 100%; }
  footer .btn { flex: 1; height: 44px; }
}
</style>
