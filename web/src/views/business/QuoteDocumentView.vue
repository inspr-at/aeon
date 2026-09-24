<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { exportURL, listVersions, type QuoteVersion } from '../../lib/business'
import { useBusiness } from '../../stores/business'
import { useProjects } from '../../stores/projects'
import { useSession } from '../../stores/session'
import { formatAmount, lineTax, ratePercent, sumAmounts } from '../../components/business/money'
import AppIcon from '../../components/AppIcon.vue'
import MarkdownBody from '../../components/MarkdownBody.vue'

// The offer as the customer reads it: one frozen version on a sheet of paper,
// ready to print or save as PDF. It renders only stored values; the server's
// Markdown and PDF exports are the same version in other formats.
const route = useRoute()
const router = useRouter()
const business = useBusiness()
const projects = useProjects()
const session = useSession()
const versions = ref<QuoteVersion[]>([])
const state = ref<'loading' | 'ready' | 'missing' | 'error'>('loading')
const error = ref('')

const key = computed(() => String(route.params.quoteKey ?? ''))
const quote = computed(() => business.quoteByKey(key.value))
const chosen = computed(() => {
  const wanted = Number(route.query.v)
  return versions.value.find(v => v.version === wanted) ?? versions.value.find(v => v.version === quote.value?.current_version) ?? versions.value[versions.value.length - 1] ?? null
})
const org = computed(() => quote.value ? business.organisation(quote.value.customer_org_node_id) : undefined)
const recipient = computed(() => chosen.value ? business.contact(chosen.value.recipient_contact_node_id) : undefined)
const project = computed(() => quote.value ? projects.byId(quote.value.project_node_id) : undefined)
const text = (value: unknown) => typeof value === 'string' ? value.trim() : ''
const money = (value: string) => { try { return formatAmount(value, chosen.value?.currency ?? 'EUR') } catch { return value } }
const taxes = computed(() => {
  const by = new Map<string, string[]>()
  for (const line of chosen.value?.lines ?? []) if (!/^0(\.0+)?$/.test(line.tax_rate)) by.set(line.tax_rate, [...(by.get(line.tax_rate) ?? []), lineTax(line.net_amount, line.tax_rate)])
  return [...by.entries()].sort(([a], [b]) => b.localeCompare(a)).map(([rate, amounts]) => ({ rate, amount: sumAmounts(amounts) }))
})
const dateFormat = new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'long', year: 'numeric' })
const day = (iso: string | undefined | null) => iso ? dateFormat.format(new Date(iso)) : ''
const status = computed(() => {
  const v = chosen.value
  if (!v) return ''
  if (v.acceptance) return `Accepted on ${day(v.acceptance.accepted_at)}`
  if (v.issue) return v.version === quote.value?.current_version ? `Issued on ${day(v.issue.issued_at)}` : `Issued on ${day(v.issue.issued_at)}, replaced by a later version`
  return v.version === quote.value?.current_version ? 'Draft, not issued' : 'Draft, replaced by a later version'
})

async function load() {
  state.value = 'loading'
  try {
    await business.loadPlugins()
    await Promise.all([business.loadQuotes(), business.loadCRM(), business.loadCostUnits(), business.loadPrincipals(), projects.load()])
    if (!quote.value) { state.value = 'missing'; return }
    versions.value = await listVersions(quote.value.quote_node_id)
    state.value = versions.value.length ? 'ready' : 'missing'
  } catch (e) { state.value = 'error'; error.value = e instanceof Error ? e.message : 'The offer could not be loaded.' }
}
watch(key, load)
onMounted(load)
watch([quote, chosen], ([q, v]) => { if (q) document.title = `${q.key}${v ? ` v${v.version}` : ''} ${q.title} · PAIMOS AEON` })
function choose(version: number) { void router.replace({ query: { ...route.query, v: String(version) } }) }
function download(format: 'pdf' | 'markdown') { if (quote.value && chosen.value) window.open(exportURL(quote.value.quote_node_id, chosen.value.version, format), '_blank', 'noopener') }
function print() { window.print() }
</script>

