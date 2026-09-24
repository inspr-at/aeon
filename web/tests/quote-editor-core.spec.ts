// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test, type Page } from '@playwright/test'

const sectionId = '11111111-1111-4111-8111-111111111111'
const nodeId = '22222222-2222-4222-8222-222222222222'
const positionId = '33333333-3333-4333-8333-333333333333'
function fixture() {
  return {
    schema_version: 1, minimum_writer_version: 1, title: 'Synthetic quote', subtitle: 'A neutral example', project_ref: 'TEST-1',
    offer_date: '2026-09-24', valid_until: '2026-10-24', currency: 'EUR',
    sender: { company: 'Example Studio', street: 'Sample Lane 1', postal_code: '1000', city: 'Test City', country: 'AT', email: 'studio@example.invalid', contact_person: 'Alex' },
    recipient: { name: 'Sample Customer', address: 'Demo Street 2', country: 'AT', customer_no: 'C-1', email: 'customer@example.invalid' },
    legal: { intro: 'A test introduction.', accept_text: 'I accept this offer.', vat_note: 'Taxes according to the agreed terms.' },
    layout: {},
    sections: [{ id: sectionId, heading: 'Scope', body: 'A😀B', nodes: [{ id: nodeId, kind: 'paragraph', text: 'A😀B', marks: [{ start: 1, end: 3, bold: true }] }] }],
    positions: [{ id: positionId, pricing_source: 'manual', short_text: 'Work', long_text: 'A synthetic item.', quantity: '1.25', unit_label: 'hour', unit_price_cents: 105, total_cents: 131, currency: 'EUR' }],
    net_total_cents: 131,
  }
}
async function blank(page: Page) {
  await page.route('**/api/**', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{}' }))
  await page.goto('/')
}

test('structured prose, all six levels, numbering, section and row changes survive JSON and history', async ({ page }) => {
  await blank(page)
  const result = await page.evaluate(async (document) => {
    const modulePath = '/src/lib/quotes/editor.ts'
    const prosePath = '/src/lib/quotes/prose.ts'
    const { QuoteEditor } = await import(/* @vite-ignore */ modulePath)
    const prose = await import(/* @vite-ignore */ prosePath)
    const editor = new QuoteEditor(document)
    editor.select({ sectionId: document.sections[0].id, text: { sectionId: document.sections[0].id, anchor: { nodeId: document.sections[0].nodes[0].id, offset: 1 }, focus: { nodeId: document.sections[0].nodes[0].id, offset: 3 } } })
    editor.setMarks('italic', true)
    editor.setListMode('numbered')
    const first = editor.document.sections[0].nodes[0]
    let at = { nodeId: first.id, offset: first.text.length }
    let nodes = [first]
    for (let depth = 1; depth <= 5; depth++) {
      const inserted = prose.replaceText(nodes, at, at, '\nLevel ' + depth)
      nodes = inserted.nodes
      at = inserted.caret
      nodes = prose.setListMode(nodes, [nodes.at(-1).id], 'numbered')
      nodes = prose.changeLevel(nodes, [nodes.at(-1).id], 1)
      nodes[nodes.length - 1].depth = depth
    }
    editor.editSection(document.sections[0].id, { nodes })
    editor.select({ sectionId: document.sections[0].id, text: { sectionId: document.sections[0].id, anchor: { nodeId: first.id, offset: 0 }, focus: { nodeId: first.id, offset: 0 } } })
    editor.setNumbering({ mode: 'section' })
    editor.setOffsets({ markerX: '-1.5', markerY: '0.5', textStart: '2.0' })
    const newSection = editor.insertSection(document.sections[0].id)
    editor.moveSection(newSection, 0)
    const newPosition = editor.addPosition(positionId)
    editor.editPosition(newPosition, { short_text: 'Second', quantity: '2.50', unit_price_cents: 199 })
    editor.movePosition(newPosition, 0)
    const beforeUndo = JSON.stringify(editor.document)
    editor.undo()
    editor.redo()
    return { roundTrip: JSON.stringify(JSON.parse(JSON.stringify(editor.document))) === beforeUndo,
      ids: editor.document.sections.flatMap((s: { id: string; nodes: Array<{ id: string }> }) => [s.id, ...s.nodes.map(n => n.id)]),
      labels: prose.markerLabels(nodes, 1),
      marks: editor.document.sections[1].nodes[0].marks,
      offsets: editor.document.sections[1].nodes[0],
      total: editor.document.net_total_cents,
      sectionOrder: editor.document.sections.map((s: { id: string }) => s.id), positionOrder: editor.document.positions.map((p: { id: string }) => p.id) }
  }, fixture())
  expect(result.roundTrip).toBe(true)
  expect(new Set(result.ids).size).toBe(result.ids.length)
  expect(result.labels).toHaveLength(6)
  expect(result.labels[5]).toMatch(/1\.1\.1\.1\.1\.1/)
  expect(result.marks).toEqual([{ start: 1, end: 3, bold: true, italic: true }])
  expect(result.offsets.marker_x_mm).toBe('-1.5')
  expect(result.offsets.marker_y_mm).toBe('0.5')
  expect(result.total).toBe(629)
  expect(result.sectionOrder[1]).toBe(sectionId)
  expect(result.positionOrder[1]).toBe(positionId)
})

test('typed P1 client sends CAS and stable document to the mocked draft API', async ({ page }) => {
  const requests: { method: string; headers: Record<string, string>; body: unknown }[] = []
  const document = fixture()
  await blank(page)
  await page.route('**/api/quotes/*/draft', async route => {
    if (route.request().method() === 'GET') return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ document, document_sha256: 'a'.repeat(64), draft_revision: 3, quote_revision: 7, schema_version: 1, minimum_writer_version: 1, base_version: 0, updated_at: '', updated_by_principal_id: nodeId }) })
    requests.push({ method: route.request().method(), headers: route.request().headers(), body: route.request().postDataJSON() })
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ mutation_id: nodeId, acknowledged_revision: 4, acknowledged_quote_revision: 8, current_revision: 4, current_quote_revision: 8, replayed: false, document }) })
  })
  const result = await page.evaluate(async () => {
    const path = '/src/lib/quotes/api.ts'
    const client = await import(/* @vite-ignore */ path)
    const draft = await client.getDraft('quote-1')
    const saved = await client.saveDraft('quote-1', draft.draft_revision, draft.document, '44444444-4444-4444-8444-444444444444', '22222222-2222-4222-8222-222222222222')
    return { draft, saved }
  })
  expect(result.saved.acknowledged_revision).toBe(4)
  expect(requests[0]?.method).toBe('PATCH')
  expect(requests[0]?.headers['if-match']).toBe('"qd-3"')
  expect((requests[0]?.body as { document: unknown }).document).toEqual(document)
})

