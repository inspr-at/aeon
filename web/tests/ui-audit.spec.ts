// SPDX-License-Identifier: AGPL-3.0-only
// Offline UI audit. Run `npm run audit:ui` in web. It writes a deduplicated
// ../../qa2-findings.json and screenshots in ../../design-ref/shots/qa2a/. A finding
// is evidence for the UI builder; this spec deliberately does not fix UI code.
import { test, expect, type Page } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { createHash } from 'node:crypto'
import { mkdirSync, writeFileSync } from 'node:fs'
import { resolve, join } from 'node:path'
import { fixtures, me, mockWork } from './work-fixtures'
import { agentData, mockAgents } from './agents-fixtures'
import { businessData, mockBusiness } from './business-fixtures'
import { crmData, HOFER, mockCRM } from './crm-fixtures'
import { Q, mockPublicQuote, mockQuotes, quoteWorld } from './quote-list-fixtures'
import { mockSettings, settingsData, makePng } from './settings-fixtures'
import { mockProfiles, profileWorld, PROFILE } from './profile-fixtures'
import { mockReleases, releaseHistory } from './releases-fixtures'
import { journeyWorld, mockJourney } from './journey-fixtures'
import { mockQuoteEditor, QUOTE_ID } from './quote-inspector-fixtures'

type Kind = 'horizontal-overflow' | 'interactive-overlap' | 'clipped-text' | 'text-icon' | 'axe' | 'console' | 'unhandled-rejection' | 'layout-shift' | 'small-touch-target' | 'invisible-focus' | 'scenario-error'
type Raw = { kind: Kind; severity: 'critical' | 'serious' | 'moderate'; selector: string; detail: string }
type Finding = Raw & { id: string; route: string; state: string; viewport: string; theme: string; screenshot: string }
type Setup = 'default' | 'editor' | 'journey' | 'public' | 'signed-out'
type Scenario = { state: string; route: string; setup?: Setup; act?: (page: Page) => Promise<void> }

const output = resolve(process.cwd(), '../../qa2-findings.json')
const shotDir = resolve(process.cwd(), '../../design-ref/shots/qa2a')
const findings = new Map<string, Finding>()
const widths = process.env.AUDIT_WIDTHS ? process.env.AUDIT_WIDTHS.split(',').map(Number) : [390, 1024, 1280, 1440, 1920]
const themes = process.env.AUDIT_THEMES ? process.env.AUDIT_THEMES.split(',') as ('light' | 'dark')[] : ['light', 'dark'] as const
const quote = '/business/quotes'
const editor = `${quote}/${QUOTE_ID}`
const history = releaseHistory()
const signedOut: Scenario['act'] = async page => { await expect(page.getByLabel('Email address')).toBeVisible() }
const visible = (selector: string): Scenario['act'] => async page => { await expect(page.locator(selector).first()).toBeVisible() }
const listContent = (text: string): Scenario['act'] => async page => { await expect(page.getByText(text, { exact: false }).first()).toBeVisible() }
const inspectorTab = (name: string): Scenario['act'] => async page => {
  await visible('.quote-document')(page)
  const panel = page.getByRole('complementary', { name: 'Format' })
  if (await panel.count() === 0) await page.getByRole('button', { name: 'Format panel' }).click()
  await panel.getByRole('tab', { name }).click()
}

