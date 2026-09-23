<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import AppIcon from '../../components/AppIcon.vue'
import { api, createNode, getKinds, getNodes, type Kind, type WorkNode } from '../../lib/api'
import { useSession } from '../../stores/session'

interface Relation {
  id: string
  source_node_id: string
  target_node_id: string
  type: string
  created_at: string
}
interface Binding {
  principal_id: string
  contact_node_id: string
  bound_by_principal_id: string
  bound_at: string
}
interface Page<T> { items: T[]; next_cursor: string | null }

const session = useSession()
const admin = computed(() => session.identity?.principal.kind === 'person' && session.identity.principal.roles?.includes('admin') === true)
const kinds = ref<Kind[]>([])
const nodes = ref<WorkNode[]>([])
const relations = ref<Relation[]>([])
const loading = ref(true)
const pending = ref(false)
const loaded = ref(false)
const truncated = ref(false)
const loadError = ref('')
const actionError = ref('')
const notice = ref('')
const selectedOrg = ref('')
const selectedContact = ref('')
const orgTitle = ref('')
const contactTitle = ref('')
const principalId = ref('')
const linkType = ref<'customer_of' | 'contact_for'>('contact_for')
const linkSource = ref('')
const linkTarget = ref('')
const binding = ref<Binding | null>(null)

const kindBySlug = computed(() => new Map(kinds.value.map(kind => [kind.slug, kind])))
const orgKind = computed(() => kindBySlug.value.get('organisation'))
const contactKind = computed(() => kindBySlug.value.get('contact'))
const ofKind = (slug: string) => nodes.value.filter(node => node.kind_id === kindBySlug.value.get(slug)?.id)
const organisations = computed(() => ofKind('organisation'))
const contacts = computed(() => ofKind('contact'))
const projects = computed(() => ofKind('project'))
const quotes = computed(() => ofKind('quote'))
const titles = computed(() => new Map(nodes.value.map(node => [node.id, node.title])))
const linkSources = computed(() => linkType.value === 'customer_of' ? organisations.value : contacts.value)
const linkTargets = computed(() => linkType.value === 'customer_of' ? [...projects.value, ...quotes.value] : [...organisations.value, ...projects.value, ...quotes.value])
const visibleLinks = computed(() => relations.value.filter(rel => rel.type === 'customer_of' || rel.type === 'contact_for'))

watch(linkSources, items => {
  if (!items.some(node => node.id === linkSource.value)) linkSource.value = items[0]?.id ?? ''
})
watch(linkTargets, items => {
  if (!items.some(node => node.id === linkTarget.value)) linkTarget.value = items[0]?.id ?? ''
})

function text(cause: unknown) {
  return cause instanceof Error ? cause.message : 'Request failed'
}
async function read<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const response = await api(path, {
    method,
    ...(body === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
  })
  const data = await response.json().catch(() => ({}))
  if (!response.ok) {
    const message = typeof data?.message === 'string' ? data.message : typeof data?.error === 'string' ? data.error : `Request failed (${response.status})`
    throw new Error(message)
  }
  return data as T
}
async function everyPage<T>(path: string): Promise<{ items: T[]; truncated: boolean }> {
  const items: T[] = []
  let cursor = ''
  for (let page = 0; page < 5; page++) {
    const result = await read<Page<T>>(`${path}${path.includes('?') ? '&' : '?'}limit=200${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`)
    items.push(...result.items)
    if (!result.next_cursor) return { items, truncated: false }
    cursor = result.next_cursor
  }
  return { items, truncated: true }
}
function title(id: string) {
  return titles.value.get(id) ?? id
}
function linkLabel(rel: Relation) {
  const verb = rel.type === 'customer_of' ? 'customer of' : 'contact for'
  return `${title(rel.source_node_id)} ${verb} ${title(rel.target_node_id)}`
}
async function loadNodes() {
  const wanted = ['organisation', 'contact', 'project', 'quote']
  const found = kinds.value.filter(kind => wanted.includes(kind.slug))
  const pages = await Promise.all(found.map(kind => getNodes({ kind_id: kind.id, limit: 200 })))
  const items: WorkNode[] = []
  let more = false
  for (const page of pages) {
    items.push(...page.items)
    if (page.next_cursor) more = true
  }
  nodes.value = items
  truncated.value = more
  if (!organisations.value.some(node => node.id === selectedOrg.value)) selectedOrg.value = organisations.value[0]?.id ?? ''
  if (!contacts.value.some(node => node.id === selectedContact.value)) selectedContact.value = contacts.value[0]?.id ?? ''
}
async function loadRelations() {
  const ids = [...new Set([selectedOrg.value, selectedContact.value].filter(Boolean))]
  const pages = await Promise.all(ids.map(id => everyPage<Relation>(`/relations?node_id=${encodeURIComponent(id)}`)))
  const merged = new Map<string, Relation>()
  for (const page of pages) {
    if (page.truncated) truncated.value = true
    for (const rel of page.items) merged.set(rel.id, rel)
  }
  relations.value = [...merged.values()]
}
async function load() {
  loading.value = true
  loadError.value = ''
  try {
    kinds.value = (await getKinds()).items
    await loadNodes()
    await loadRelations()
    loaded.value = true
  } catch (cause) {
    loadError.value = text(cause)
  } finally {
    loading.value = false
  }
}
async function run(action: () => Promise<void>) {
  if (pending.value || !admin.value) return
  pending.value = true
  actionError.value = ''
  notice.value = ''
  try {
    await action()
  } catch (cause) {
    actionError.value = text(cause)
  } finally {
    pending.value = false
  }
}
function add(kind: Kind | undefined, titleText: string, clear: () => void) {
  const name = titleText.trim()
  if (!kind || !name) return
  return run(async () => {
    await createNode({ kind_id: kind.id, title: name })
    clear()
    notice.value = `${kind.label} created.`
    await load()
  })
}
function bindPerson() {
  const contact = selectedContact.value
  const principal = principalId.value.trim()
  if (!contact || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(principal)) {
    actionError.value = 'Enter the person principal id. An email address does not bind anyone.'
    notice.value = ''
    return
  }
  return run(async () => {
    binding.value = await read<Binding>(`/crm/contacts/${encodeURIComponent(contact)}/principals`, 'POST', { principal_id: principal })
    notice.value = `Bound principal ${binding.value.principal_id} to this contact.`
  })
}
function createLink() {
  if (!linkSource.value || !linkTarget.value || linkSource.value === linkTarget.value) return
  return run(async () => {
    await read<Relation>('/relations', 'POST', { source_node_id: linkSource.value, target_node_id: linkTarget.value, type: linkType.value })
    notice.value = 'Link recorded.'
    await loadRelations()
  })
}
onMounted(load)
watch([selectedOrg, selectedContact], () => { if (loaded.value) void loadRelations().catch(cause => { actionError.value = text(cause) }) })
</script>

