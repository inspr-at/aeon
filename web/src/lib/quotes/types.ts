// SPDX-License-Identifier: AGPL-3.0-only
// P1 QuoteDocument wire model. IDs are durable UUIDs; offsets are UTF-16.
export type QuoteMarker = 'disc' | 'circle' | 'square' | 'dash' | 'decimal'
export type ListMode = 'none' | 'bullet' | 'numbered'
export type NumberingMode = 'restart' | 'continue' | 'start' | 'follow' | 'section' | 'independent' | 'bound' | 'unbound'
export type MarkName = 'bold' | 'italic'
export interface InlineMark { start: number; end: number; bold?: boolean; italic?: boolean }
export interface TextNode {
  id: string; kind: 'paragraph' | 'item'; text: string; depth?: number; marker?: QuoteMarker
  numbering?: 'outline'; list_start?: number; list_continue?: boolean; section_bound?: boolean
  glyph?: string; marker_x_mm?: string; marker_y_mm?: string; text_start_mm?: string; marks?: InlineMark[]
}
export interface QuoteSection { id: string; heading: string; body: string; nodes: TextNode[] }
export interface QuotePosition {
  id: string; pricing_source: 'manual' | 'cost_unit'; short_text: string; long_text: string
  quantity: string; unit_label: string; unit_price_cents: number; total_cents: number; currency: string
  cost_unit_node_id?: string; rate_unit?: 'hour' | 'day' | 'item'
}
export interface QuoteSender {
  company?: string; street?: string; postal_code?: string; city?: string; country?: string
  register_no?: string; register_court?: string; email?: string; phone?: string; website?: string
  uid?: string; bank_name?: string; iban?: string; bic?: string; contact_person?: string
  logo_file_id?: string; logo_sha256?: string
}
export interface QuoteRecipient {
  name?: string; address?: string; contact?: string; country?: string; customer_no?: string
  email?: string; contact_node_id?: string
}
export interface QuoteLegal { intro?: string; accept_text?: string; vat_note?: string }
export interface QuoteLayout {
  logo_width_mm?: string; logo_offset_mm?: string; logo_file_id?: string; logo_sha256?: string; page_style?: string
}
export interface QuoteDocumentData {
  schema_version: 1; minimum_writer_version: 1; title: string; subtitle: string; project_ref: string
  offer_date: string; valid_until: string; currency: string; sender: QuoteSender; recipient: QuoteRecipient
  legal: QuoteLegal; layout: QuoteLayout; sections: QuoteSection[]; positions: QuotePosition[]; net_total_cents: number
}
export interface TextPoint { nodeId: string; offset: number }
export interface TextSelection { sectionId: string; anchor: TextPoint; focus: TextPoint }
export interface EditorSelection { sectionId?: string; positionId?: string; text?: TextSelection }
export interface NumberingOptions { mode: NumberingMode; start?: number; sectionBound?: boolean }
export interface OffsetPatch { markerX?: string | null; markerY?: string | null; textStart?: string | null }
export type DocumentSettings = Partial<Pick<QuoteDocumentData, 'title' | 'subtitle' | 'project_ref' | 'offer_date' | 'valid_until' | 'currency'>> & {
  layout?: Partial<QuoteLayout>; legal?: Partial<QuoteLegal>
}
export const newId = (): string => crypto.randomUUID()
