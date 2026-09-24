<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import { errorText, setCooperation, setDocumentMetadata, type Cooperation, type Related, type RelatedDocument, type RelatedProject } from '../../lib/crm'
import '../../styles/crm.css'
import { contentUrl, deleteAttachment, uploadAttachment } from '../../lib/attachments'
import { confirmAction } from '../../lib/confirm'
import { formatAmount } from '../business/money'
import { formatSpan } from '../business/duration'
import { sentenceCase, statusMeta } from '../../lib/work'
import { useProjects } from '../../stores/projects'
import { useBusiness } from '../../stores/business'
import AppIcon from '../AppIcon.vue'
import BizIcon from '../business/BizIcon.vue'
import StatusIcon from '../work/StatusIcon.vue'

// What the customer is part of: its projects, quotes and the hours booked on
// those projects, each a way into its own page. Documents list what is on file.
const props = defineProps<{ related: Related | null; customerId: string; admin: boolean; error?: string }>()
const emit = defineEmits<{ retry: []; changed: [] }>()
const projects = useProjects()
const business = useBusiness()
void projects.load()
const projectLink = (id: string, key: string) => `/p/${encodeURIComponent(projects.byId(id)?.routeKey ?? key)}`
const projectTitle = (id: string) => props.related?.projects.find(p => p.id === id)?.title ?? projects.byId(id)?.title ?? 'A project'
const money = (amount: string, currency: string) => { try { return `${formatAmount(amount, currency)} ${currency}` } catch { return `${amount} ${currency}` } }
const hours = computed(() => [...(props.related?.hours ?? [])].sort((a, b) => b.duration_seconds - a.duration_seconds))
const totalSeconds = computed(() => hours.value.reduce((sum, h) => sum + h.duration_seconds, 0))
const quotes = computed(() => [...(props.related?.quotes ?? [])].sort((a, b) => Number(a.archived) - Number(b.archived) || (b.offer_no ?? '').localeCompare(a.offer_no ?? '')))
const day = (value: string | null) => value ? new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${value.slice(0, 10)}T00:00:00Z`)) : ''
// A range keeps its dash with both dates (no line starts or ends with it).
const validity = (from: string | null, until: string | null) => from && until ? `${day(from)}\u00a0–\u00a0${day(until)}` : from ? `from ${day(from)}` : until ? `until ${day(until)}` : ''
const docMeta = (d: Related['documents'][number]) => [d.category && sentenceCase(d.category), d.status && sentenceCase(d.status), validity(d.valid_from, d.valid_until)].filter(Boolean) as string[]
const uploadTarget = ref('')
const busy = ref(false)
const actionError = ref('')
const editingDocument = ref<string | null>(null)
const docDraft = ref({ title: '', category: '', status: 'draft', valid_from: '', valid_until: '' })
const editingProject = ref<string | null>(null)
const cooperationDraft = ref<Omit<Cooperation, 'revision'>>({ engagement: '', ownership: '', environment_responsibility: '', sla: '', report_contract: '' })
function editDocument(doc: RelatedDocument) {
  editingDocument.value = doc.attachment_id; editingProject.value = null; actionError.value = ''
  docDraft.value = { title: doc.title, category: doc.category, status: doc.status, valid_from: doc.valid_from ?? '', valid_until: doc.valid_until ?? '' }
}
async function saveDocument(doc: RelatedDocument) {
  if (busy.value) return
  busy.value = true; actionError.value = ''
  try {
    await setDocumentMetadata(doc, { ...docDraft.value, valid_from: docDraft.value.valid_from || null, valid_until: docDraft.value.valid_until || null })
    editingDocument.value = null; emit('changed')
  } catch (e) { actionError.value = errorText(e) }
  finally { busy.value = false }
}
async function removeDocument(doc: RelatedDocument) {
  if (busy.value || !(await confirmAction({ title: `Delete ${doc.title || doc.name}?`, body: 'This removes the file from this customer or project.', confirmLabel: 'Delete file', danger: true }))) return
  busy.value = true; actionError.value = ''
  try { await deleteAttachment(doc.attachment_id); editingDocument.value = null; emit('changed') }
  catch (e) { actionError.value = errorText(e) }
  finally { busy.value = false }
}
async function upload(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file || busy.value) return
  busy.value = true; actionError.value = ''
  try { await uploadAttachment(uploadTarget.value || props.customerId, file); emit('changed') }
  catch (e) { actionError.value = errorText(e) }
  finally { busy.value = false; input.value = '' }
}
function editProject(project: RelatedProject) {
  editingProject.value = project.id; editingDocument.value = null; actionError.value = ''
  const current = project.cooperation ?? {} as Cooperation
  cooperationDraft.value = { engagement: current.engagement ?? '', ownership: current.ownership ?? '', environment_responsibility: current.environment_responsibility ?? '', sla: current.sla ?? '', report_contract: current.report_contract ?? '' }
}
async function saveProject(project: RelatedProject) {
  if (busy.value) return
  busy.value = true; actionError.value = ''
  try { await setCooperation(project, cooperationDraft.value); editingProject.value = null; emit('changed') }
  catch (e) { actionError.value = errorText(e) }
  finally { busy.value = false }
}
</script>

<template>
  <section class="crm-card glass-card related" aria-labelledby="related-title">
    <header class="card-head">
      <span class="card-icon" aria-hidden="true"><AppIcon name="layers" :size="15" /></span>
      <div class="card-titles">
        <h2 id="related-title">Projects, quotes and hours</h2>
        <p class="card-lead">Where this customer’s work lives.</p>
      </div>
    </header>

    <p v-if="error" class="f-error" role="alert"><AppIcon name="alert" :size="14" /><span>{{ error }}</span><button type="button" class="btn sm" @click="emit('retry')">Try again</button></p>
    <div v-else-if="!related" class="sk" aria-hidden="true"><span v-for="i in 3" :key="i" class="skeleton" /></div>
    <div v-else class="groups">
      <div class="group" role="group" aria-labelledby="rel-projects">
        <h3 id="rel-projects" class="group-title">Projects <span class="card-count">{{ related.projects.length }}</span></h3>
        <ul v-if="related.projects.length" class="rows">
          <li v-for="p in related.projects" :key="p.id">
            <RouterLink class="rel-row" :to="projectLink(p.id, p.key)">
              <StatusIcon :state="p.state" :size="13" />
              <span class="rel-title">{{ p.title }}</span>
              <span class="rel-meta">{{ statusMeta(p.state).label }}</span>
              <AppIcon name="chevron-right" :size="13" class="go" />
            </RouterLink>
            <div v-if="p.cooperation && Object.values(p.cooperation).some(Boolean) && editingProject !== p.id" class="cooperation-summary">
              <span v-if="p.cooperation.engagement">{{ p.cooperation.engagement }}</span>
              <span v-if="p.cooperation.ownership">Owner: {{ p.cooperation.ownership }}</span>
              <span v-if="p.cooperation.environment_responsibility">Environment: {{ p.cooperation.environment_responsibility }}</span>
              <span v-if="p.cooperation.sla">SLA: {{ p.cooperation.sla }}</span>
              <span v-if="p.cooperation.report_contract">Report: {{ p.cooperation.report_contract }}</span>
            </div>
            <button v-if="admin && editingProject !== p.id" type="button" class="btn sm ghost" @click="editProject(p)">Edit cooperation</button>
            <form v-if="admin && editingProject === p.id" class="metadata-form" @submit.prevent="saveProject(p)">
              <label>Engagement<input v-model="cooperationDraft.engagement" maxlength="500" /></label>
              <label>Ownership<input v-model="cooperationDraft.ownership" maxlength="500" /></label>
              <label>Environment responsibility<input v-model="cooperationDraft.environment_responsibility" maxlength="500" /></label>
              <label>SLA<textarea v-model="cooperationDraft.sla" maxlength="5000" /></label>
              <label>Report contract<textarea v-model="cooperationDraft.report_contract" maxlength="5000" /></label>
              <div class="form-actions"><button type="submit" class="btn sm" :disabled="busy">Save cooperation</button><button type="button" class="btn sm ghost" @click="editingProject = null">Cancel</button></div>
            </form>
          </li>
        </ul>
        <p v-else class="none">No project names this customer yet.</p>
      </div>

      <div class="group" role="group" aria-labelledby="rel-quotes">
        <h3 id="rel-quotes" class="group-title">Quotes <span class="card-count">{{ related.quotes.length }}</span></h3>
        <ul v-if="quotes.length" class="rows">
          <li v-for="q in quotes" :key="q.id">
            <RouterLink class="rel-row" :to="`/business/quotes/${encodeURIComponent(q.id)}`" :class="{ archived: q.archived }">
              <BizIcon name="document" :size="13" class="rel-icon" />
              <span class="rel-title" :class="{ mono: q.offer_no }">{{ q.offer_no ?? 'Draft, no number yet' }}</span>
              <span class="rel-meta dot-list"><span>{{ sentenceCase(q.state) }}</span><span v-if="q.archived">archived</span></span>
              <AppIcon name="chevron-right" :size="13" class="go" />
            </RouterLink>
          </li>
        </ul>
        <p v-else class="none">No quotes yet. The customer number is assigned with the first one.</p>
      </div>

      <div v-if="business.open.hours || hours.length" class="group" role="group" aria-labelledby="rel-hours">
        <h3 id="rel-hours" class="group-title">Hours <span v-if="hours.length" class="card-count">{{ formatSpan(totalSeconds) }}</span></h3>
        <ul v-if="hours.length" class="rows">
          <li v-for="h in hours" :key="`${h.project_node_id}-${h.currency}`">
            <RouterLink class="rel-row" to="/business/hours">
              <AppIcon name="clock" :size="13" class="rel-icon" />
              <span class="rel-title">{{ projectTitle(h.project_node_id) }}</span>
              <span class="rel-meta mono">{{ formatSpan(h.duration_seconds) }}</span>
              <span class="rel-amount mono">{{ money(h.amount, h.currency) }}</span>
              <AppIcon name="chevron-right" :size="13" class="go" />
            </RouterLink>
          </li>
        </ul>
        <p v-else class="none">No hours on this customer’s projects yet.</p>
      </div>

      <div class="group" role="group" aria-labelledby="rel-docs">
        <h3 id="rel-docs" class="group-title">Documents <span class="card-count">{{ related.documents.length }}</span></h3>
        <div v-if="admin" class="document-upload">
          <label>For <select v-model="uploadTarget"><option value="">{{ 'Customer' }}</option><option v-for="p in related.projects" :key="p.id" :value="p.id">{{ p.title }}</option></select></label>
          <label>Upload file<input type="file" :disabled="busy" @change="upload" /></label>
        </div>
        <ul v-if="related.documents.length" class="rows">
          <li v-for="d in related.documents" :key="d.attachment_id">
            <a class="rel-row" :href="contentUrl(d.attachment_id, 'original')" target="_blank" rel="noopener">
              <AppIcon name="paperclip" :size="13" class="rel-icon" />
              <span class="rel-title">{{ d.title || d.name }}</span>
              <span class="rel-meta dot-list"><span v-for="part in docMeta(d)" :key="part">{{ part }}</span></span>
              <AppIcon name="external" :size="12" class="go" />
            </a>
            <span v-if="d.node_id !== customerId" class="doc-owner">{{ related.projects.find(p => p.id === d.node_id)?.title ?? 'Project' }}</span>
            <button v-if="admin && editingDocument !== d.attachment_id" type="button" class="btn sm ghost" @click="editDocument(d)">Edit details</button>
            <form v-if="admin && editingDocument === d.attachment_id" class="metadata-form" @submit.prevent="saveDocument(d)">
              <label>Label<input v-model="docDraft.title" maxlength="255" /></label>
              <label>Category<input v-model="docDraft.category" maxlength="100" /></label>
              <label>Status<select v-model="docDraft.status"><option value="draft">Draft</option><option value="active">Active</option><option value="expired">Expired</option></select></label>
              <label>Valid from<input v-model="docDraft.valid_from" type="date" /></label>
              <label>Valid until<input v-model="docDraft.valid_until" type="date" :min="docDraft.valid_from || undefined" /></label>
              <div class="form-actions"><button type="submit" class="btn sm" :disabled="busy">Save details</button><button type="button" class="btn sm ghost" @click="editingDocument = null">Cancel</button><button type="button" class="btn sm ghost" :disabled="busy" @click="removeDocument(d)">Delete file</button></div>
            </form>
          </li>
        </ul>
        <p v-else class="none">No documents yet.</p>
        <p v-if="actionError" role="alert" class="f-error">{{ actionError }}</p>
      </div>
    </div>
  </section>
</template>

<style scoped>
.sk { display: grid; gap: 12px; }
.sk .skeleton { height: 12px; }
.groups { display: grid; gap: 16px; }
.group-title { display: flex; align-items: baseline; gap: 8px; margin-bottom: 6px; font: 500 10.5px/1.4 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.group-title .card-count { letter-spacing: .02em; text-transform: none; }
.rows { display: grid; gap: 2px; margin: 0; padding: 0; list-style: none; }
.rel-row { display: flex; align-items: center; gap: 10px; min-height: 38px; padding: 6px 10px; margin: 0 -10px; border-radius: 10px; color: var(--ink); text-decoration: none; }
@media (hover: hover) { .rel-row:hover { background: var(--row-hover); } .rel-row:hover .go { color: var(--teal-ink); } }
.rel-row:focus-visible { box-shadow: var(--focus-ring); }
.rel-row.archived .rel-title { color: var(--ink-2); }
.rel-icon { flex-shrink: 0; color: var(--ink-3); }
.rel-title { flex: 1; min-width: 0; font-size: 13.5px; overflow-wrap: anywhere; }
.rel-title.mono { font-family: var(--mono); font-size: 13px; font-variant-ligatures: none; }
.rel-meta { flex: 0 1 auto; font-size: 12.5px; color: var(--ink-2); }
.rel-amount { flex-shrink: 0; min-width: 96px; text-align: right; font-size: 12.5px; color: var(--ink); }
.mono { font-family: var(--mono); font-variant-numeric: tabular-nums; font-variant-ligatures: none; }
.go { flex-shrink: 0; color: var(--ink-3); }
.none { font-size: 13px; color: var(--ink-3); }
.cooperation-summary { display: flex; flex-wrap: wrap; gap: 5px 12px; margin: 0 0 6px 10px; color: var(--ink-2); font-size: 12px; }
.metadata-form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; margin: 8px 0 14px; padding: 12px; border: 1px solid var(--line); border-radius: 10px; }
.metadata-form label, .document-upload label { display: grid; gap: 4px; color: var(--ink-2); font-size: 12px; }
.metadata-form input, .metadata-form select, .metadata-form textarea, .document-upload select { width: 100%; min-width: 0; padding: 7px 9px; border: 1px solid var(--line); border-radius: 7px; background: var(--surface); color: var(--ink); font: inherit; }
.form-actions { grid-column: 1 / -1; display: flex; flex-wrap: wrap; gap: 6px; }
.document-upload { display: flex; flex-wrap: wrap; align-items: end; gap: 10px; margin: 7px 0 11px; }
.doc-owner { display: block; margin: -2px 0 5px 23px; color: var(--ink-3); font-size: 11px; }
@media (max-width: 600px) {
  .metadata-form { grid-template-columns: 1fr; }
  .rel-row { flex-wrap: wrap; row-gap: 2px; }
  .rel-title { flex-basis: calc(100% - 60px); }
  .rel-meta { margin-left: 23px; }
  .rel-amount { min-width: 0; margin-left: auto; }
  .go { display: none; }
}
</style>