<template>
  <section class="doc-page" aria-label="Offer document">
    <div class="doc-bar">
      <RouterLink class="icon-btn sm flat" :to="`/business/quotes/${encodeURIComponent(key)}`" :aria-label="`Back to ${key.toUpperCase()}`" :data-tip="`Back to ${key.toUpperCase()}`"><AppIcon name="chevron-left" :size="16" /></RouterLink>
      <span class="key-badge">{{ key.toUpperCase() }}</span>
      <div v-if="versions.length > 1" class="seg versions" role="radiogroup" aria-label="Version">
        <button v-for="v in versions" :key="v.version" type="button" role="radio" :aria-checked="chosen?.version === v.version" @click="choose(v.version)">v{{ v.version }}</button>
      </div>
      <span class="spacer" />
      <button type="button" class="btn sm" :disabled="!chosen" @click="download('markdown')"><AppIcon name="document" :size="13" />Markdown</button>
      <button type="button" class="btn sm" :disabled="!chosen" @click="download('pdf')"><AppIcon name="download" :size="13" />PDF</button>
      <button type="button" class="btn sm primary" :disabled="!chosen" @click="print"><AppIcon name="print" :size="13" />Print</button>
    </div>

    <div v-if="state === 'loading'" class="sheet skeleton-body" role="status" aria-label="Loading the offer">
      <span class="skeleton w40" /><span class="skeleton w70" /><span class="skeleton w100" /><span class="skeleton w100" /><span class="skeleton w60" />
    </div>
    <div v-else-if="state !== 'ready' || !chosen || !quote" class="doc-state glass-card" role="alert">
      <span class="state-icon"><AppIcon :name="state === 'error' ? 'alert' : 'document'" :size="18" /></span>
      <h1>{{ state === 'error' ? 'The offer could not be loaded' : quote ? `${quote.key} has no version yet` : `No quote called ${key.toUpperCase()}` }}</h1>
      <p>{{ state === 'error' ? error : quote ? 'The document exists once the first version is saved.' : 'It may have been deleted.' }}</p>
      <RouterLink class="btn" :to="quote ? `/business/quotes/${encodeURIComponent(quote.key)}` : '/business/quotes'">{{ quote ? `Back to ${quote.key}` : 'All quotes' }}</RouterLink>
    </div>

    <article v-else class="sheet" :aria-label="`${quote.key}, version ${chosen.version}`">
      <header class="sheet-head">
        <div>
          <p class="issuer">{{ session.identity?.tenant.name }}</p>
          <p class="doc-kind">Offer</p>
        </div>
        <dl class="ref">
          <div><dt>Offer</dt><dd class="mono">{{ quote.key }} · v{{ chosen.version }}</dd></div>
          <div><dt>Date</dt><dd>{{ day(chosen.issue?.issued_at ?? chosen.created_at) }}</dd></div>
          <div><dt>Status</dt><dd>{{ status }}</dd></div>
        </dl>
      </header>

      <h1 class="doc-title">{{ chosen.title }}</h1>

      <div class="parties">
        <div>
          <p class="label">For</p>
          <p class="strong">{{ text(org?.fields.legal_name) || org?.title || 'Customer' }}</p>
          <p v-if="text(org?.fields.legal_name) && org?.title !== text(org?.fields.legal_name)">{{ org?.title }}</p>
          <p v-if="recipient">Attn. {{ recipient.title }}<template v-if="text(recipient.fields.role)">, {{ text(recipient.fields.role) }}</template></p>
          <p v-if="text(recipient?.fields.email)" class="muted">{{ text(recipient?.fields.email) }}</p>
        </div>
        <div>
          <p class="label">Project</p>
          <p class="strong">{{ project?.title ?? '—' }}</p>
          <p class="muted">Prices in {{ chosen.currency }}, per the rates in force on {{ day(chosen.created_at) }}.</p>
        </div>
      </div>

      <table class="doc-lines">
        <thead>
          <tr><th class="pos">#</th><th>Description</th><th class="num">Qty</th><th>Unit</th><th class="num">Rate</th><th class="num">Tax</th><th class="num">Amount</th></tr>
        </thead>
        <tbody>
          <tr v-for="line in chosen.lines" :key="line.position">
            <td class="pos mono">{{ line.position + 1 }}</td>
            <td class="desc">{{ line.description }}</td>
            <td class="num mono">{{ line.quantity.replace(/\.?0+$/, '') }}</td>
            <td>{{ line.unit }}</td>
            <td class="num mono">{{ money(line.rate_amount) }}</td>
            <td class="num mono">{{ ratePercent(line.tax_rate) }} %</td>
            <td class="num mono">{{ money(line.net_amount) }}</td>
          </tr>
        </tbody>
      </table>

      <dl class="doc-totals">
        <div><dt>Net</dt><dd class="mono">{{ money(chosen.subtotal) }} {{ chosen.currency }}</dd></div>
        <div v-for="tax in taxes" :key="tax.rate"><dt>Tax {{ ratePercent(tax.rate) }} %</dt><dd class="mono">{{ money(tax.amount) }} {{ chosen.currency }}</dd></div>
        <div class="grand"><dt>Total</dt><dd class="mono">{{ money(chosen.total) }} {{ chosen.currency }}</dd></div>
      </dl>

      <section v-if="chosen.terms_markdown.trim()" class="doc-terms" aria-label="Terms">
        <p class="label">Terms</p>
        <MarkdownBody :body="chosen.terms_markdown" />
      </section>

      <section class="doc-accept" aria-label="Acceptance">
        <template v-if="chosen.acceptance">
          <p class="label">Accepted</p>
          <p>Accepted by {{ business.nameOf(chosen.acceptance.customer_principal_id) }} on {{ day(chosen.acceptance.accepted_at) }}. The acceptance is recorded against this exact version.</p>
        </template>
        <template v-else-if="chosen.issue">
          <p class="label">Acceptance</p>
          <p>{{ recipient?.title ?? 'The recipient' }} accepts this offer by signing in to PAIMOS AEON and accepting version {{ chosen.version }} of {{ quote.key }}.</p>
        </template>
        <template v-else>
          <p class="label">Draft</p>
          <p>This version has not been issued. It is shown for review only.</p>
        </template>
      </section>

      <footer class="sheet-foot">
        <span>{{ quote.key }} · version {{ chosen.version }}</span>
        <span class="digest">SHA-256 {{ chosen.content_sha256 }}</span>
      </footer>
    </article>
  </section>
