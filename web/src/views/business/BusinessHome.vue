<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { listEntries, listPeriods, type TimeEntry, type TimePeriod } from '../../lib/business'
import { absoluteTime, plural, relativeTime } from '../../lib/work'
import { addDays, dayKey, isoWeek, startOfWeek, weekDays, weekLabel, WEEKDAYS } from '../../lib/week'
import { formatSpan } from '../../components/business/duration'
import { sumAmounts } from '../../components/business/money'
import { useBusiness } from '../../stores/business'
import { useSession } from '../../stores/session'
import AppIcon from '../../components/AppIcon.vue'
import BusinessPage from '../../components/business/BusinessPage.vue'
import MoneyText from '../../components/business/MoneyText.vue'
import QuoteStatus from '../../components/business/QuoteStatus.vue'
import SetupCard from '../../components/business/SetupCard.vue'

// A calm desk for the business side: what is on offer, the week's hours and
// what waits for approval, and the organisations touched last.
const business = useBusiness()
const session = useSession()
const router = useRouter()
const now = ref(Date.now())
const managing = ref(false)
const me = computed(() => session.identity?.principal.id ?? '')

// ---------- Quotes ----------
const openQuotes = computed(() => business.quotes.filter(q => q.state === 'draft' || q.state === 'issued').sort((a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at)))
const openValue = computed(() => {
  const byCurrency = new Map<string, string[]>()
  for (const q of openQuotes.value) if (q.current) byCurrency.set(q.current.currency, [...(byCurrency.get(q.current.currency) ?? []), q.current.total])
  return [...byCurrency.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([currency, totals]) => ({ currency, amount: sumAmounts(totals) }))
})
const orgName = (id: string) => business.organisation(id)?.title ?? '—'

// ---------- Hours this week ----------
const week = startOfWeek(new Date())
const days = weekDays(week)
const today = dayKey(new Date())
const weekEntries = ref<TimeEntry[]>([])
const allPeriods = ref<TimePeriod[]>([])
const hoursLoaded = ref(false)
const hoursError = ref('')
async function loadHours() {
  hoursError.value = ''
  try {
    const start = week.getTime(), end = addDays(week, 7).getTime()
    const mine = await listPeriods(me.value)
    const overlapping = mine.filter(p => Date.parse(p.starts_at) < end && Date.parse(p.ends_at) > start)
    const entries = (await Promise.all(overlapping.map(p => listEntries({ period_id: p.id })))).flat()
    weekEntries.value = entries.filter(e => Date.parse(e.started_at) >= start && Date.parse(e.started_at) < end)
    if (business.admin) allPeriods.value = await listPeriods()
    hoursLoaded.value = true
  } catch (e) { hoursError.value = e instanceof Error ? e.message : 'Hours could not be loaded.' }
}
const perDay = computed(() => days.map(day => weekEntries.value.filter(e => dayKey(new Date(e.started_at)) === dayKey(day)).reduce((sum, e) => sum + e.duration_seconds, 0)))
const weekTotal = computed(() => perDay.value.reduce((a, b) => a + b, 0))
const maxDay = computed(() => Math.max(8 * 3600, ...perDay.value))
const awaiting = computed(() => allPeriods.value.filter(p => p.state === 'open' && Date.parse(p.ends_at) <= now.value))

