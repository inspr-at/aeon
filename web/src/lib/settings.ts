// SPDX-License-Identifier: AGPL-3.0-only
// Settings: the sections, who sees them, and the server calls they read. Personal
// settings are everyone's; the rest are for workspace admins.
import { api } from './api.ts'

export type SectionId = 'personal' | 'workspace' | 'business' | 'projects'
export interface SettingsSection { id: SectionId; label: string; summary: string; admin: boolean }
export const SETTINGS_SECTIONS: readonly SettingsSection[] = [
  { id: 'personal', label: 'Personal', summary: 'Theme, greeting and keys', admin: false },
  { id: 'workspace', label: 'Workspace', summary: 'Members and agent keys', admin: true },
  { id: 'business', label: 'Business', summary: 'Parts and quote settings', admin: true },
  { id: 'projects', label: 'Projects', summary: 'Ticket types', admin: true },
]
export function sectionOf(param: unknown): SectionId {
  const value = Array.isArray(param) ? param[0] : param
  return SETTINGS_SECTIONS.find(section => section.id === value)?.id ?? 'personal'
}
export const visibleSections = (admin: boolean) => SETTINGS_SECTIONS.filter(section => admin || !section.admin)
// Deep links into a section: /settings/business#quotes.
export const settingsLink = (section: SectionId, anchor?: string) => `/settings/${section}${anchor ? `#${anchor}` : ''}`

async function read<T>(path: string): Promise<T> {
  const response = await api(path)
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw Object.assign(new Error(typeof body?.error === 'string' ? body.error : `Request failed (${response.status})`), { status: response.status })
  }
  return response.json() as Promise<T>
}
export const statusOf = (error: unknown) => (error as { status?: number })?.status ?? 0

// ---------- Profile (GET/PATCH /api/me/profile) ----------
export interface Profile {
  principal_id: string; email: string | null; first_name: string; last_name: string; preferred_name: string; short_name: string
  initials: string; timezone: string; locale: string; greeting_enabled: boolean; avatar_color: string; week_start: number; revision: number
}
export const getProfile = () => read<Profile>('/me/profile')
export async function patchProfile(fields: Partial<Pick<Profile, 'greeting_enabled'>>): Promise<Profile> {
  const response = await api('/me/profile', { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(fields) })
  if (!response.ok) throw Object.assign(new Error('Your setting could not be saved.'), { status: response.status })
  return response.json()
}

// ---------- Agent keys (GET /api/agent-keys, admins) ----------
export interface AgentKey {
  id: string; principal_id: string; name: string; prefix: string; scopes: string[]
  created_at: string; expires_at: string | null; last_used_at: string | null; revoked_at: string | null
}
export const listAgentKeys = async () => (await read<{ keys: AgentKey[] }>('/agent-keys')).keys
export type KeyState = 'active' | 'expired' | 'revoked'
export function keyState(key: AgentKey, now = Date.now()): KeyState {
  if (key.revoked_at) return 'revoked'
  if (key.expires_at && Date.parse(key.expires_at) <= now) return 'expired'
  return 'active'
}

// ---------- Quote settings (GET /api/quotes/settings, staff while Quotes is on) ----------
export interface QuoteSettings {
  revision: number; numbering_time_zone: string; default_currency: string
  sender: Record<string, string>; smtp_confirmation_enabled: boolean; smtp_configured: boolean
}
export const getQuoteSettings = () => read<QuoteSettings>('/quotes/settings')
// The sender as one line: company, city and country, the parts that are set.
export function senderLine(sender: Record<string, unknown> | null | undefined) {
  if (!sender) return ''
  return ['company', 'city', 'country'].map(key => sender[key]).filter((v): v is string => typeof v === 'string' && v.trim() !== '').join(' · ')
}