// Concrete URLs for every router record, including aliases, redirects and the
// catch-all. Extra states exercise controls that have no dedicated route.
const scenarios: Scenario[] = [
  { state: 'projects', route: '/', act: visible('main') },
  { state: 'project', route: '/p/PHAROS', act: visible('tr.ticket-row:not(.ghost)') },
  { state: 'ticket panel', route: '/p/PHAROS/PHAROS-11', act: visible('.ticket-ws') },
  { state: 'workspace redirect', route: '/workspace', act: visible('main') },
  { state: 'legacy project redirect', route: '/projects/p-pharos/journey/plan', setup: 'journey', act: visible('main') },
  { state: 'project journey', route: '/p/PHAROS?view=journey', setup: 'journey', act: visible('main') },
  { state: 'business overview', route: '/business', act: visible('main') },
  { state: 'customers', route: '/business/customers', act: listContent('Bäckerei Hofer') },
  { state: 'customer page', route: `/business/customers/${HOFER}`, act: visible('main') },
  { state: 'quotes', route: quote, act: listContent('Onlineshop Erweiterung Weihnachten') },
  { state: 'full quote workspace', route: editor, setup: 'editor', act: visible('.quote-document') },
  { state: 'hours', route: '/business/hours', act: visible('main') },
  { state: 'rates', route: '/business/rates', act: visible('main') },
  { state: 'costs redirect', route: '/business/costs', act: visible('main') },
  { state: 'cost units redirect', route: '/business/cost-units', act: visible('main') },
  { state: 'invalid quote redirect', route: '/business/quotes/not-a-uuid', act: visible('main') },
  { state: 'quote child redirect', route: `${editor}/old`, act: visible('main') },
  { state: 'parked crm redirect', route: '/business/crm/old', act: visible('main') },
  { state: 'crm redirect', route: '/crm', act: visible('main') },
  { state: 'agents', route: '/agents', act: visible('main') },
  { state: 'agent session', route: '/agents/5e000000-0000-4000-8000-000000000001', act: visible('main') },
  { state: 'runs redirect', route: '/runs/example', act: visible('main') },
  { state: 'approvals redirect', route: '/approvals', act: visible('main') },
  { state: 'pacing redirect', route: '/pacing', act: visible('main') },
  { state: 'release history route', route: `/releases/${history.current}`, act: async page => { await expect(page.getByRole('dialog', { name: 'PAIMOS AEON releases' })).toBeVisible() } },
  { state: 'settings redirect', route: '/settings', act: visible('main') },
  { state: 'document profiles', route: '/settings/business/profiles', act: visible('main') },
  { state: 'document profile', route: `/settings/business/profiles/${PROFILE.steel}`, act: visible('main') },
  ...(['personal', 'workspace', 'business', 'projects'] as const).map(section => ({ state: `settings ${section}`, route: `/settings/${section}`, act: visible('main') })),
  { state: 'sign in', route: '/signin', setup: 'signed-out', act: signedOut },
  { state: 'public quote', route: '/offers/sel-demo/tok-example', setup: 'public', act: visible('#pq-title') },
  { state: 'not found', route: '/unknown/audit', act: visible('main') },
  { state: 'dock quote workspace', route: `${quote}?quote=${Q.draft}`, act: visible('.quote-dock') },
  { state: 'inspector Text', route: editor, setup: 'editor', act: inspectorTab('Text') },
  { state: 'inspector Section', route: editor, setup: 'editor', act: inspectorTab('Section') },
  { state: 'inspector Document', route: editor, setup: 'editor', act: inspectorTab('Document') },
  { state: 'release history sheet', route: '/', act: async page => { await visible('main')(page); await page.getByRole('button', { name: /^Release history, version / }).click(); await expect(page.getByRole('dialog', { name: 'PAIMOS AEON releases' })).toBeVisible() } },
  { state: 'new quote dialog', route: quote, act: async page => { await listContent('Onlineshop Erweiterung Weihnachten')(page); await page.getByRole('button', { name: 'New quote' }).first().click(); await expect(page.getByRole('dialog', { name: 'New quote' })).toBeVisible() } },
  { state: 'crop dialog', route: '/settings/personal', act: async page => { await visible('main')(page); const chooser = page.waitForEvent('filechooser'); await page.getByRole('button', { name: /^(Add a photo|Change your photo)$/ }).click(); await (await chooser).setFiles({ name: 'audit.png', mimeType: 'image/png', buffer: makePng(240, 240) }); await expect(page.getByRole('dialog', { name: 'Crop your photo' })).toBeVisible() } },
  { state: 'link dialog', route: `${quote}/${Q.issued}`, act: async page => { await visible('.quote-document')(page); const details = page.getByRole('complementary', { name: 'Details' }); if (!await details.isVisible()) await page.getByRole('button', { name: 'Details' }).click(); await expect(details).toBeVisible(); const revoke = details.getByRole('button', { name: 'Revoke link' }); if (!await revoke.isVisible()) { const create = details.getByRole('button', { name: 'Create link' }); await expect(create).toBeVisible(); await create.click(); await expect(revoke).toBeVisible() } await revoke.click(); await expect(page.getByRole('dialog', { name: 'Revoke the customer link?' })).toBeVisible() } },
  { state: 'quote row menu', route: quote, act: async page => { await listContent('Onlineshop Erweiterung Weihnachten')(page); const row = page.locator(`#quote-${Q.issued}`); await row.hover(); await row.getByRole('button', { name: /^Actions for / }).click(); await expect(page.getByRole('menu')).toBeVisible() } },
  { state: 'account menu', route: '/', act: async page => { await visible('main')(page); await page.getByRole('button', { name: /^Account for/ }).click(); await expect(page.getByRole('dialog', { name: 'Account' })).toBeVisible() } },
  { state: 'command palette', route: '/', act: async page => { await visible('main')(page); await page.keyboard.press('Control+k'); await expect(page.getByRole('dialog', { name: 'Search and commands' })).toBeVisible() } },
]

