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
export type SectionNumberingStyle = 'decimal' | 'upper-roman' | 'lower-roman' | 'upper-alpha' | 'lower-alpha' | 'none'
export interface QuoteSection {
  id: string; heading: string; body: string; nodes: TextNode[]
  numbering_style?: SectionNumberingStyle; page_break_before?: boolean; keep_together?: boolean
  spacing_before_mm?: string; spacing_after_mm?: string
}
export interface SectionSettingsPatch {
  numberingStyle?: SectionNumberingStyle; pageBreakBefore?: boolean; keepTogether?: boolean
  spacingBeforeMm?: string | null; spacingAfterMm?: string | null
  spacingMm?: string | null
}
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
export interface QuoteLegal { intro?: string; accept_text?: string; vat_note?: string; discount_note?: string; payment_terms?: string }
export interface QuoteLayout {
  logo_width_mm?: string; logo_offset_mm?: string; logo_file_id?: string; logo_sha256?: string; page_style?: string
}
export interface QuoteDocumentData {
  schema_version: 1; minimum_writer_version: 1 | 2; title: string; subtitle: string; project_ref: string
  offer_date: string; valid_until: string; currency: string; sender: QuoteSender; recipient: QuoteRecipient
  legal: QuoteLegal; layout: QuoteLayout; profile?: QuoteProfileSnapshot | null; sections: QuoteSection[]; positions: QuotePosition[]; net_total_cents: number
}
export interface QuoteProfileDefinition {
  schema: 'inspr.document-profile.v1'; layout_variant: 'standard' | 'classic-v1'; locale: 'de-AT' | 'en'
  fonts: { role: 'body' | 'display'; family: string; weight: number; style: 'normal' | 'italic'; asset_id: string }[]
  colors: Record<string, string>; typography: Record<string, string>
  page: { width_mm: string; height_mm: string; top_mm: string; right_mm: string; bottom_mm: string; left_mm: string }
  cover: Record<string, string>; sections: Record<string, string>
  positions_table: { columns: { key: string; width_mm: string }[]; separator: 'rule' | 'none'; repeat_header: boolean }
  totals: { vat: 'note' | 'line' | 'hidden'; discount: 'line' | 'hidden'; net_label: string }
  payment_terms: { position: 'sections' | 'after-totals'; heading: string }
  acceptance: { signature_columns: 1 | 2; gap_mm: string; lead_mm: string }
  footer: { asset_id?: string; dots_asset_id?: string; width_mm: string; offset_mm: string; page_number_format: string }
  labels: Record<string, string>
}
export interface QuoteProfileSnapshot { id: string; revision: number; definition: QuoteProfileDefinition }
export interface TextPoint { nodeId: string; offset: number }
export interface TextSelection { sectionId: string; anchor: TextPoint; focus: TextPoint }
export interface EditorSelection { sectionId?: string; positionId?: string; text?: TextSelection }
export interface NumberingOptions { mode: NumberingMode; start?: number; sectionBound?: boolean }
export interface OffsetPatch { markerX?: string | null; markerY?: string | null; textStart?: string | null }
export type DocumentSettings = Partial<Pick<QuoteDocumentData, 'title' | 'subtitle' | 'project_ref' | 'offer_date' | 'valid_until' | 'currency'>> & {
  layout?: Partial<QuoteLayout>; legal?: Partial<QuoteLegal>
}
export const newId = (): string => crypto.randomUUID()