</template>

<style scoped>
.doc-page { width: 100%; max-width: 1000px; margin: 0 auto; padding: 14px 28px 40px; }
.doc-bar { position: sticky; top: 0; z-index: 4; display: flex; align-items: center; gap: 8px; padding: 10px 0 14px; }
.versions button { height: 26px; padding: 0 10px; font-family: var(--mono); font-size: 12px; }
.spacer { flex: 1; }
/* Paper stays paper in both themes: the sheet is what gets printed. */
.sheet {
  --paper: #fffefa; --paper-ink: #1f2f30; --paper-ink-2: #4a5c5e; --paper-ink-3: #5b6d6f; --paper-line: rgba(31, 47, 48, .14); --paper-accent: #0b5c59;
  display: grid; gap: 22px; padding: 56px 64px 40px; border-radius: 6px; background: var(--paper); color: var(--paper-ink);
  box-shadow: 0 0 0 1px rgba(32, 60, 61, .08), 0 30px 60px -30px rgba(16, 35, 39, .45), 0 6px 14px -8px rgba(16, 35, 39, .2);
  font-size: 13px; line-height: 1.55;
}
.sheet.skeleton-body { min-height: 700px; align-content: start; }
.sheet .skeleton { background: rgba(32, 60, 61, .08); }
.w40 { width: 40%; } .w70 { width: 70%; height: 20px !important; } .w100 { width: 100%; } .w60 { width: 60%; }
.sheet p, .sheet :deep(p) { color: var(--paper-ink); }
.sheet-head { display: flex; justify-content: space-between; gap: 24px; padding-bottom: 18px; border-bottom: 2px solid var(--paper-accent); }
.issuer { font: 600 13px/1.2 var(--mono); letter-spacing: .24em; text-transform: uppercase; color: var(--paper-accent) !important; font-variant-ligatures: none; }
.doc-kind { margin-top: 6px; font: 300 30px/1.1 var(--serif); letter-spacing: -.02em; }
.ref { display: grid; gap: 4px; margin: 0; }
.ref > div { display: grid; grid-template-columns: 64px auto; gap: 12px; }
.ref dt, .label { font: 500 10px/1.6 var(--mono); letter-spacing: .16em; text-transform: uppercase; color: var(--paper-ink-3) !important; font-variant-ligatures: none; }
.ref dd { margin: 0; text-align: right; }
.mono { font-family: var(--mono); font-variant-numeric: tabular-nums; font-variant-ligatures: none; }
.doc-title { font: 500 22px/1.25 var(--serif); letter-spacing: -.01em; color: var(--paper-ink); }
.parties { display: grid; grid-template-columns: 1fr 1fr; gap: 32px; }
.parties p { margin: 0; }
.strong { font-weight: 650; }
.muted { color: var(--paper-ink-2) !important; }
.doc-lines { width: 100%; border-collapse: collapse; }
.doc-lines th { padding: 8px 6px; border-bottom: 1px solid var(--paper-ink); text-align: left; font: 500 9.5px/1 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--paper-ink-3); font-variant-ligatures: none; }
.doc-lines td { padding: 9px 6px; border-bottom: 1px solid var(--paper-line); vertical-align: top; font-size: 12.5px; }
.doc-lines .num { text-align: right; white-space: nowrap; }
.doc-lines .pos { width: 28px; color: var(--paper-ink-3); }
.doc-lines .desc { width: 44%; }
.doc-totals { display: grid; justify-content: end; gap: 3px; margin: -6px 0 0; }
.doc-totals > div { display: grid; grid-template-columns: 120px 170px; gap: 12px; }
.doc-totals dt { text-align: right; color: var(--paper-ink-2); }
.doc-totals dd { margin: 0; text-align: right; }
.doc-totals .grand { margin-top: 6px; padding-top: 8px; border-top: 2px solid var(--paper-ink); font-weight: 700; font-size: 14px; }
.doc-totals .grand dt { color: var(--paper-ink); }
.doc-terms :deep(.markdown-body) { font-size: 12.5px; color: var(--paper-ink); }
.doc-terms :deep(.markdown-body p), .doc-terms :deep(.markdown-body li) { color: var(--paper-ink); }
.doc-accept { padding: 12px 14px; border-radius: 6px; background: rgba(14, 111, 108, .06); box-shadow: inset 0 0 0 1px rgba(14, 111, 108, .2); }
.doc-accept p:last-child { font-size: 12.5px; }
.sheet-foot { display: flex; justify-content: space-between; gap: 16px; padding-top: 14px; border-top: 1px solid var(--paper-line); font: 10px/1.5 var(--mono); color: var(--paper-ink-3); font-variant-ligatures: none; }
.digest { overflow-wrap: anywhere; text-align: right; }
.doc-state { display: grid; justify-items: center; gap: 8px; padding: 48px 24px; text-align: center; }
.doc-state h1 { font-size: 20px; }
.doc-state p { font-size: 13.5px; }
.doc-state .btn { margin-top: 8px; }
.state-icon { display: grid; place-items: center; width: 44px; height: 44px; margin-bottom: 4px; border-radius: 50%; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
@media (max-width: 720px) {
  .doc-page { padding: 8px 10px 24px; }
  .doc-bar { flex-wrap: wrap; }
  .doc-bar .btn { height: 36px; }
  .sheet { padding: 24px 18px; gap: 18px; }
  .sheet-head { flex-direction: column; }
  .ref dd { text-align: left; }
  .parties { grid-template-columns: 1fr; gap: 14px; }
  /* Phones keep quantity, unit and amount; rate and tax stay in the totals and the PDF. */
  .doc-lines th:nth-child(5), .doc-lines td:nth-child(5), .doc-lines th:nth-child(6), .doc-lines td:nth-child(6) { display: none; }
  .doc-lines .desc { width: auto; }
  .doc-totals > div { grid-template-columns: 100px 150px; }
  .sheet-foot { flex-direction: column; }
}
@media print {
  :global(.app-header), :global(.app-footer), :global(.skip-link), :global(.toast-host), :global(.tip) { display: none !important; }
  :global(html), :global(body), :global(#app), :global(.app-shell), :global(main), :global(.page-flow) { display: block !important; height: auto !important; overflow: visible !important; background: #fff !important; }
  :global(body::before) { display: none !important; }
  .doc-page { max-width: none; padding: 0; }
  .doc-bar { display: none; }
  .sheet { padding: 0; box-shadow: none; border-radius: 0; background: #fff; }
  .doc-lines tr { break-inside: avoid; }
}
</style>