async function installMocks(page: Page, setup: Setup) {
  if (setup === 'signed-out') {
    await page.route('**/api/**', route => new URL(route.request().url()).pathname === '/api/me'
      ? route.fulfill({ status: 401, json: { error: 'unauthorized', dev_mode: true } })
      : route.fulfill({ status: 404, json: { error: 'not found' } }))
    return
  }
  await mockWork(page, fixtures())
  if (setup === 'public') { await mockPublicQuote(page, { acceptable: true }); return }
  await mockAgents(page, agentData({ me: me.id, projects: { pharos: 'p-pharos', aeon: 'p-aeon', pai: 'p-frozen' }, tickets: { fleet: 'n-1', restore: 'n-2', web: 'n-a1', release: 'n-5', approvals: 'n-6' }, nodes: {
    'p-pharos': { key: 'PRJ-17', title: 'Pharos' }, 'p-aeon': { key: 'PRJ-35', title: 'Aeon' }, 'p-frozen': { key: 'PRJ-26', title: 'Studio infrastructure' },
    'n-1': { key: 'PHAROS-11', title: 'Connect Hetzner Cloud for managed provisioning' }, 'n-2': { key: 'PHAROS-12', title: 'Add an Oracle Cloud connector' },
    'n-a1': { key: 'AEON-1', title: 'Aeon foundation' }, 'n-5': { key: 'PHAROS-15', title: 'Beacon health probes' }, 'n-6': { key: 'PHAROS-16', title: 'Retire the old dashboard' },
  } }))
  await mockBusiness(page, businessData())
  await mockCRM(page, crmData())
  await mockQuotes(page, quoteWorld())
  await mockSettings(page, settingsData({ photo: true }), { photo: true, people: ['22222222-2222-4222-8222-222222222222'] })
  await mockProfiles(page, profileWorld())
  if (setup === 'journey') await mockJourney(page, journeyWorld())
  await mockReleases(page, history)
  if (setup === 'editor') await mockQuoteEditor(page)
}

function domAudit(): Raw[] {
  const result: Raw[] = []
  const add = (kind: Kind, severity: Raw['severity'], selector: string, detail: string) => {
    if (result.filter(f => f.kind === kind).length < 30) result.push({ kind, severity, selector, detail })
  }
  const selector = (el: Element): string => {
    if (el.id) return `#${CSS.escape(el.id)}`
    const parts: string[] = []
    for (let node: Element | null = el; node && parts.length < 5; node = node.parentElement) {
      const cls = [...node.classList].filter(v => /^[\w-]+$/.test(v)).slice(0, 2).map(v => `.${CSS.escape(v)}`).join('')
      const index = node.parentElement ? [...node.parentElement.children].indexOf(node) + 1 : 1
      parts.unshift(`${node.tagName.toLowerCase()}${cls}:nth-child(${index})`)
      if (node.id) { parts[0] = `#${CSS.escape(node.id)}`; break }
    }
    return parts.join(' > ')
  }
  const shown = (el: Element) => { const r = el.getBoundingClientRect(), c = getComputedStyle(el); return r.width > 1 && r.height > 1 && c.visibility === 'visible' && c.display !== 'none' && !el.closest('[aria-hidden="true"], [hidden]') }
  const root = document.documentElement
  if (root.scrollWidth > innerWidth + 1) add('horizontal-overflow', 'serious', 'html', `document ${root.scrollWidth}px wide in ${innerWidth}px viewport`)
  const elements = [...document.querySelectorAll<HTMLElement>('body *')].filter(shown)
  for (const el of elements) {
    const style = getComputedStyle(el), rect = el.getBoundingClientRect()
    let scrollContainer = el.parentElement
    while (scrollContainer && !/auto|scroll|hidden|clip/.test(getComputedStyle(scrollContainer).overflowX)) scrollContainer = scrollContainer.parentElement
    if (scrollContainer && !el.closest('svg')) {
      const pr = scrollContainer.getBoundingClientRect()
      if (rect.width > pr.width + 2 && rect.left < pr.right - 1 && rect.right > pr.right + 2)
        add('horizontal-overflow', 'moderate', selector(el), `${Math.round(rect.width)}px element exceeds ${Math.round(pr.width)}px scroll container ${selector(scrollContainer)}`)
    }
    const directText = [...el.childNodes].some(n => n.nodeType === Node.TEXT_NODE && !!n.textContent?.trim())
    if (directText && el.scrollWidth > el.clientWidth + 2 && /hidden|clip/.test(style.overflowX) && style.textOverflow !== 'ellipsis' && !el.closest('[title], [data-tip], [aria-describedby], [role="tooltip"]') && !el.querySelector('[title], [data-tip]'))
      add('clipped-text', 'moderate', selector(el), `text ${el.scrollWidth}px in ${el.clientWidth}px without ellipsis or tooltip: ${(el.textContent ?? '').trim().slice(0, 70)}`)
    if (/^(BUTTON|A)$/.test(el.tagName) && !el.querySelector('svg, img, [role="img"]')) {
      const own = (el.textContent ?? '').trim()
      if (/^[⋯…×✕✖☰⚙⚲⌄⌃➜→←＋✚✓✔✎✏★☆]{1,3}$/.test(own)) add('text-icon', 'moderate', selector(el), `text glyph used as icon: ${own}`)
    }
  }
  const controls = elements.filter(el => { const r = el.getBoundingClientRect(); return el.matches('button, a[href], input, select, textarea, [role="button"], [role="link"], [role="menuitem"], [role="tab"]') && getComputedStyle(el).pointerEvents !== 'none' && r.bottom > 0 && r.top < innerHeight && r.right > 0 && r.left < innerWidth })
  for (let i = 0; i < controls.length; i++) {
    const a = controls[i], ar = a.getBoundingClientRect()
    if (innerWidth <= 390 && (ar.width < 44 || ar.height < 44) && !a.closest('[role="checkbox"], [role="radio"], [role="switch"]'))
      add('small-touch-target', 'moderate', selector(a), `${Math.round(ar.width)}×${Math.round(ar.height)}px touch target`)
    for (let j = i + 1; j < controls.length; j++) {
      const b = controls[j], br = b.getBoundingClientRect()
      if (a.contains(b) || b.contains(a) || a.closest('label') === b.closest('label') && a.closest('label')) continue
      const overlap = Math.max(0, Math.min(ar.right, br.right) - Math.max(ar.left, br.left)) * Math.max(0, Math.min(ar.bottom, br.bottom) - Math.max(ar.top, br.top))
      if (overlap > 16) add('interactive-overlap', 'serious', selector(a), `overlaps ${selector(b)} by ${Math.round(overlap)}px²`)
    }
  }
  return result
}