test('numbering restart, explicit start, continuation across prose and section binding', async ({ page }) => {
  await blank(page)
  const labels = await page.evaluate(async () => {
    const path = '/src/lib/quotes/prose.ts'
    const prose = await import(/* @vite-ignore */ path)
    const ids = Array.from({ length: 9 }, (_, i) => `eeeeeeee-eeee-4eee-8eee-${String(i).padStart(12, '0')}`)
    const nodes = ids.map((id, i) => ({ id, kind: i === 3 ? 'paragraph' : 'item', text: `Line ${i}`, ...(i === 3 ? {} : { marker: 'decimal', numbering: 'outline', depth: i === 2 || i === 4 ? 1 : 0 }) }))
    let result = prose.setNumbering(nodes, [ids[0]], { mode: 'start', start: 3 })
    result = prose.setNumbering(result, [ids[2]], { mode: 'continue' })
    result = prose.setNumbering(result, [ids[4]], { mode: 'continue' })
    result = prose.setNumbering(result, [ids[5]], { mode: 'restart' })
    result = prose.setNumbering(result, [ids[5]], { mode: 'section' })
    result = prose.setNumbering(result, [ids[6]], { mode: 'continue', sectionBound: true })
    result = prose.setNumbering(result, [ids[7]], { mode: 'start', start: 7 })
    result = prose.setNumbering(result, [ids[8]], { mode: 'continue' })
    return prose.markerLabels(result, 2)
  })
  expect(labels[0]).toBe('3')
  expect(labels[1]).toBe('4')
  expect(labels[2]).toBe('4.1')
  expect(labels[3]).toBe('')
  expect(labels[4]).toBe('4.2')
  expect(labels[5]).toBe('2.1')
  expect(labels[6]).toBe('2.2')
  expect(labels[7]).toBe('7')
  expect(labels[8]).toBe('8')
})
