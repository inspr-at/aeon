// SPDX-License-Identifier: AGPL-3.0-only
// The quote a document profile is previewed on (U19, AEON-110): invented content
// for an invented customer, long enough to show a cover, sections with a list,
// a positions table that continues on a second page, the totals and signatures.
// The sender is the workspace's own (Settings › Business); nothing else is real.
import type { QuoteDocumentData, QuoteSender, QuoteLayout, TextNode, QuoteSection, QuotePosition } from './types'
import type { Locale } from './profileForm'

const id = (n: number) => `5a3e0000-0000-4000-8000-${String(n).padStart(12, '0')}`
const paragraph = (n: number, text: string): TextNode => ({ id: id(n), kind: 'paragraph', text })
const item = (n: number, text: string): TextNode => ({ id: id(n), kind: 'item', text, depth: 0, marker: 'disc' })
function section(n: number, heading: string, nodes: TextNode[]): QuoteSection {
  return { id: id(n), heading, body: nodes.map(node => node.text).join('\n'), nodes }
}
function position(n: number, short: string, long: string, quantity: string, unit: string, cents: number): QuotePosition {
  const total = Math.round(Number(quantity) * cents)
  return { id: id(n), pricing_source: 'manual', short_text: short, long_text: long, quantity, unit_label: unit, unit_price_cents: cents, total_cents: total, currency: 'EUR' }
}

const TEXT = {
  'de-AT': {
    title: 'Modernisierung der Hallensteuerung', subtitle: 'Konzept, Umsetzung und Inbetriebnahme in zwei Etappen',
    intro: 'Vielen Dank für Ihre Anfrage. Gerne bieten wir Ihnen die folgenden Leistungen an.',
    sections: [
      ['Ausgangslage', ['Die bestehende Steuerung der Produktionshalle ist über fünfzehn Jahre gewachsen und soll auf eine einheitliche Plattform gebracht werden.', 'Ziel ist ein Betrieb ohne Stillstand, mit klarer Dokumentation für Ihr Team.']],
      ['Leistungsumfang', ['Wir gehen in drei Schritten vor:'], ['Bestandsaufnahme vor Ort und Abstimmung der Anforderungen', 'Umsetzung in zwei Etappen mit gemeinsamer Abnahme', 'Schulung des Teams und Übergabe der Dokumentation']],
      ['Termine', ['Start in der Kalenderwoche nach Auftragserteilung, Abschluss innerhalb von zehn Wochen.']],
    ] as [string, string[], string[]?][],
    positions: [
      ['Bestandsaufnahme', 'Begehung, Interviews und Aufnahme der bestehenden Anlagen', '2', 'Tag', 96000],
      ['Konzept', 'Zielbild, Etappenplan und Abnahmekriterien', '3', 'Tag', 96000],
      ['Umsetzung Etappe 1', 'Steuerung der Hallen A und B', '64', 'Stunde', 11800],
      ['Umsetzung Etappe 2', 'Steuerung der Halle C und der Außenanlagen', '48', 'Stunde', 11800],
      ['Inbetriebnahme', 'Begleitung vor Ort, Protokoll und Übergabe', '1', 'Pauschale', 180000],
      ['Schulung', 'Zwei Termine für das Betriebsteam', '2', 'Termin', 64000],
    ] as [string, string, string, string, number][],
    legal: { accept_text: 'Hiermit nehmen wir das Angebot an.', vat_note: 'Alle Beträge verstehen sich zuzüglich der gesetzlichen Umsatzsteuer.', payment_terms: '30 Tage netto ab Rechnungsdatum.', discount_note: 'Kein Rabatt vereinbart.' },
    recipient: { name: 'Beispiel Stahl GmbH', address: 'Werkstraße 12\n8020 Graz', contact: 'Dana Beispiel', country: 'Österreich', customer_no: 'K-10042' },
    sender: { company: 'Ihr Unternehmen', street: 'Musterweg 1', postal_code: '8010', city: 'Graz', country: 'Österreich', email: 'office@beispiel.invalid' },
  },
  en: {
    title: 'Production hall control upgrade', subtitle: 'Concept, delivery and commissioning in two stages',
    intro: 'Thank you for your enquiry. We are pleased to offer the following services.',
    sections: [
      ['Starting point', ['The control system of the production hall has grown over fifteen years and is to move onto one platform.', 'The aim is operation without downtime and clear documentation for your team.']],
      ['Scope of work', ['We work in three steps:'], ['On-site survey and agreement on the requirements', 'Delivery in two stages with a joint acceptance', 'Training for the team and handover of the documentation']],
      ['Schedule', ['Start in the week after the order, completion within ten weeks.']],
    ] as [string, string[], string[]?][],
    positions: [
      ['Survey', 'Site visit, interviews and inventory of the existing systems', '2', 'day', 96000],
      ['Concept', 'Target picture, stage plan and acceptance criteria', '3', 'day', 96000],
      ['Delivery stage 1', 'Control of halls A and B', '64', 'hour', 11800],
      ['Delivery stage 2', 'Control of hall C and the outdoor area', '48', 'hour', 11800],
      ['Commissioning', 'On-site support, report and handover', '1', 'flat fee', 180000],
      ['Training', 'Two sessions for the operations team', '2', 'session', 64000],
    ] as [string, string, string, string, number][],
    legal: { accept_text: 'We hereby accept this quote.', vat_note: 'All amounts are exclusive of statutory VAT.', payment_terms: 'Net 30 days from the invoice date.', discount_note: 'No discount agreed.' },
    recipient: { name: 'Beispiel Stahl GmbH', address: 'Werkstraße 12\n8020 Graz', contact: 'Dana Beispiel', country: 'Austria', customer_no: 'K-10042' },
    sender: { company: 'Your company', street: 'Sample Lane 1', postal_code: '8010', city: 'Graz', country: 'Austria', email: 'office@example.invalid' },
  },
}

export const SAMPLE_OFFER_NO = 'A260925-07'
// The sample for a locale; the workspace's sender and mark when there is one.
export function sampleDocument(locale: Locale, sender?: QuoteSender | null, layout?: QuoteLayout | null): QuoteDocumentData {
  const t = TEXT[locale] ?? TEXT['de-AT']
  let n = 1
  const sections = t.sections.map(([heading, paragraphs, items]) => {
    const nodes = [...paragraphs.map(text => paragraph(100 + n++, text)), ...(items ?? []).map(text => item(100 + n++, text))]
    return section(n++, heading, nodes)
  })
  const positions = t.positions.map(([short, long, quantity, unit, cents], i) => position(300 + i, short, long, quantity, unit, cents))
  const own = sender?.company ? sender : null
  return {
    schema_version: 1, minimum_writer_version: 2, title: t.title, subtitle: t.subtitle, project_ref: 'PRJ-2026-014',
    offer_date: '2026-09-21', valid_until: '2026-10-21', currency: 'EUR',
    sender: { ...(own ?? t.sender), contact_person: own?.contact_person || (locale === 'en' ? 'Alex Sample' : 'Alex Beispiel') },
    recipient: { ...t.recipient }, legal: { intro: t.intro, ...t.legal }, layout: { ...(layout ?? {}) },
    sections, positions, net_total_cents: positions.reduce((sum, p) => sum + p.total_cents, 0),
  }
}