async function focusAudit(page: Page): Promise<Raw[]> {
  const result: Raw[] = [], seen = new Set<string>()
  await page.evaluate(() => {
    const baseline = new WeakMap<Element, string>()
    for (const el of document.querySelectorAll('button, a[href], input, select, textarea, [tabindex], [contenteditable]')) {
      if (el === document.activeElement) continue
      const cs = getComputedStyle(el)
      baseline.set(el, `${cs.outlineStyle}|${cs.outlineWidth}|${cs.outlineColor}|${cs.boxShadow}`)
    }
    ;(window as unknown as { auditFocusBaseline: WeakMap<Element, string> }).auditFocusBaseline = baseline
  })
  for (let i = 0; i < 180; i++) {
    await page.keyboard.press('Tab')
    const item = await page.evaluate(() => {
      const el = document.activeElement as HTMLElement | null
      if (!el || el === document.body) return null
      const cs = getComputedStyle(el), rect = el.getBoundingClientRect()
      const parts: string[] = []
      for (let node: Element | null = el; node && node !== document.body; node = node.parentElement) {
        if (node.id) { parts.unshift(`#${CSS.escape(node.id)}`); break }
        const index = node.parentElement ? [...node.parentElement.children].indexOf(node) + 1 : 1
        parts.unshift(`${node.tagName.toLowerCase()}:nth-child(${index})`)
      }
      const path = parts.join(' > ')
      const now = `${cs.outlineStyle}|${cs.outlineWidth}|${cs.outlineColor}|${cs.boxShadow}`
      const before = (window as unknown as { auditFocusBaseline: WeakMap<Element, string> }).auditFocusBaseline.get(el)
      return { path, visible: rect.width > 0 && rect.height > 0, changed: before === undefined || before !== now }
    })
    if (!item) break
    if (seen.has(item.path)) break
    seen.add(item.path)
    if (item.visible && !item.changed)
      result.push({ kind: 'invisible-focus', severity: 'serious', selector: item.path, detail: 'Tab focus did not change outline or box shadow' })
    if (result.length >= 30) break
  }
  return result
}

