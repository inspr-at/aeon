<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed } from 'vue'
import type { Related } from '../../lib/crm'
import '../../styles/crm.css'
import { contentUrl } from '../../lib/attachments'
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
const props = defineProps<{ related: Related | null; error?: string; canQuote?: boolean }>()
const emit = defineEmits<{ retry: []; newQuote: [] }>()
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
          </li>
        </ul>
        <p v-else class="none">No project names this customer yet.</p>
      </div>

      <div class="group" role="group" aria-labelledby="rel-quotes">
        <div class="group-head">
          <h3 id="rel-quotes" class="group-title">Quotes <span class="card-count">{{ related.quotes.length }}</span></h3>
          <button v-if="canQuote" type="button" class="add-quote" @click="emit('newQuote')"><AppIcon name="plus" :size="12" />New quote</button>
        </div>
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

      <div v-if="related.documents.length" class="group" role="group" aria-labelledby="rel-docs">
        <h3 id="rel-docs" class="group-title">Documents <span class="card-count">{{ related.documents.length }}</span></h3>
        <ul class="rows">
          <li v-for="d in related.documents" :key="d.attachment_id">
            <a class="rel-row" :href="contentUrl(d.attachment_id, 'original')" target="_blank" rel="noopener">
              <AppIcon name="paperclip" :size="13" class="rel-icon" />
              <span class="rel-title">{{ d.title || d.name }}</span>
              <span class="rel-meta dot-list"><span v-for="part in docMeta(d)" :key="part">{{ part }}</span></span>
              <AppIcon name="external" :size="12" class="go" />
            </a>
          </li>
        </ul>
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
.group-head { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; }
.add-quote { display: inline-flex; align-items: center; gap: 5px; height: 24px; margin-bottom: 6px; padding: 0 8px; border: 0; border-radius: 999px; background: transparent; color: var(--teal-ink); font-size: 12px; font-weight: 600; }
.add-quote:hover { background: var(--row-selected); }
.add-quote:focus-visible { box-shadow: var(--focus-ring); }
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
@media (max-width: 600px) {
  .rel-row { flex-wrap: wrap; row-gap: 2px; }
  .rel-title { flex-basis: calc(100% - 60px); }
  .rel-meta { margin-left: 23px; }
  .rel-amount { min-width: 0; margin-left: auto; }
  .go { display: none; }
}
</style>