<template>
  <section class="crm" aria-labelledby="crm-title">
    <header class="crm-heading">
      <div>
        <p class="eyebrow">Business</p>
        <h1 id="crm-title">CRM</h1>
        <p>Organisations and contacts are ordinary nodes. An administrator binds a person principal to a contact before that person can accept an offer. A matching email or display name never does.</p>
      </div>
      <button class="icon-button" type="button" aria-label="Refresh CRM" :disabled="loading || pending" @click="load">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><path d="M21 12a9 9 0 1 1-2.64-6.36" /><path d="M21 3v6h-6" /></svg>
      </button>
    </header>
    <p v-if="loadError" class="error" role="alert">{{ loadError }}</p>
    <p v-if="actionError" class="error" role="alert">{{ actionError }}</p>
    <p v-if="notice" role="status">{{ notice }}</p>
    <p v-if="!admin" class="crm-note">Binding a person, and creating organisations, contacts, and links, requires an administrator.</p>
    <p v-if="truncated" class="crm-note">This view shows the first page of each list. Older nodes may be outside it.</p>
    <p v-if="loaded && !loadError && !orgKind && !contactKind" class="glass-card crm-empty">Organisation and contact kinds are not configured for this tenant.</p>
    <div v-else class="crm-grid">
      <section class="glass-card crm-card" aria-labelledby="org-title">
        <h2 id="org-title"><span class="crm-mark" aria-hidden="true"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M4 20V6l8-3 8 3v14" /><path d="M9 20v-6h6v6" /></svg></span>Organisations</h2>
        <p v-if="loaded && !organisations.length" class="crm-quiet">No organisations yet.</p>
        <ul v-else class="crm-list">
          <li v-for="org in organisations" :key="org.id">
            <button type="button" :aria-pressed="selectedOrg === org.id" :disabled="pending" @click="selectedOrg = org.id">
              <span>{{ org.title }}</span><span class="crm-key">{{ org.key }}</span>
            </button>
          </li>
        </ul>
        <form v-if="admin && orgKind" class="crm-form" @submit.prevent="add(orgKind, orgTitle, () => { orgTitle = '' })">
          <label>Organisation name<input v-model="orgTitle" required maxlength="200" :disabled="pending" /></label>
          <button class="button secondary" type="submit" :disabled="pending || !orgTitle.trim()"><AppIcon name="plus" />Create organisation</button>
        </form>
      </section>
      <section class="glass-card crm-card" aria-labelledby="contact-title">
        <h2 id="contact-title"><span class="crm-mark" aria-hidden="true"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="3.2" /><path d="M5 20v-1.5a7 7 0 0 1 14 0V20" /></svg></span>Contacts</h2>
        <p v-if="loaded && !contacts.length" class="crm-quiet">No contacts yet.</p>
        <ul v-else class="crm-list">
          <li v-for="contact in contacts" :key="contact.id">
            <button type="button" :aria-pressed="selectedContact === contact.id" :disabled="pending" @click="selectedContact = contact.id; binding = null">
              <span>{{ contact.title }}</span><span class="crm-key">{{ contact.key }}</span>
            </button>
          </li>
        </ul>
        <form v-if="admin && contactKind" class="crm-form" @submit.prevent="add(contactKind, contactTitle, () => { contactTitle = '' })">
          <label>Contact name<input v-model="contactTitle" required maxlength="200" :disabled="pending" /></label>
          <button class="button secondary" type="submit" :disabled="pending || !contactTitle.trim()"><AppIcon name="plus" />Create contact</button>
        </form>
      </section>
    </div>
    <section v-if="admin && selectedContact" class="glass-card crm-card" aria-labelledby="bind-title">
      <h2 id="bind-title"><span class="crm-mark" aria-hidden="true"><AppIcon name="user" /></span>Bind a person</h2>
      <p>Enter the signed-in customer’s person principal id for {{ title(selectedContact) }}. The contact node alone grants nothing.</p>
      <form class="crm-form" @submit.prevent="bindPerson">
        <label>Person principal<input v-model="principalId" type="text" autocomplete="off" spellcheck="false" maxlength="36" :disabled="pending" /></label>
        <button class="button" type="submit" :disabled="pending || !principalId.trim()"><AppIcon name="user" />Bind person</button>
      </form>
    </section>
    <section v-if="loaded && (orgKind || contactKind)" class="glass-card crm-card" aria-labelledby="link-title">
      <h2 id="link-title"><span class="crm-mark" aria-hidden="true"><AppIcon name="list" /></span>Links</h2>
      <p>customer of runs from an organisation to a project or quote. contact for runs from a contact to an organisation, project, or quote.</p>
      <form v-if="admin" class="crm-form" @submit.prevent="createLink">
        <label>Link type
          <select v-model="linkType" :disabled="pending">
            <option value="contact_for">contact for</option>
            <option value="customer_of">customer of</option>
          </select>
        </label>
        <div class="crm-pair">
          <label>Link source
            <select v-model="linkSource" :disabled="pending || !linkSources.length">
              <option v-for="node in linkSources" :key="node.id" :value="node.id">{{ node.title }}</option>
            </select>
          </label>
          <label>Link target
            <select v-model="linkTarget" :disabled="pending || !linkTargets.length">
              <option v-for="node in linkTargets" :key="node.id" :value="node.id">{{ node.title }}</option>
            </select>
          </label>
        </div>
        <button class="button secondary" type="submit" :disabled="pending || !linkSource || !linkTarget"><AppIcon name="plus" />Create link</button>
      </form>
      <p v-if="loaded && !visibleLinks.length" class="crm-quiet">No CRM links for the selected organisation or contact.</p>
      <ul v-else class="crm-links">
        <li v-for="rel in visibleLinks" :key="rel.id">{{ linkLabel(rel) }}</li>
      </ul>
    </section>
  </section>
