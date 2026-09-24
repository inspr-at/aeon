// SPDX-License-Identifier: AGPL-3.0-only
// Declare the pinned JS API without altering the verified vendor bundle.
declare module '*calendar-version-display/version.js' {
  export function renderVersion(element: HTMLElement, value: string, scheme: string, options: {
    config: object; mode: 'pretty' | 'reduced'; brand: string; interactive?: boolean
  }): void
  export function disposeVersion(element: HTMLElement): void
  export function parts(value: string, scheme: string): Record<'v' | 'yy' | 'mm' | 'dd' | 'hh' | 'mi' | 'ss' | 'tail', string> | null
}
