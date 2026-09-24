// SPDX-License-Identifier: AGPL-3.0-only
import { ref } from 'vue'

// The global error boundary. Errors while a page renders or loads replace the page
// with a calm error page; errors in a click or key handler only toast. The page
// shows a short reference and the error's name and message, never a stack.
// path: where a failed navigation was going, so Try again retries it rather than the old page.
export interface Fatal { kind: 'render' | 'navigation' | 'update'; name: string; message: string; reference: string; at: string; path?: string }
export const fatal = ref<Fatal | null>(null)

// Errors that leave a page unable to show itself: setup, render, async component load,
// component update and the create/mount/update hooks. Vue passes a readable phrase in
// development and an error-reference URL ending in the code in production.
const RENDER_CODES = new Set(['0', '1', '13', '15', 'bc', 'c', 'bm', 'm', 'bu', 'u'])
const RENDER_PHRASES = new Set(['setup function', 'render function', 'async component loader', 'component update', 'beforeCreate hook', 'created hook', 'beforeMount hook', 'mounted hook', 'beforeUpdate hook', 'updated'])
export function isRenderError(info: string): boolean {
  const code = /#runtime-(\w+)$/.exec(info)?.[1]
  return code === undefined ? RENDER_PHRASES.has(info) : RENDER_CODES.has(code)
}
export function isStaleBuild(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error)
  return /dynamically imported module|Importing a module script failed|Failed to fetch dynamically|error loading dynamically imported module|Unable to preload CSS/i.test(message)
}
export function reference(now = Date.now()): string {
  return `E-${now.toString(36).toUpperCase().slice(-6)}`
}
export function reportFatal(error: unknown, kind: Fatal['kind'], path?: string) {
  const err = error instanceof Error ? error : new Error(String(error))
  const stale = isStaleBuild(err)
  fatal.value = {
    kind: stale ? 'update' : kind,
    name: err.name || 'Error',
    message: (err.message || 'Unknown error').split('\n')[0].slice(0, 180),
    reference: reference(),
    at: new Date().toISOString(),
    ...(path ? { path } : {}),
  }
}
export function clearFatal() { fatal.value = null }
