// SPDX-License-Identifier: AGPL-3.0-only
// Typed P1 quote draft adapter. The editor never accepts a stale response as a new baseline.
import { api, APIError } from '../api'
import type { QuoteDocumentData } from './types'
export interface QuoteDraft {
  document: QuoteDocumentData; document_sha256: string; draft_revision: number; quote_revision: number
  schema_version: number; minimum_writer_version: number; base_version: number; updated_at: string; updated_by_principal_id: string
}
export interface MutationReceipt {
  mutation_id: string; acknowledged_revision: number; acknowledged_quote_revision: number
  current_revision: number; current_quote_revision: number; replayed: boolean
  document?: QuoteDocumentData; document_sha256?: string; updated_at?: string; updated_by_principal_id?: string
}
const quotePath = (quoteId: string) => `/quotes/${encodeURIComponent(quoteId)}/draft`
async function parse<T>(response: Response): Promise<T> {
  const body = await response.json().catch(() => ({})) as Record<string, unknown>
  if (!response.ok) throw new APIError(response.status, typeof body.error === 'string' ? body.error : `Quote request failed (${response.status})`, body)
  return body as T
}
export async function getDraft(quoteId: string): Promise<QuoteDraft> {
  return parse<QuoteDraft>(await api(quotePath(quoteId)))
}
export async function saveDraft(quoteId: string, draftRevision: number, document: QuoteDocumentData, clientSessionId: string, mutationId: string): Promise<MutationReceipt> {
  if (!Number.isSafeInteger(draftRevision) || draftRevision < 1) throw new Error('Invalid draft revision')
  return parse<MutationReceipt>(await api(quotePath(quoteId), {
    method: 'PATCH', headers: { 'Content-Type': 'application/json', 'If-Match': `"qd-${draftRevision}"` },
    body: JSON.stringify({ client_session_id: clientSessionId, mutation_id: mutationId, writer_version: 1, document }),
  }))
}
