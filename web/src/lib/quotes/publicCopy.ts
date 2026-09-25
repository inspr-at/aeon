// SPDX-License-Identifier: AGPL-3.0-only
// The customer's page speaks the document's language: every word, date and amount
// on it. German and English catalogs; the signed-in app stays English.
//
// A document carries no language field yet. A declared one (layout.language, when
// the document schema gains it) wins; until then the page reads the document's own
// text and picks the language it is written in, German when in doubt, as the paper
// itself is German. A page without a document (an unknown link) follows the browser.
import type { QuoteDocumentData } from './types'

export type DocLanguage = 'de' | 'en'
const LOCALE: Record<DocLanguage, string> = { de: 'de-AT', en: 'en-GB' }

const GERMAN = new Set(['und', 'der', 'die', 'das', 'den', 'dem', 'des', 'für', 'mit', 'wir', 'sie', 'ihre', 'ihren', 'ihnen', 'ist', 'sind', 'nicht', 'ein', 'eine', 'einer', 'zu', 'zum', 'zur', 'von', 'bei', 'auf', 'im', 'am', 'wird', 'werden', 'angebot', 'leistung', 'leistungen', 'gerne', 'vielen', 'dank', 'zuzüglich', 'ust'])
const ENGLISH = new Set(['the', 'and', 'for', 'with', 'we', 'you', 'your', 'is', 'are', 'not', 'a', 'an', 'to', 'of', 'this', 'that', 'offer', 'quote', 'our', 'will', 'be', 'by', 'on', 'at', 'only', 'thank', 'please', 'accept', 'according', 'terms', 'work', 'service'])
function words(doc: QuoteDocumentData): string[] {
  const texts = [doc.title, doc.subtitle, doc.legal?.intro, doc.legal?.accept_text, doc.legal?.vat_note,
    ...doc.sections.flatMap(s => [s.heading, s.body, ...s.nodes.map(n => n.text)]),
    ...doc.positions.flatMap(p => [p.short_text, p.long_text])]
  return texts.filter((t): t is string => typeof t === 'string').join(' ').toLowerCase().match(/[\p{L}]+/gu) ?? []
}
export function documentLanguage(doc: QuoteDocumentData | null | undefined): DocLanguage {
  if (!doc) return browserLanguage()
  const declared = (doc.layout as { language?: unknown } | undefined)?.language
  if (typeof declared === 'string' && declared.trim()) return declared.toLowerCase().startsWith('en') ? 'en' : 'de'
  let german = 0, english = 0
  for (const word of words(doc)) {
    if (GERMAN.has(word) || /[äöüß]/.test(word)) german++
    if (ENGLISH.has(word)) english++
  }
  return english > german ? 'en' : 'de'
}
export function browserLanguage(): DocLanguage {
  const langs = typeof navigator === 'undefined' ? [] : navigator.languages?.length ? navigator.languages : [navigator.language]
  const first = langs.find(l => /^(de|en)\b/i.test(l))
  return first?.toLowerCase().startsWith('en') ? 'en' : first ? 'de' : 'en'
}

