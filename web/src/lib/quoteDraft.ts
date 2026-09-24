// SPDX-License-Identifier: AGPL-3.0-only
// The next version of a quote while it is being written: lines as typed, their
// exact amounts from the rates in force, the totals the server will freeze, and
// what still blocks saving. No Vue here, so tests run it under plain Node.
import type { CostRate, QuoteVersion, QuoteVersionWrite, Unit } from './business.ts'
import { lineNet, lineTax, parsePercentInput, parseQuantityInput, ratePercent, sumAmounts } from '../components/business/money.ts'

export interface DraftLine { id: string; description: string; costUnitId: string; unit: Unit; quantity: string; tax: string }
export interface Draft {
  quoteId: string; baseVersion: number; baseRevision: number
  recipientId: string; currency: string; terms: string; lines: DraftLine[]
}
export interface LineResult {
  line: DraftLine; rate: CostRate | null; quantity: string | null; taxRate: string | null
  net: string | null; tax: string | null; problem: string; blank: boolean
}
export interface DraftTotals { net: string; tax: string; total: string; taxes: { rate: string; amount: string }[] }

let counter = 0
export function lineId() { counter += 1; return `l${Date.now().toString(36)}${counter}` }
export function blankLine(previous?: DraftLine): DraftLine {
  return { id: lineId(), description: '', costUnitId: previous?.costUnitId ?? '', unit: previous?.unit ?? 'hour', quantity: '', tax: previous?.tax ?? '20' }
}
export function isBlank(line: DraftLine) { return !line.description.trim() && !line.quantity.trim() }

// A draft that starts from a frozen version (or empty for the first one).
export function draftFrom(quoteId: string, revision: number, version: QuoteVersion | null, fallback: { recipientId?: string; currency?: string } = {}): Draft {
  return {
    quoteId, baseVersion: version?.version ?? 0, baseRevision: revision,
    recipientId: version?.recipient_contact_node_id ?? fallback.recipientId ?? '',
    currency: version?.currency ?? fallback.currency ?? 'EUR',
    terms: version?.terms_markdown ?? '',
    lines: version?.lines.length
      ? version.lines.map(line => ({ id: lineId(), description: line.description, costUnitId: line.cost_unit_node_id, unit: line.unit, quantity: trimDecimal(line.quantity), tax: ratePercent(line.tax_rate) }))
      : [blankLine()],
  }
}
function trimDecimal(value: string) { return value.includes('.') ? value.replace(/\.?0+$/, '') : value }

type RateOf = (costUnitId: string, unit: Unit, currency: string) => CostRate | null
export function evaluate(draft: Draft, rateOf: RateOf): LineResult[] {
  return draft.lines.map((line, index) => {
    const blank = isBlank(line)
    const rate = line.costUnitId ? rateOf(line.costUnitId, line.unit, draft.currency) : null
    const quantity = parseQuantityInput(line.quantity)
    const taxRate = parsePercentInput(line.tax)
    const net = rate && quantity ? lineNet(rate.bill_amount, quantity) : null
    const tax = net && taxRate ? lineTax(net, taxRate) : null
    const at = `Line ${index + 1}`
    let problem = ''
    if (!blank) {
      if (!line.description.trim()) problem = `${at} needs a description.`
      else if (!line.costUnitId) problem = `${at} needs a cost unit.`
      else if (!rate) problem = `${at}: no ${draft.currency} rate per ${line.unit} is in force today.`
      else if (!quantity) problem = `${at}: the quantity must be a positive number with at most four decimals.`
      else if (!taxRate) problem = `${at}: tax must be a percentage from 0 to 100.`
    }
    return { line, rate, quantity, taxRate, net, tax, problem, blank }
  })
}

export function totals(results: LineResult[]): DraftTotals {
  const counted = results.filter(r => !r.blank && r.net)
  const net = sumAmounts(counted.map(r => r.net!))
  const tax = sumAmounts(counted.map(r => r.tax ?? '0'))
  const byRate = new Map<string, string[]>()
  for (const r of counted) if (r.taxRate && r.tax) byRate.set(r.taxRate, [...(byRate.get(r.taxRate) ?? []), r.tax])
  const taxes = [...byRate.entries()].sort(([a], [b]) => b.localeCompare(a)).map(([rate, amounts]) => ({ rate, amount: sumAmounts(amounts) }))
  return { net, tax, total: sumAmounts([net, tax]), taxes }
}

// The first thing that stops this draft from becoming a version, or ''.
export function blocker(draft: Draft, results: LineResult[]): string {
  if (!draft.recipientId) return 'Choose who receives the offer.'
  if (!/^[A-Z]{3}$/.test(draft.currency)) return 'Choose a currency.'
  const problem = results.find(r => r.problem)?.problem
  if (problem) return problem
  if (!results.some(r => !r.blank)) return 'Add at least one line.'
  if (results.filter(r => !r.blank).length > 100) return 'A quote holds at most 100 lines.'
  return ''
}

export function toWrite(draft: Draft, title: string, results: LineResult[]): QuoteVersionWrite {
  return {
    expected_revision: draft.baseRevision, recipient_contact_node_id: draft.recipientId, currency: draft.currency, title: title.trim(), terms_markdown: draft.terms,
    lines: results.filter(r => !r.blank).map(r => ({ description: r.line.description.trim(), cost_unit_node_id: r.line.costUnitId, unit: r.line.unit, quantity: r.quantity!, tax_rate: r.taxRate! })),
  }
}

// Whether the draft differs from the version it started from.
export function changed(draft: Draft, base: QuoteVersion | null): boolean {
  if (!base) return draft.lines.some(line => !isBlank(line)) || !!draft.terms.trim()
  if (draft.recipientId !== base.recipient_contact_node_id || draft.currency !== base.currency || draft.terms !== base.terms_markdown) return true
  const lines = draft.lines.filter(line => !isBlank(line))
  if (lines.length !== base.lines.length) return true
  return lines.some((line, i) => {
    const frozen = base.lines[i]
    return line.description.trim() !== frozen.description || line.costUnitId !== frozen.cost_unit_node_id || line.unit !== frozen.unit
      || parseQuantityInput(line.quantity) !== parseQuantityInput(frozen.quantity) || parsePercentInput(line.tax) !== parsePercentInput(ratePercent(frozen.tax_rate))
  })
}

export function moveLine(lines: DraftLine[], from: number, to: number): DraftLine[] {
  if (to < 0 || to >= lines.length || from === to) return lines
  const next = [...lines]
  const [line] = next.splice(from, 1)
  next.splice(to, 0, line)
  return next
}

// Unsaved drafts live for the session in memory, like other preferences.
const drafts = new Map<string, Draft>()
export const rememberDraft = (draft: Draft) => { drafts.set(draft.quoteId, draft) }
export const recallDraft = (quoteId: string) => drafts.get(quoteId) ?? null
export const forgetDraft = (quoteId: string) => { drafts.delete(quoteId) }
