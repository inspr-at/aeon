// SPDX-License-Identifier: AGPL-3.0-only
import { api } from '../api'
import type { QuoteProfileDefinition, QuoteProfileSnapshot } from './types'
import { decimalCents } from './layout'

export interface QuoteProfile { id: string; name: string; revision: number; definition: QuoteProfileDefinition; archived: boolean }
export const profileAssetUrl = (id: string) => `/api/quote-profiles/assets/${encodeURIComponent(id)}`
const json = async <T>(pending: Promise<Response>): Promise<T> => {
  const response = await pending
  if (!response.ok) {
    const data = await response.json().catch(() => ({})) as { error?: string }
    throw new Error(data.error || `Profile request failed (${response.status})`)
  }
  return response.json() as Promise<T>
}
export const listProfiles = () => json<QuoteProfile[]>(api('/quote-profiles'))
export const saveProfile = (name: string, definition: QuoteProfileDefinition, existing?: QuoteProfile) => json<QuoteProfile>(api(existing ? `/quote-profiles/${existing.id}` : '/quote-profiles', {
  method: existing ? 'PATCH' : 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ name, definition, ...(existing ? { expected_revision: existing.revision } : {}) }),
}))
export const archiveProfile = async (id: string) => { const response = await api(`/quote-profiles/${id}`, { method: 'DELETE' }); if (!response.ok) throw new Error(`Archive failed (${response.status})`) }
export const undoProfile = (profile: QuoteProfile) => json<QuoteProfile>(api(`/quote-profiles/${profile.id}/undo`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ expected_revision: profile.revision }) }))
export const uploadProfileAsset = (file: File) => json<{ id: string; content_type: string }>(api('/quote-profiles/assets', { method: 'POST', headers: { 'Content-Type': 'application/octet-stream' }, body: file }))

export function defaultProfile(): QuoteProfileDefinition {
  return {
    schema: 'inspr.document-profile.v1', layout_variant: 'classic-v1', locale: 'de-AT', fonts: [],
    colors: { ink: '#253335', muted: '#637477', soft: '#91a1a3', accent: '#287f78', rule: '#d5dfdf', paper: '#ffffff' },
    typography: { body_pt: '10', title_pt: '17', section_pt: '18', table_pt: '9.6', footer_pt: '7.5' },
    page: { width_mm: '210', height_mm: '297', top_mm: '18', right_mm: '20', bottom_mm: '16', left_mm: '22' },
    cover: { top_mm: '11', title_gap_mm: '7', columns_gap_mm: '8', columns_padding_mm: '6' },
    sections: { numbering: 'upper-roman', heading_case: 'upper' },
    positions_table: { columns: [
      { key: 'position', width_mm: '9' }, { key: 'description', width_mm: '71' }, { key: 'quantity', width_mm: '15' },
      { key: 'unit', width_mm: '22' }, { key: 'unit_price', width_mm: '24' }, { key: 'total', width_mm: '27' },
    ], separator: 'rule', repeat_header: true },
    totals: { vat: 'note', discount: 'hidden', net_label: 'Nettosumme' },
    payment_terms: { position: 'sections', heading: 'Zahlungsbedingungen' },
    acceptance: { signature_columns: 2, gap_mm: '14', lead_mm: '28' },
    footer: { width_mm: '33', offset_mm: '0', page_number_format: 'SEITE {page} VON {total}' },
    labels: { quote: 'ANGEBOT', recipient: 'Auftraggeber', positions: 'Leistungsaufstellung', number: 'Angebotsnummer', date: 'Angebotsdatum', customer: 'Kundennummer', valid: 'Gültig bis', contact: 'Ansprechpartner', project: 'Projektreferenz', net: 'Nettosumme', signature_customer: 'Ort, Datum, Unterschrift Auftraggeber', signature_sender: 'Ort, Datum, Unterschrift Auftragnehmer' },
  }
}