// ---------- Organisations ----------
const recentOrgs = computed(() => [...business.organisations].sort((a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at)).slice(0, 6))
function website(value: unknown) { return typeof value === 'string' ? value.replace(/^https?:\/\//, '').replace(/\/$/, '') : '' }

const summary = computed(() => {
  const parts: string[] = []
  if (business.open.quotes && business.quotesLoaded) parts.push(openQuotes.value.length ? `${plural(openQuotes.value.length, 'open quote')}` : 'No open quotes')
  if (business.open.hours && hoursLoaded.value) parts.push(`${formatSpan(weekTotal.value)} logged this week`)
  if (business.admin && awaiting.value.length) parts.push(`${plural(awaiting.value.length, 'period')} to approve`)
  return parts.join(' · ')
})

async function load() {
  await business.loadPlugins()
  if (business.open.crm) void business.loadCRM()
  if (business.open.quotes) void business.loadQuotes(true)
  if (business.open.hours) void loadHours()
  if (business.anyOpen && business.staff) void business.loadPrincipals()
}
watch(() => business.open, (open, before) => { if (before && JSON.stringify(open) !== JSON.stringify(before)) void load() })
let clock: ReturnType<typeof setInterval> | undefined
onMounted(() => { void load(); clock = setInterval(() => { now.value = Date.now() }, 60_000) })
onBeforeUnmount(() => clearInterval(clock))
</script>

<template>
  <BusinessPage title="Business">
    <template #summary>
      <span v-if="summary">{{ summary }}</span>
      <span v-else-if="business.plugins && !business.anyOpen">{{ business.admin ? 'Not set up yet' : 'Not enabled for this workspace' }}</span>
      <span v-else class="skeleton summary-skeleton" />
    </template>
    <template v-if="business.admin && business.anyOpen" #actions>
      <button type="button" class="btn sm" :aria-pressed="managing" @click="managing = !managing"><AppIcon name="sliders" :size="14" />Manage parts</button>
    </template>

    <div v-if="!business.anyOpen" class="intro">
      <SetupCard v-if="business.admin" variant="intro" />
      <div v-else class="closed glass-card">
        <span class="state-icon"><AppIcon name="briefcase" :size="18" /></span>
        <h2>Business is not set up for this workspace</h2>
        <p>Quotes, hours, organisations and rates appear here once a workspace admin enables them.</p>
      </div>
    </div>

    <div v-else class="layout">
      <div class="main-col">
        <SetupCard v-if="managing || (business.admin && !business.allOpen)" :variant="managing ? 'manage' : 'intro'" @done="managing = false" />

        <section v-if="business.open.quotes" class="card glass-card" aria-labelledby="open-quotes-title">
          <header class="card-head">
            <h2 id="open-quotes-title">Open quotes</h2>
            <span v-if="business.quotesLoaded" class="count mono">{{ openQuotes.length }}</span>
            <span class="spacer" />
            <span v-if="openValue.length" class="value"><span class="value-label">Open value</span><MoneyText v-for="row in openValue" :key="row.currency" :amount="row.amount" :currency="row.currency" strong /></span>
          </header>
          <div v-if="!business.quotesLoaded && !business.quotesError" class="rows-skeleton" aria-hidden="true"><span v-for="i in 4" :key="i" class="skeleton" /></div>
          <p v-else-if="business.quotesError" class="inline-error" role="alert"><AppIcon name="alert" :size="14" />{{ business.quotesError }}</p>
          <div v-else-if="!openQuotes.length" class="empty">
            <p>No quote is waiting as a draft or for the customer’s answer.</p>
            <RouterLink v-if="business.staff" class="btn sm" to="/business/quotes?new=1"><AppIcon name="plus" :size="13" />New quote</RouterLink>
          </div>
          <ul v-else class="quote-rows" aria-label="Open quotes">
            <li v-for="q in openQuotes.slice(0, 6)" :key="q.quote_node_id">
              <RouterLink class="quote-row" :to="`/business/quotes/${encodeURIComponent(q.key)}`">
                <span class="key-badge">{{ q.key }}</span>
                <span class="q-text"><span class="q-title">{{ q.title }}</span><span class="q-org">{{ orgName(q.customer_org_node_id) }}</span></span>
                <QuoteStatus :state="q.state" />
                <span class="q-total"><MoneyText v-if="q.current" :amount="q.current.total" :currency="q.current.currency" /><span v-else class="faint">No version</span></span>
                <time class="q-when" :datetime="q.updated_at" :data-tip="absoluteTime(q.updated_at)">{{ relativeTime(q.updated_at, { now }) }}</time>
              </RouterLink>
            </li>
          </ul>
          <footer v-if="openQuotes.length > 6 || business.quotes.length > openQuotes.length" class="card-foot">
            <RouterLink class="more-link" to="/business/quotes">All quotes<AppIcon name="arrow" :size="13" /></RouterLink>
          </footer>
        </section>
      </div>

      <aside class="side-col" aria-label="Hours and organisations">
        <section v-if="business.open.hours" class="card glass-card" aria-labelledby="week-title">
          <header class="card-head">
            <h2 id="week-title">This week</h2>
            <span class="sub">Week {{ isoWeek(week) }} · {{ weekLabel(week) }}</span>
          </header>
          <div class="week-body">
            <p class="week-total"><b class="mono">{{ hoursLoaded ? formatSpan(weekTotal) : '—' }}</b><span>logged by you</span></p>
            <p v-if="hoursError" class="inline-error" role="alert"><AppIcon name="alert" :size="14" />{{ hoursError }}</p>
            <div class="bars" role="list" aria-label="Hours per day">
              <div v-for="(day, i) in days" :key="i" class="bar-col" :class="{ today: dayKey(day) === today }" role="listitem" :aria-label="`${WEEKDAYS[i]}: ${formatSpan(perDay[i])}`" :data-tip="`${WEEKDAYS[i]} ${day.getDate()}: ${perDay[i] ? formatSpan(perDay[i]) : 'nothing logged'}`">
                <span class="bar-track"><i :style="{ height: `${Math.round(perDay[i] / maxDay * 100)}%` }" /></span>
                <span class="bar-day">{{ WEEKDAYS[i].slice(0, 2) }}</span>
              </div>
            </div>
          </div>
          <footer class="card-foot split">
            <RouterLink v-if="business.admin && awaiting.length" class="more-link gold" to="/business/hours?view=approvals"><AppIcon name="seal" :size="13" />{{ plural(awaiting.length, 'period') }} to approve</RouterLink>
            <span v-else class="faint foot-note">{{ business.admin ? 'No period waits for approval' : '' }}</span>
            <button type="button" class="btn sm" @click="router.push('/business/hours?log=1')"><AppIcon name="plus" :size="13" />Log time</button>
          </footer>
        </section>

        <section v-if="business.open.crm" class="card glass-card" aria-labelledby="orgs-title">
          <header class="card-head">
            <h2 id="orgs-title">Recent organisations</h2>
            <span class="spacer" />
            <RouterLink class="more-link" to="/business/organisations">All<AppIcon name="arrow" :size="13" /></RouterLink>
          </header>
          <div v-if="!business.crmLoaded" class="rows-skeleton" aria-hidden="true"><span v-for="i in 3" :key="i" class="skeleton" /></div>
          <div v-else-if="!recentOrgs.length" class="empty"><p>No organisation yet. Customers you add appear here.</p></div>
          <ul v-else class="org-rows" aria-label="Recent organisations">
            <li v-for="org in recentOrgs" :key="org.id">
              <RouterLink class="org-row" :to="`/business/organisations/${encodeURIComponent(org.key)}`">
                <span class="org-mark" aria-hidden="true"><AppIcon name="building" :size="13" /></span>
                <span class="org-text"><span class="org-name">{{ org.title }}</span><span v-if="website(org.fields.website)" class="org-site">{{ website(org.fields.website) }}</span></span>
                <time class="q-when" :datetime="org.updated_at">{{ relativeTime(org.updated_at, { now }) }}</time>
              </RouterLink>
            </li>
          </ul>
        </section>
      </aside>
    </div>
  </BusinessPage>
</template>

<style scoped>
.summary-skeleton { display: inline-block; width: 240px; }
.intro { max-width: 760px; }
.closed { display: grid; justify-items: center; gap: 8px; padding: 44px 28px; text-align: center; }
.closed h2 { font-size: 17px; }
.closed p { font-size: 13.5px; max-width: 46ch; }
.state-icon { display: grid; place-items: center; width: 44px; height: 44px; margin-bottom: 4px; border-radius: 50%; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.layout { display: grid; grid-template-columns: minmax(0, 1fr) 340px; gap: 20px; align-items: start; }
.main-col, .side-col { display: grid; grid-template-columns: minmax(0, 1fr); gap: 16px; min-width: 0; }
.side-col { position: sticky; top: 16px; }
.card { overflow: clip; }
.card-head { display: flex; align-items: center; gap: 10px; min-height: 50px; padding: 12px 18px 10px; }
.card-head h2 { font-size: 15px; font-weight: 650; }
.count { display: inline-grid; place-items: center; min-width: 22px; height: 20px; padding: 0 6px; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); font-size: 11px; color: var(--ink-2); }
.sub { font-size: 12.5px; color: var(--ink-3); }
.spacer { flex: 1; }
.value { display: inline-flex; align-items: baseline; gap: 12px; font-size: 14px; }
.value-label { font: 500 10px/1 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.quote-rows, .org-rows { margin: 0; padding: 0 6px 8px; list-style: none; border-top: 1px solid var(--line); }
.quote-row { display: grid; grid-template-columns: max-content minmax(0, 1fr) 110px minmax(120px, max-content) 64px; align-items: center; gap: 14px; min-height: 52px; margin-top: 4px; padding: 6px 12px; border-radius: 10px; color: var(--ink); text-decoration: none; }
@media (hover: hover) { .quote-row:hover, .org-row:hover { background: var(--row-hover); } }
.quote-row:focus-visible, .org-row:focus-visible { background: var(--row-selected); box-shadow: inset 3px 0 0 var(--row-accent), 0 0 0 1px var(--glass-rim); }
.q-text, .org-text { display: grid; min-width: 0; }
.q-title, .org-name { font-size: 14px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.q-org, .org-site { font-size: 12.5px; color: var(--ink-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.q-total { text-align: right; font-size: 13.5px; }
.q-when { font-size: 12px; color: var(--ink-3); text-align: right; white-space: nowrap; }
.faint { color: var(--ink-3); font-size: 12.5px; }
.card-foot { display: flex; align-items: center; justify-content: flex-end; gap: 10px; padding: 10px 18px 12px; border-top: 1px solid var(--line); }
.card-foot.split { justify-content: space-between; }
.foot-note { font-size: 12px; }
.more-link { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px; margin-right: -10px; border-radius: 999px; color: var(--teal-ink); font-size: 12.5px; font-weight: 600; text-decoration: none; }
.more-link:hover { background: var(--row-hover); }
.more-link:focus-visible { box-shadow: var(--focus-ring); }
.more-link.gold { margin: 0 0 0 -10px; color: var(--gold-ink); }
.empty { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 18px; border-top: 1px solid var(--line); }
.empty p { font-size: 13.5px; }
.rows-skeleton { display: grid; gap: 14px; padding: 18px; border-top: 1px solid var(--line); }
.rows-skeleton .skeleton { height: 12px; }
.rows-skeleton .skeleton:nth-child(2n) { width: 70%; }
.inline-error { display: flex; align-items: center; gap: 8px; margin: 0 18px 12px; padding: 8px 12px; border-radius: 10px; background: var(--danger-bg); color: var(--danger); font-size: 13px; }
.week-body { padding: 4px 18px 14px; }
.week-total { display: flex; align-items: baseline; gap: 10px; margin-bottom: 12px; font-size: 12.5px; color: var(--ink-3); }
.week-total b { font-size: 26px; font-weight: 500; color: var(--ink); letter-spacing: -.01em; }
.bars { display: grid; grid-template-columns: repeat(7, 1fr); gap: 8px; height: 96px; }
.bar-col { display: grid; grid-template-rows: minmax(0, 1fr) auto; justify-items: center; gap: 6px; }
.bar-track { position: relative; width: 100%; max-width: 26px; height: 100%; border-radius: 7px; background: var(--track); overflow: hidden; }
.bar-track i { position: absolute; inset: auto 0 0; border-radius: 7px; background: linear-gradient(0deg, #0e6f6c, #a4e5df); box-shadow: 0 0 8px rgba(164, 229, 223, .6); }
.bar-day { font: 500 10.5px/1 var(--mono); letter-spacing: .06em; color: var(--ink-3); text-transform: uppercase; font-variant-ligatures: none; }
.today .bar-day { color: var(--teal-ink); font-weight: 700; }
.today .bar-track { box-shadow: inset 0 0 0 1px var(--chip-teal-line); }
.org-row { display: grid; grid-template-columns: 28px minmax(0, 1fr) auto; align-items: center; gap: 12px; min-height: 48px; margin-top: 4px; padding: 6px 12px; border-radius: 10px; color: var(--ink); text-decoration: none; }
.org-mark { display: grid; place-items: center; width: 28px; height: 28px; border-radius: 8px; background: var(--code-bg); color: var(--ink-2); }
@media (max-width: 1180px) { .quote-row { grid-template-columns: max-content minmax(0, 1fr) 100px max-content; } .quote-row .q-when { display: none; } }
@media (max-width: 1040px) { .layout { grid-template-columns: minmax(0, 1fr); } .side-col { position: static; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); } }
@media (max-width: 720px) {
  .card-head { flex-wrap: wrap; padding: 12px 14px 8px; }
  .value { width: 100%; }
  .quote-row { grid-template-columns: minmax(0, 1fr) auto; grid-template-areas: "key total" "text text" "status when"; row-gap: 4px; padding: 10px; }
  .quote-row .key-badge { grid-area: key; justify-self: start; }
  .q-text { grid-area: text; }
  .quote-row .quote-status { grid-area: status; }
  .q-total { grid-area: total; }
  .quote-row .q-when { display: block; grid-area: when; }
  .q-title { white-space: normal; }
  .side-col { grid-template-columns: minmax(0, 1fr); }
  .empty { flex-direction: column; align-items: flex-start; }
  .card-foot .btn { height: 40px; }
}
</style>
