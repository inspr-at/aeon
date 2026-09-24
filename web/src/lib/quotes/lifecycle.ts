// SPDX-License-Identifier: AGPL-3.0-only
// Quote lifecycle adapters (api/openapi.yaml): create, issue, revise, duplicate,
// archive, frozen versions, the customer link and the acceptance receipt. Each
// call sends the precondition the server asks for, so a stale view fails loudly
// instead of changing a quote someone else just changed.
import { api, APIError } from '../api'
import type { QuoteDocumentData } from './types'
import type { QuoteRow, QuoteState } from './list'

export interface QuoteProjection {
  quote_node_id: string; project_node_id: string; customer_org_node_id: string; current_version: number
  state: QuoteState; revision: number; offer_no?: string; archived: boolean; project_ref: string
  classic_status: QuoteRow['classic_status']
}
export interface QuoteVersion {
  quote_node_id: string; version: number; currency: string; title: string; content_sha256: string
  created_by_principal_id: string; created_at: string; digest_mode: 'r4-v1' | 'document-v1'; pricing_mode: 'rate-4' | 'cent-half-up-v1'
  total: string; document?: QuoteDocumentData; offer_no?: string; validity_time_zone?: string
}
export interface PublicLink {
  id: string; public_tenant: string; quote_node_id: string; version: number; target_content_sha256: string
  expires_at: string; revoked_at?: string; path?: string; token?: string
}
export type ReceiptState = 'pending' | 'rendering' | 'ready' | 'sending' | 'sent' | 'failed' | 'uncertain'
export interface ConfirmationJob {
  quote_node_id: string; version: number; state: ReceiptState; attempts: number; next_attempt_at: string
  receipt_sha256?: string; renderer_version?: string; updated_at: string
}
export interface AcceptanceNotice { quote_node_id: string; version: number; channel: 'authenticated' | 'public'; accepted_at: string; confirmation_state: string }
export interface Readiness { renderer_available: boolean; smtp_enabled: boolean; smtp_configured: boolean; email_delivery: string }
export interface SettingsState { revision: number; sender: Record<string, unknown> }

const seg = (value: string) => encodeURIComponent(value)
const quote = (id: string) => `/quotes/${seg(id)}`
async function send<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const response = await api(path, body === undefined ? { method } : { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
  const data = await response.json().catch(() => ({})) as Record<string, unknown>
  if (!response.ok) throw new APIError(response.status, typeof data.error === 'string' ? data.error : `Quote request failed (${response.status})`, data)
  return data as T
}
// What a failed lifecycle call means for the person, in the app's words.
export function lifecycleError(e: unknown, fallback: string): string {
  if (!(e instanceof APIError)) return e instanceof TypeError ? 'The server could not be reached. Nothing changed.' : fallback
  if (e.status === 403) return 'You cannot do that with this quote. An admin can.'
  if (e.status === 409 || e.status === 412) return 'Someone changed this quote a moment ago. It is reloaded; try again.'
  if (e.status === 400 && e.message) return `${e.message.charAt(0).toUpperCase()}${e.message.slice(1)}.`
  return fallback
}

export const getQuote = (id: string) => send<QuoteProjection>(quote(id))
export const createQuote = (body: { title: string; customer_org_node_id: string; project_node_id?: string }) => send<QuoteProjection>('/quotes', 'POST', body)
export const getSettings = () => send<SettingsState>('/quotes/settings')
export const finalizeQuote = (id: string, pre: { expected_quote_revision: number; expected_draft_revision: number; expected_document_sha256: string }) =>
  send<QuoteProjection>(`${quote(id)}/finalize`, 'POST', pre)
export const branchQuote = (id: string, pre: { expected_quote_revision: number; expected_version: number; expected_content_sha256: string }) =>
  send<unknown>(`${quote(id)}/draft/branch`, 'POST', pre)
export const duplicateQuote = (id: string, expectedRevision: number) => send<QuoteProjection>(`${quote(id)}/duplicate`, 'POST', { expected_revision: expectedRevision })
export const setArchived = (id: string, expectedRevision: number, archived: boolean) =>
  send<QuoteProjection>(`${quote(id)}/visibility`, 'PATCH', { expected_revision: expectedRevision, archived })
export const listVersions = (id: string) => send<QuoteVersion[]>(`${quote(id)}/versions`)
export const getVersion = (id: string, version: number) => send<QuoteVersion>(`${quote(id)}/versions/${version}`)

// The customer link. Its token is returned once, on creation; afterwards only its metadata.
export async function getLink(id: string, version: number): Promise<PublicLink | null> {
  try { return await send<PublicLink>(`${quote(id)}/versions/${version}/public-link`) } catch (e) { if (e instanceof APIError && e.status === 404) return null; throw e }
}
export const createLink = (id: string, version: number, expiresAt: string) => send<PublicLink>(`${quote(id)}/versions/${version}/public-link`, 'POST', { expires_at: expiresAt })
export const revokeLink = (id: string, version: number) => send<PublicLink>(`${quote(id)}/versions/${version}/public-link/revoke`, 'POST', {})
export const linkUrl = (link: Pick<PublicLink, 'path'>, origin = location.origin) => link.path ? `${origin}${link.path}` : ''

// The acceptance receipt: an immutable PDF, rendered once after acceptance.
export async function getConfirmation(id: string, version: number): Promise<ConfirmationJob | null> {
  try { return await send<ConfirmationJob>(`${quote(id)}/versions/${version}/confirmation`) } catch (e) { if (e instanceof APIError && e.status === 404) return null; throw e }
}
export const retryConfirmation = (id: string, version: number, acknowledgeUncertain: boolean) =>
  send<ConfirmationJob>(`${quote(id)}/versions/${version}/confirmation/retry`, 'POST', { acknowledge_uncertain: acknowledgeUncertain })
export const receiptUrl = (id: string, version: number) => `/api${quote(id)}/versions/${version}/confirmation/receipt`
export const acceptanceNotices = () => send<AcceptanceNotice[]>('/quotes/acceptances')
export const readiness = () => send<Readiness>('/quotes/readiness')

// "3f2a9c…81d0": enough of a digest to compare by eye; the full value is copied.
export const shortDigest = (sha: string) => sha.length > 16 ? `${sha.slice(0, 8)}…${sha.slice(-6)}` : sha
export const RECEIPT_PILL: Record<ReceiptState, string> = { pending: 'Waiting', rendering: 'Making', ready: 'Ready', sending: 'Sending', sent: 'Sent', failed: 'Failed', uncertain: 'Uncertain' }
export const receiptBusy = (state: ReceiptState | undefined) => state === 'pending' || state === 'rendering' || state === 'sending'
