// SPDX-License-Identifier: AGPL-3.0-only
// Why a draft save was refused, in the editor's words: which part of the quote,
// what is wrong with it, and where to go to fix it. The server's 400 carries the
// reasons; this reads the common shapes (a list of { path, message } under
// fields, problems, errors, details, violations or reasons, or a map from path to
// message) and names each path the way the editor shows it: "Section 2
// “Leistungen”: heading", "Position 3: quantity", "Quote date". Free of Vue for
// unit tests.
import type { QuoteDocumentData } from './types'

export interface SaveProblem {
  // Where it is, as the editor names it.
  where: string
  // What is wrong, as a sentence.
  message: string
  // Where "Show" goes: a section or position on the paper, or a field on the Document tab.
  target: { section: string } | { position: string } | { field: 'dates' | 'reference' | 'currency' } | null
}

const LISTS = ['fields', 'problems', 'errors', 'details', 'violations', 'reasons']
const PATH_KEYS = ['path', 'field', 'pointer', 'location', 'loc', 'name']
const MESSAGE_KEYS = ['message', 'reason', 'error', 'detail', 'msg', 'description']
const TOP: Record<string, string> = {
  title: 'Title', subtitle: 'Subtitle', offer_date: 'Quote date', valid_until: 'Valid until', currency: 'Currency', project_ref: 'Project reference',
  recipient: 'Customer address', sender: 'Sender', legal: 'Legal notes', layout: 'Layout', profile: 'Document profile', net_total_cents: 'Total',
  schema_version: 'Document format', minimum_writer_version: 'Document format',
}
const PART: Record<string, string> = {
  heading: 'heading', body: 'text', nodes: 'text', short_text: 'text', long_text: 'description', quantity: 'quantity', unit_label: 'unit',
  unit_price_cents: 'price', total_cents: 'amount', currency: 'currency', cost_unit_node_id: 'rate', rate_unit: 'rate unit', pricing_source: 'pricing',
}

// "sections[2].heading", "sections.2.heading", "/sections/2/heading" and "document.sections[2]" all read as ['sections', 2, 'heading'].
export function pathParts(path: string): (string | number)[] {
  const parts = path.replace(/^\$\.?/, '').replace(/\[(\d+)\]/g, '.$1').split(/[./]/).filter(Boolean)
  if (parts[0] === 'document') parts.shift()
  return parts.map(part => /^\d+$/.test(part) ? Number(part) : part)
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}
function sentence(message: string): string {
  const trimmed = message.trim()
  if (!trimmed) return 'This part was not accepted.'
  const first = trimmed.charAt(0).toUpperCase() + trimmed.slice(1)
  return /[.!?]$/.test(first) ? first : `${first}.`
}
const quoted = (value: string) => value ? ` “${value.length > 40 ? `${value.slice(0, 39)}…` : value}”` : ''

// One problem from a path and a message, named against the document when it is known.
export function describe(path: string, message: string, document: QuoteDocumentData | null): SaveProblem {
  const parts = pathParts(path)
  const [head, index, part] = parts
  if (head === 'sections' && typeof index === 'number') {
    const section = document?.sections[index]
    const rest = typeof part === 'string' ? PART[part] ?? part.replace(/_/g, ' ') : ''
    return { where: `Section ${index + 1}${quoted(section?.heading ?? '')}${rest ? `: ${rest}` : ''}`, message: sentence(message), target: section ? { section: section.id } : null }
  }
  if (head === 'positions' && typeof index === 'number') {
    const position = document?.positions[index]
    const rest = typeof part === 'string' ? PART[part] ?? part.replace(/_/g, ' ') : ''
    return { where: `Position ${index + 1}${quoted(position?.short_text ?? '')}${rest ? `: ${rest}` : ''}`, message: sentence(message), target: position ? { position: position.id } : null }
  }
  const name = typeof head === 'string' ? TOP[head] ?? (head ? head.charAt(0).toUpperCase() + head.slice(1).replace(/_/g, ' ') : 'The quote') : 'The quote'
  const field = head === 'offer_date' || head === 'valid_until' ? 'dates' : head === 'project_ref' ? 'reference' : head === 'currency' ? 'currency' : null
  return { where: name, message: sentence(message), target: field ? { field } : null }
}

// Every reason the response names, at most eight, in the order the server gave them.
export function saveProblems(body: Record<string, unknown> | null | undefined, document: QuoteDocumentData | null): SaveProblem[] {
  if (!body || typeof body !== 'object') return []
  const out: SaveProblem[] = []
  for (const key of LISTS) {
    const value = body[key]
    if (Array.isArray(value)) {
      for (const item of value) {
        if (typeof item === 'string') { out.push(describe('', item, document)); continue }
        if (!item || typeof item !== 'object') continue
        const record = item as Record<string, unknown>
        const path = PATH_KEYS.map(k => record[k]).map(v => Array.isArray(v) ? v.join('.') : text(v)).find(Boolean) ?? ''
        const message = MESSAGE_KEYS.map(k => text(record[k])).find(Boolean) ?? ''
        if (path || message) out.push(describe(path, message, document))
      }
    } else if (value && typeof value === 'object') {
      for (const [path, message] of Object.entries(value as Record<string, unknown>)) {
        const said = Array.isArray(message) ? message.map(text).filter(Boolean).join('; ') : text(message)
        out.push(describe(path, said, document))
      }
    }
    if (out.length) break
  }
  return out.slice(0, 8)
}
