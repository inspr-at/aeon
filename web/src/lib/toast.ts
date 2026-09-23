// SPDX-License-Identifier: AGPL-3.0-only
import { reactive } from 'vue'

export interface Toast { id: number; message: string; tone: 'info' | 'error'; action?: { label: string; run: () => void } }
export const toasts = reactive<Toast[]>([])
let next = 1

// Short, dismissible confirmations. Errors stay a little longer than info.
export function toast(message: string, options: { tone?: Toast['tone']; action?: Toast['action']; timeout?: number } = {}) {
  const item: Toast = { id: next++, message, tone: options.tone ?? 'info', action: options.action }
  toasts.push(item)
  while (toasts.length > 3) toasts.shift()
  setTimeout(() => dismiss(item.id), options.timeout ?? (item.tone === 'error' ? 7000 : 4500))
  return item.id
}
export function dismiss(id: number) {
  const index = toasts.findIndex(item => item.id === id)
  if (index !== -1) toasts.splice(index, 1)
}
