// SPDX-License-Identifier: AGPL-3.0-only
// A release history in the inspr.release-history.v1 shape, modelled on the real
// generated manifest: releases hours and days before now, one reserved version
// that was never published, and evidence from complete to missing.
import type { Page } from '@playwright/test'

const pad = (n: number) => String(n).padStart(2, '0')
export function calendarVersion(at: number) {
  const d = new Date(at)
  return `${pad(d.getUTCFullYear() % 100)}${pad(d.getUTCMonth() + 1)}${pad(d.getUTCDate())}${pad(d.getUTCHours())}${pad(d.getUTCMinutes())}${pad(d.getUTCSeconds())}.0.0`
}
const HOUR = 3_600_000
const sha = (seed: string) => Array.from({ length: 40 }, (_, i) => '0123456789abcdef'[(seed.charCodeAt(i % seed.length) * (i + 7)) % 16]).join('')
const digest = (seed: string) => `sha256:${sha(seed)}${sha(seed + 'x').slice(0, 24)}`

type Change = { type: string; subject: string; tickets: string[] }
interface Spec { hoursAgo: number; headline: string; tickets: string[]; changes: Change[]; reserved?: boolean; evidence?: 'full' | 'no-digest' | 'no-release' }

const SPECS: Spec[] = [
  { hoursAgo: 1, headline: 'time entry editing (AEON-75)', tickets: ['AEON-75'], changes: [
    { type: 'feat', subject: 'feat(AEON-75): correct and delete time entries in the Hours view', tickets: ['AEON-75'] },
    { type: 'fix', subject: 'fix(AEON-75): keep the week total when an entry moves days', tickets: ['AEON-75'] },
    { type: 'test', subject: 'test(AEON-75): hours edit covers undo and conflicts', tickets: ['AEON-75'] },
  ] },
  { hoursAgo: 4, headline: 'wide lists and columns (AEON-74)', tickets: ['AEON-74', 'PHAROS-11'], changes: [
    { type: 'feat', subject: 'feat(AEON-74): wide screens show more columns', tickets: ['AEON-74'] },
    { type: 'feat', subject: 'feat(AEON-74): edit mode with app dropdowns', tickets: ['AEON-74'] },
    { type: 'fix', subject: 'fix(PHAROS-11): the Hetzner token row keeps its width', tickets: ['PHAROS-11'] },
    { type: 'docs', subject: 'docs: explain the column picker', tickets: [] },
  ] },
  { hoursAgo: 5, headline: 'journey release walker', tickets: [], reserved: true, changes: [] },
  { hoursAgo: 7, headline: 'retry a busy BEGIN in release acceptance (PAI-1057)', tickets: ['PAI-1057'], evidence: 'no-digest', changes: [
    { type: 'fix', subject: 'fix(PAI-1057): retry a busy BEGIN in release acceptance transactions', tickets: ['PAI-1057'] },
  ] },
  { hoursAgo: 27, headline: 'project journey (AEON-77)', tickets: ['AEON-77'], changes: [
    { type: 'feat', subject: 'feat(AEON-77): the journey as a third view of the project', tickets: ['AEON-77'] },
    { type: 'feat', subject: 'feat(AEON-77): plan walker with screens', tickets: ['AEON-77'] },
    { type: 'fix', subject: 'fix(AEON-77): dark mode colours in the walker', tickets: ['AEON-77'] },
    { type: 'other', subject: 'B10: journey projection fields', tickets: [] },
  ] },
  { hoursAgo: 50, headline: 'agents workspace', tickets: ['AEON-60'], changes: [
    { type: 'feat', subject: 'feat(AEON-60): sessions, approvals and pacing on one page', tickets: ['AEON-60'] },
    { type: 'refactor', subject: 'refactor: one store for harness sessions', tickets: [] },
  ] },
  { hoursAgo: 98, headline: 'first release', tickets: [], evidence: 'no-release', changes: [
    { type: 'feat', subject: 'feat: projects, tickets and sign-in', tickets: [] },
  ] },
]

export function releaseHistory(now = Date.now(), repository = 'inspr-at/aeon') {
  let sequence = SPECS.filter(s => !s.reserved).length
  const releases = SPECS.map(spec => {
    const at = now - spec.hoursAgo * HOUR
    const version = calendarVersion(at)
    const iso = new Date(at).toISOString()
    const commit = sha(version)
    const changes = [{ type: 'release', subject: `release: v${version}`, tickets: spec.tickets }, ...spec.changes].map((c, i) => ({
      commit: sha(`${version}-${i}`), subject: c.subject, type: c.type, scope: /\(([^)]*)\)/.exec(c.subject)?.[1] ?? '', tickets: c.tickets, at: new Date(at - i * 600_000).toISOString(),
    }))
    if (spec.reserved) {
      return { version, tag: '', release_channel: '', release_sequence: 0, state: 'reserved', reserved_at: iso, tagged_at: null, published_at: null, headline: '', tickets: [], changes: [], changes_omitted: 0,
        evidence: { source_commit: '', source_url: '', image: null, ci: null, release_run: null, release_url: '', unavailable: ['This version was reserved but never published.'] } }
    }
    const published = spec.evidence === 'no-release' ? null : new Date(at + 150_000).toISOString()
    const run = (name: string, id: number, conclusion = 'success') => ({ name, url: `https://github.com/${repository}/actions/runs/${id}`, status: 'completed', conclusion })
    return {
      version, tag: `v${version}`, release_channel: 'stable', release_sequence: sequence--, state: 'published', reserved_at: iso, tagged_at: iso, published_at: published,
      headline: spec.headline, tickets: spec.tickets, changes, changes_omitted: 0,
      evidence: {
        source_commit: commit, source_url: `https://github.com/${repository}/commit/${commit}`,
        image: spec.evidence === 'no-release' ? null : { reference: `ghcr.io/${repository}:${version}`, digest: spec.evidence === 'no-digest' ? '' : digest(version) },
        ci: run('CI', 36000000000 + spec.hoursAgo), release_run: run('Release', 36000000100 + spec.hoursAgo, spec.evidence === 'no-release' ? 'failure' : 'success'),
        release_url: spec.evidence === 'no-release' ? '' : `https://github.com/${repository}/releases/tag/v${version}`,
        unavailable: spec.evidence === 'no-release' ? ['No GitHub release exists for this tag, so it has no publication time or image record.']
          : spec.evidence === 'no-digest' ? ['The GitHub release does not record an image digest.'] : [],
      },
    }
  })
  return {
    schema: 'inspr.release-history.v1', product: 'PAIMOS AEON', repository, version_scheme: 'inspr-calendar-v2', generated_at: new Date(now - HOUR + 60_000).toISOString(),
    source: 'git+github', current: releases[0].version, live_since: new Date(now - 50 * 60_000).toISOString(), releases,
  }
}
export type History = ReturnType<typeof releaseHistory>

// Serves the history and the running version; registered after the page's other
// mocks so these routes win. `server` can later move to a newer version.
export async function mockReleases(page: Page, history: History | Record<string, unknown>, options: { running?: string; brand?: Record<string, string> } = {}) {
  const state = { server: options.running ?? (history as History).current, requests: 0 }
  await page.route('**/api/releases**', route => { state.requests++; return route.fulfill({ json: history }) })
  await page.route('**/api/version', route => route.fulfill({ json: { version: state.server, scheme: 'inspr-calendar-v2', ...(options.brand ? { brand: options.brand } : {}) } }))
  return state
}
