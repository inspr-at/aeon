// SPDX-License-Identifier: AGPL-3.0-only
// Quote page zoom: whole percents from 25 to 800, a compact preset list (no 500
// or 700: they stay valid typed values), and the two fit modes. Free of Vue.

export const ZOOM_MIN = 25
export const ZOOM_MAX = 800
export const ZOOM_STEPS = [25, 50, 75, 100, 125, 150, 175, 200, 250, 300, 400, 600, 800] as const
export type ZoomMode = 'width' | 'page' | number
// A4 at 96 dpi, and the desk's breathing room around the page.
export const PAGE_PX = { width: 210 / 25.4 * 96, height: 297 / 25.4 * 96 } as const
export const DESK_PAD = 24

// "125", "125 %" or "125%" as a whole percent in range; anything else is null.
export function parseZoom(raw: string): number | null {
  const text = raw.trim().replace(/\s*%$/, '').trim()
  if (!/^\d{1,3}$/.test(text)) return null
  const value = Number(text)
  return value >= ZOOM_MIN && value <= ZOOM_MAX ? value : null
}
// The next preset beyond the current percent, or null at that end.
export function stepZoom(current: number, direction: 1 | -1): number | null {
  if (direction > 0) return ZOOM_STEPS.find(step => step > current + 0.01) ?? null
  return [...ZOOM_STEPS].reverse().find(step => step < current - 0.01) ?? null
}
const clampZoom = (value: number) => Math.max(ZOOM_MIN, Math.min(ZOOM_MAX, Math.floor(value)))
// The percent a mode means for a desk of this size.
export function zoomPercent(mode: ZoomMode, desk: { width: number; height: number }): number {
  if (typeof mode === 'number') return clampZoom(mode)
  const width = (desk.width - DESK_PAD * 2) / PAGE_PX.width * 100
  if (mode === 'width') return clampZoom(width)
  return clampZoom(Math.min(width, (desk.height - DESK_PAD * 2) / PAGE_PX.height * 100))
}
export function zoomLabel(mode: ZoomMode, percent: number): string {
  return `${typeof mode === 'number' ? clampZoom(mode) : percent} %`
}
export function zoomModeName(mode: ZoomMode): string {
  return mode === 'width' ? 'Fit width' : mode === 'page' ? 'Fit page' : `${mode} %`
}
// A stored preference back as a mode; unknown values fall back to the default.
export function readZoom(value: unknown, fallback: ZoomMode): ZoomMode {
  if (value === 'width' || value === 'page') return value
  if (typeof value === 'number' && Number.isInteger(value) && value >= ZOOM_MIN && value <= ZOOM_MAX) return value
  return fallback
}
