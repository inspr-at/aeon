// SPDX-License-Identifier: AGPL-3.0-only
import { reactive } from 'vue'

// points: what happens, one line each, shown as a list under the body.
export interface ConfirmRequest { title: string; body?: string; points?: string[]; confirmLabel: string; cancelLabel?: string; danger?: boolean }
export const confirmState = reactive<{ request: ConfirmRequest | null; resolve: ((ok: boolean) => void) | null }>({ request: null, resolve: null })

// One styled confirmation dialog for the app (delete, discard changes).
export function confirmAction(request: ConfirmRequest): Promise<boolean> {
  confirmState.resolve?.(false)
  return new Promise(resolve => {
    confirmState.request = request
    confirmState.resolve = resolve
  })
}
export function settleConfirm(ok: boolean) {
  const resolve = confirmState.resolve
  confirmState.request = null
  confirmState.resolve = null
  resolve?.(ok)
}
