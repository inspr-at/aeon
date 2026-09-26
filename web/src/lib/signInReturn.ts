// SPDX-License-Identifier: AGPL-3.0-only
const key = 'aeon.signInReturn'

export function safeReturnPath(value: unknown): string {
  return typeof value === 'string' && value.startsWith('/') && !value.startsWith('//') && !value.startsWith('/\\') && !value.startsWith('/signin') ? value : '/'
}

export function pendingSignInReturn(): string {
  try { return safeReturnPath(sessionStorage.getItem(key)) } catch { return '/' }
}

export function rememberSignInReturn(path: string): void {
  try { sessionStorage.setItem(key, safeReturnPath(path)) } catch { /* Storage may be disabled. */ }
}

export function clearSignInReturn(): void {
  try { sessionStorage.removeItem(key) } catch { /* Storage may be disabled. */ }
}

export function takeSignInReturn(): string {
  const path = pendingSignInReturn()
  clearSignInReturn()
  return path
}