test('offline route and state audit', async ({ browser }) => {
  test.setTimeout(3_600_000)
  mkdirSync(shotDir, { recursive: true })
  const filtered = scenarios.filter(s => !process.env.AUDIT_FILTER || s.state.includes(process.env.AUDIT_FILTER))
  if (!filtered.length) throw new Error('AUDIT_FILTER matched no states')
  const save = () => writeFileSync(output, `${JSON.stringify([...findings.values()], null, 2)}\n`)
  save()
  for (const theme of themes) for (const width of widths) for (const scenario of filtered) {
    const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: theme, reducedMotion: 'reduce' })
    const page = await context.newPage()
    page.setDefaultTimeout(7000)
    const errors: Raw[] = []
    page.on('console', message => { if (message.type() === 'error') errors.push({ kind: 'console', severity: 'serious', selector: 'window.console', detail: `${message.text()} ${message.location().url}`.slice(0, 300) }) })
    page.on('pageerror', error => errors.push({ kind: 'unhandled-rejection', severity: 'serious', selector: 'window', detail: error.message.slice(0, 300) }))
    await page.addInitScript(() => {
      ;(window as unknown as { auditShift: number }).auditShift = 0
      new PerformanceObserver(list => { for (const entry of list.getEntries() as (PerformanceEntry & { value: number; hadRecentInput: boolean })[]) if (!entry.hadRecentInput) (window as unknown as { auditShift: number }).auditShift += entry.value }).observe({ type: 'layout-shift', buffered: true })
      window.addEventListener('unhandledrejection', event => { (window as unknown as { auditRejections: string[] }).auditRejections ??= []; (window as unknown as { auditRejections: string[] }).auditRejections.push(String(event.reason)) })
    })
    let raw: Raw[] = []
    try {
      await installMocks(page, scenario.setup ?? 'default')
      await page.goto(scenario.route)
      if (scenario.act) await scenario.act(page)
      await page.waitForTimeout(200)
      raw.push(...await page.evaluate(domAudit))
      const axe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
      for (const v of axe.violations.filter(v => v.impact === 'serious' || v.impact === 'critical')) for (const node of v.nodes.slice(0, 10))
        raw.push({ kind: 'axe', severity: v.impact as Raw['severity'], selector: node.target.join(' '), detail: `${v.id}: ${v.help}; ${node.failureSummary?.slice(0, 250) ?? ''}` })
      raw.push(...await focusAudit(page))
      const shift = await page.evaluate(() => (window as unknown as { auditShift: number }).auditShift)
      if (shift > 0.02) raw.push({ kind: 'layout-shift', severity: 'moderate', selector: 'document', detail: `CLS after load: ${shift.toFixed(4)}` })
      const rejections = await page.evaluate(() => (window as unknown as { auditRejections?: string[] }).auditRejections ?? [])
      for (const rejection of rejections) raw.push({ kind: 'unhandled-rejection', severity: 'serious', selector: 'window', detail: rejection.slice(0, 300) })
    } catch (error) { raw.push({ kind: 'scenario-error', severity: 'serious', selector: 'document', detail: String(error).slice(0, 500) }) }
    raw.push(...errors)
    const newRows = raw.filter(row => !findings.has(`${row.kind}|${scenario.route}|${scenario.state}|${theme}|${row.selector}|${row.detail.replace(/\d+(?:\.\d+)?/g, '#')}`))
    let shot = ''
    if (newRows.length) {
      shot = join(shotDir, `${scenario.state.replace(/[^a-z0-9]+/gi, '-').toLowerCase()}-${width}-${theme}.png`)
      try { await page.screenshot({ path: shot, fullPage: true, animations: 'disabled' }) } catch { shot = '' }
    }
    for (const row of raw) {
      const key = `${row.kind}|${scenario.route}|${scenario.state}|${theme}|${row.selector}|${row.detail.replace(/\d+(?:\.\d+)?/g, '#')}`
      const old = findings.get(key)
      if (old) { if (!old.viewport.split(',').includes(String(width))) old.viewport += `,${width}`; continue }
      findings.set(key, { ...row, id: `${row.kind}-${createHash('sha1').update(key).digest('hex').slice(0, 10)}`, route: scenario.route, state: scenario.state, viewport: String(width), theme, screenshot: shot })
    }
    save()
    console.log(`audited ${scenario.state} ${width} ${theme}: ${raw.length} observations`)
    await context.close()
  }
  console.log(`UI audit: ${findings.size} deduplicated findings in ${output}`)
  expect([...findings.values()].filter(f => f.kind === 'scenario-error').map(f => `${f.state}: ${f.detail}`), 'every audit state must be reachable').toEqual([])
})