// ---------- Formats: one locale for dates and amounts on the page ----------
export function formatDay(iso: string | undefined, lang: DocLanguage): string {
  if (!iso || !/^\d{4}-\d{2}-\d{2}/.test(iso)) return ''
  return new Intl.DateTimeFormat(LOCALE[lang], { day: '2-digit', month: '2-digit', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${iso.slice(0, 10)}T00:00:00Z`))
}
export function formatMoment(iso: string | undefined, lang: DocLanguage): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const day = new Intl.DateTimeFormat(LOCALE[lang], { day: '2-digit', month: '2-digit', year: 'numeric' }).format(date)
  const time = new Intl.DateTimeFormat(LOCALE[lang], { hour: '2-digit', minute: '2-digit', hour12: false }).format(date)
  return lang === 'de' ? `${day}, ${time} Uhr` : `${day}, ${time}`
}
// Whole cents as "5.560,00 EUR" (German) or "5,560.00 EUR" (English); the grouping
// is a dot in German as on the printed quote, never a space.
export function formatMoney(cents: number, currency: string, lang: DocLanguage): string {
  const negative = cents < 0
  const abs = BigInt(Math.abs(Math.trunc(cents)))
  const whole = (abs / 100n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, lang === 'de' ? '.' : ',')
  const fraction = String(abs % 100n).padStart(2, '0')
  return `${negative ? '-' : ''}${whole}${lang === 'de' ? ',' : '.'}${fraction} ${currency}`.trim()
}

// ---------- The words ----------
export interface PublicCopy {
  quote: string; quoteNo: (no: string) => string; version: (v: number) => string; loading: string
  missingTitle: string; missingBody: string; failedTitle: string; failedBody: string; retry: string
  forRecipient: (name: string) => string; netTotal: string; dated: string; validUntil: string
  invite: string; accepted: string; acceptedOn: (moment: string) => string; closed: string
  linkEnded: (moment: string) => string; validityEnded: (day: string) => string; replaced: string
  reviewAndAccept: string; pdf: string; receiptPdf: string; newTab: string; documentRegion: string; overflow: (what: string) => string
  thanks: (name: string) => string; recorded: (v: number, moment: string) => string; receiptReady: string; receiptPending: string; openReceipt: string
  acceptTitle: string; acceptLead: (v: number, no: string, total: string) => string; yourName: string; company: string; optional: string
  noteTo: (sender: string) => string; theSender: string; confirm: string; submit: string; submitting: string; nameMissing: string
  evidence: (digest: string) => string; tooMany: string; noLongerAcceptable: string; notSaved: string
  footer: (sender: string, wordmark: string) => string; pageTitle: (no: string, sender: string) => string
}
export const COPY: Record<DocLanguage, PublicCopy> = {
  de: {
    quote: 'Angebot', quoteNo: no => `Angebot ${no}`, version: v => `Version ${v}`, loading: 'Das Angebot wird geladen',
    missingTitle: 'Dieser Link öffnet kein Angebot', missingBody: 'Er ist vielleicht abgelaufen oder wurde widerrufen, oder er wurde nur teilweise kopiert. Der Absender kann Ihnen einen neuen Link schicken.',
    failedTitle: 'Das Angebot konnte nicht geladen werden', failedBody: 'Bitte versuchen Sie es in einem Moment noch einmal.', retry: 'Erneut versuchen',
    forRecipient: name => `Für ${name}`, netTotal: 'Nettosumme', dated: 'Datum', validUntil: 'Gültig bis',
    invite: 'Bitte lesen Sie das Angebot unten. Wenn es für Sie passt, können Sie es am Ende dieser Seite annehmen; ein Konto brauchen Sie dafür nicht.',
    accepted: 'Dieses Angebot wurde angenommen.', acceptedOn: moment => `Dieses Angebot wurde am ${moment} angenommen.`,
    closed: 'Dieses Angebot können Sie weiterhin lesen. Die Annahme ist geschlossen.',
    linkEnded: moment => `Dieser Link ist am ${moment} abgelaufen.`, validityEnded: day => `Das Angebot war bis ${day} gültig.`, replaced: 'Eine neuere Fassung ersetzt es möglicherweise.',
    reviewAndAccept: 'Prüfen und annehmen', pdf: 'PDF öffnen', receiptPdf: 'Annahmebestätigung (PDF)', newTab: ' (öffnet in einem neuen Tab)', documentRegion: 'Angebotsdokument',
    overflow: what => `Ein Teil des Dokuments passt nicht auf seine Seite: ${what}`,
    thanks: name => `Vielen Dank, ${name}`, recorded: (v, moment) => `Ihre Annahme von Version ${v} wurde${moment ? ` am ${moment}` : ''} gespeichert.`,
    receiptReady: 'Ihre Annahmebestätigung ist bereit.', receiptPending: 'Ihre Annahmebestätigung (ein PDF dieses Angebots mit Ihrer Annahme) wird erstellt und erscheint hier.', openReceipt: 'Annahmebestätigung öffnen',
    acceptTitle: 'Angebot annehmen', acceptLead: (v, no, total) => `Sie nehmen Version ${v} des Angebots ${no}${total ? ` über netto ${total}` : ''} an, genau so wie oben dargestellt.`,
    yourName: 'Ihr Name', company: 'Firma', optional: 'optional', noteTo: sender => `Nachricht an ${sender}`, theSender: 'den Absender',
    confirm: 'Ich habe dieses Angebot geprüft und nehme es an.', submit: 'Angebot annehmen', submitting: 'Wird gespeichert…', nameMissing: 'Bitte geben Sie Ihren Namen ein.',
    evidence: digest => `Ihr Name, Ihre Firma, Ihre Nachricht und der Zeitpunkt werden mit dem Fingerabdruck ${digest} des Angebots gespeichert, damit später nachvollziehbar ist, was genau angenommen wurde.`,
    tooMany: 'Zu viele Versuche in kurzer Zeit. Bitte warten Sie eine Minute und versuchen Sie es dann erneut.',
    noLongerAcceptable: 'Dieses Angebot kann nicht mehr angenommen werden. Es wurde geändert, ist abgelaufen oder wurde bereits entschieden; laden Sie die Seite neu, um den aktuellen Stand zu sehen.',
    notSaved: 'Ihre Annahme konnte nicht gespeichert werden. Es wurde nichts gespeichert; bitte versuchen Sie es erneut.',
    footer: (sender, wordmark) => `Diese Seite zeigt nur dieses Angebot${sender ? `, geteilt von ${sender}` : ''}. Erstellt mit ${wordmark}.`,
    pageTitle: (no, sender) => `Angebot ${no}${sender ? ` von ${sender}` : ''}`,
  },
  en: {
    quote: 'Quote', quoteNo: no => `Quote ${no}`, version: v => `Version ${v}`, loading: 'Loading the quote',
    missingTitle: 'This link does not open a quote', missingBody: 'It may have ended or been revoked, or it was copied only in part. The sender can share a new link with you.',
    failedTitle: 'The quote could not be loaded', failedBody: 'Please try again in a moment.', retry: 'Try again',
    forRecipient: name => `For ${name}`, netTotal: 'Net total', dated: 'Date', validUntil: 'Valid until',
    invite: 'Please read the quote below. If it suits you, you can accept it at the end of this page; no account is needed.',
    accepted: 'This quote has been accepted.', acceptedOn: moment => `This quote was accepted on ${moment}.`,
    closed: 'This quote remains available to read. Acceptance is closed.',
    linkEnded: moment => `This link ended on ${moment}.`, validityEnded: day => `The offer was valid until ${day}.`, replaced: 'A newer version may replace it.',
    reviewAndAccept: 'Review and accept', pdf: 'Open PDF', receiptPdf: 'Receipt (PDF)', newTab: ' (opens in a new tab)', documentRegion: 'Quote document',
    overflow: what => `Part of the document did not fit on its page: ${what}`,
    thanks: name => `Thank you, ${name}`, recorded: (v, moment) => `Your acceptance of version ${v} was recorded${moment ? ` on ${moment}` : ''}.`,
    receiptReady: 'Your receipt is ready.', receiptPending: 'Your receipt (a PDF of this quote with the acceptance) is being prepared and will appear here.', openReceipt: 'Open the receipt',
    acceptTitle: 'Accept this quote', acceptLead: (v, no, total) => `You accept version ${v} of quote ${no}${total ? `, net ${total}` : ''}, exactly as shown above.`,
    yourName: 'Your name', company: 'Company', optional: 'optional', noteTo: sender => `Note to ${sender}`, theSender: 'the sender',
    confirm: 'I have reviewed this quote and agree to accept it.', submit: 'Accept quote', submitting: 'Recording…', nameMissing: 'Please enter your name.',
    evidence: digest => `Your name, company, note and the time are recorded with the quote’s fingerprint ${digest}, so everyone can later see what exactly was accepted.`,
    tooMany: 'Too many attempts in a short time. Please wait a minute and try again.',
    noLongerAcceptable: 'This quote can no longer be accepted. It changed, ended or was already decided; reload the page to see where it stands.',
    notSaved: 'Your acceptance could not be saved. Nothing was recorded; please try again.',
    footer: (sender, wordmark) => `This page shows only this quote${sender ? `, shared with you by ${sender}` : ''}. Made with ${wordmark}.`,
    pageTitle: (no, sender) => `Quote ${no}${sender ? ` from ${sender}` : ''}`,
  },
}