export const profileLabel = (profile: QuoteProfileSnapshot | null | undefined, key: string, fallback: string) => profile?.definition.labels[key] || fallback
export const pageNumber = (profile: QuoteProfileSnapshot | null | undefined, page: number, total: number) => (profile?.definition.footer.page_number_format || '{page} / {total}').replaceAll('{page}', String(page)).replaceAll('{total}', String(total))
export function profileMoney(cents: number, currency: string, profile: QuoteProfileSnapshot | null | undefined): string {
  const [whole, fraction] = decimalCents(cents).split('.')
  const mark = profile?.definition.locale === 'en' ? ',' : '.'
  const decimal = profile?.definition.locale === 'en' ? '.' : ','
  const amount = `${whole!.replace(/\B(?=(\d{3})+(?!\d))/g, mark)}${decimal}${fraction}`
  return profile?.definition.layout_variant === 'classic-v1' && currency === 'EUR' ? `€ ${amount}` : `${amount} ${currency}`
}

export function profileStyle(profile: QuoteProfileSnapshot | null | undefined): Record<string, string> {
  if (!profile) return {}
  const d = profile.definition
  const style: Record<string, string> = {
    '--ink': d.colors.ink, '--ink-2': d.colors.muted, '--ink-3': d.colors.soft, '--teal': d.colors.accent,
    '--line': d.colors.rule, '--line-2': d.colors.rule, '--quote-paper': d.colors.paper,
    '--quote-top': `${d.page.top_mm}mm`, '--quote-right': `${d.page.right_mm}mm`, '--quote-bottom': `${d.page.bottom_mm}mm`, '--quote-left': `${d.page.left_mm}mm`,
    '--quote-content-height': `${297 - Number(d.page.top_mm) - Number(d.page.bottom_mm) - 22}mm`,
    '--quote-body-size': `${d.typography.body_pt || '10'}pt`, '--quote-title-size': `${d.typography.title_pt || '17'}pt`,
    '--quote-section-size': `${d.typography.section_pt || '18'}pt`, '--quote-table-size': `${d.typography.table_pt || '9.6'}pt`,
    '--quote-footer-size': `${d.typography.footer_pt || '7.5'}pt`,
    '--quote-cover-top': `${d.cover.top_mm || '11'}mm`, '--quote-title-gap': `${d.cover.title_gap_mm || '7'}mm`,
    '--quote-columns-gap': `${d.cover.columns_gap_mm || '8'}mm`, '--quote-columns-padding': `${d.cover.columns_padding_mm || '6'}mm`,
    '--quote-signature-gap': `${d.acceptance.gap_mm}mm`, '--quote-signature-lead': `${d.acceptance.lead_mm}mm`,
    '--quote-footer-width': `${d.footer.width_mm}mm`, '--quote-footer-offset': `${d.footer.offset_mm}mm`,
    '--quote-heading-transform': d.sections.heading_case === 'upper' ? 'uppercase' : 'none',
  }
  d.positions_table.columns.forEach((column, i) => { style[`--quote-column-${i + 1}`] = `${column.width_mm}mm` })
  for (const role of ['body', 'display'] as const) if (d.fonts.some(f => f.role === role)) style[`--quote-${role}-font`] = `"QuoteProfile${profile.id.replaceAll('-', '')}${role}"`
  return style
}

const fontLoads = new Map<string, Promise<void>>()
export function loadProfileFonts(profile: QuoteProfileSnapshot | null | undefined): Promise<void> {
  if (!profile?.definition.fonts.length) return Promise.resolve()
  const key = `${profile.id}:${profile.revision}`
  let pending = fontLoads.get(key)
  if (!pending) {
    pending = Promise.all(profile.definition.fonts.map(async face => {
      const name = `QuoteProfile${profile.id.replaceAll('-', '')}${face.role}`
      const font = new FontFace(name, `url("${profileAssetUrl(face.asset_id)}")`, { weight: String(face.weight), style: face.style })
      await font.load()
      document.fonts.add(font)
    })).then(() => undefined)
    fontLoads.set(key, pending)
  }
  return pending
}
