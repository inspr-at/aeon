// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test, type Page } from '@playwright/test'
import { fixtures, mockWork } from '../work-fixtures'
import { mockQuoteEditor, QUOTE_ID, SECTIONS } from '../quote-inspector-fixtures'

const people = [
  { id: 'aaaaaaaa-0000-4000-8000-aaaaaaaaaaaa', name: 'Alex Example' },
  { id: 'bbbbbbbb-0000-4000-8000-bbbbbbbbbbbb', name: 'Riley Example' },
]

test('two editors see named presence, server expiry removes an anchor, and print hides overlays', async ({ browser }) => {
  const contexts = await Promise.all([browser.newContext({ viewport: { width: 1440, height: 900 } }), browser.newContext({ viewport: { width: 1440, height: 900 } })])
  const sessions = new Map<string, { session_id: string; principal_id: string; name: string; mode: string; observed_revision: number; expires_at: string; anchor?: unknown }>()
  const pages: Page[] = []
  try {
    for (let i = 0; i < 2; i++) {
      const person = people[i]!
      const page = await contexts[i]!.newPage()
      pages.push(page)
      await mockWork(page, fixtures())
      await mockQuoteEditor(page)
      await page.route('**/api/me', route => route.fulfill({ json: { principal: { id: person.id, name: person.name, kind: 'person', roles: ['admin'] }, tenant: { id: 't1', name: 'Example Studio' } } }))
      await page.route(`**/api/quotes/${QUOTE_ID}/presence**`, route => {
        const request = route.request(), method = request.method(), path = new URL(request.url()).pathname
        const id = `${i + 1}1111111-1111-4111-8111-111111111111`.slice(0, 36)
        if (method === 'DELETE') { sessions.delete(person.id); return route.fulfill({ json: {} }) }
        if (method === 'POST') sessions.set(person.id, { session_id: id, principal_id: person.id, name: person.name, mode: 'viewing', observed_revision: 1, expires_at: '2099-01-01T00:00:00Z', anchor: { section_id: SECTIONS[0], observed_revision: 1, fidelity: 'section' } })
        if (method === 'PATCH') {
          const body = request.postDataJSON() as { mode: string; anchor?: unknown }
          const current = sessions.get(person.id)
          if (current) sessions.set(person.id, { ...current, mode: body.mode, anchor: body.anchor })
        }
        if (path === `/api/quotes/${QUOTE_ID}/presence` && method === 'POST') return route.fulfill({ json: { session_id: id, snapshot: { sessions: [...sessions.values()], draft_revision: 1, quote_revision: 1, state: 'draft' } } })
        return route.fulfill({ json: { sessions: [...sessions.values()], draft_revision: 1, quote_revision: 1, state: 'draft' } })
      })
    }
    const [alex, riley] = pages as [Page, Page]
    await alex.goto(`/business/quotes/${QUOTE_ID}`)
    await expect(alex.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
    await expect.poll(() => sessions.has(people[0]!.id)).toBe(true)
    await riley.goto(`/business/quotes/${QUOTE_ID}`)
    await expect(riley.locator('.quote-document')).toHaveAttribute('data-quote-ready', 'true')
    await expect(riley.locator('.quote-presence-overlays .label')).toContainText('Alex Example')
    await alex.bringToFront()
    await alex.evaluate(() => window.dispatchEvent(new Event('focus')))
    await expect(alex.locator('.quote-presence-overlays .label')).toContainText('Riley Example')
    await alex.emulateMedia({ media: 'print' })
    await expect(alex.locator('.quote-presence-overlays')).toBeHidden()
    await alex.emulateMedia({ media: 'screen' })
    await riley.goto('/business')
    // The mock server advances beyond Riley's lease; no stale user remains.
    sessions.delete(people[1]!.id)
    await alex.evaluate(() => window.dispatchEvent(new Event('focus')))
    await expect(alex.locator('.quote-presence-overlays .label')).toHaveCount(0)
  } finally { await Promise.all(contexts.map(context => context.close())) }
})