</template>

<style scoped>
.crm { max-width: 1100px; margin: 0 auto; padding: 28px; display: grid; gap: 22px; }
.crm-heading { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; }
.crm-heading h1 { margin-top: 8px; }
.crm-heading p:last-child { margin-top: 12px; max-width: 68ch; }
.crm-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 320px), 1fr)); gap: 20px; }
.crm-card { padding: 22px; display: grid; align-content: start; gap: 16px; min-width: 0; }
.crm-card h2 { display: flex; align-items: center; gap: 10px; margin: 0; font-size: 18px; font-weight: 560; }
.crm-mark { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 50%; background: var(--aqua-3); color: var(--teal-ink); }
.crm-list, .crm-links { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
.crm-list button { display: flex; justify-content: space-between; align-items: center; gap: 12px; width: 100%; min-height: 44px; padding: 8px 12px; border: 1px solid var(--line); border-radius: var(--radius-s); background: var(--surface); color: var(--ink); text-align: left; }
.crm-list button[aria-pressed="true"] { border-color: var(--teal); background: var(--aqua-3); }
.crm-key { font: 12px/1.4 var(--mono); color: var(--ink-2); }
.crm-form { display: grid; gap: 14px; }
.crm-form label { display: grid; gap: 6px; color: var(--ink-2); font-size: 13px; }
.crm input, .crm select { width: 100%; min-height: 44px; padding: 10px 12px; border: 1px solid var(--line-2); border-radius: var(--radius-s); background: var(--surface); color: var(--ink); font: inherit; }
.crm-pair { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.crm-note, .crm-quiet { color: var(--ink-2); }
.crm-empty { padding: 32px 22px; text-align: center; }
.crm-links li { min-height: 44px; display: flex; align-items: center; padding: 8px 0; border-bottom: 1px solid var(--line); overflow-wrap: anywhere; }
.crm :deep(svg) { display: block; }
@media (max-width: 600px) {
  .crm { padding: 20px 16px; }
  .crm-card { padding: 18px; }
  .crm-pair { grid-template-columns: 1fr; }
}
</style>
