// SPDX-License-Identifier: AGPL-3.0-only
// Whole seconds only. Integer division on a safe integer is exact.

export function formatDuration(seconds: number): string {
  if (typeof seconds !== 'number' || !Number.isSafeInteger(seconds) || seconds < 0) {
    throw new Error('Duration must be a whole number of seconds.')
  }
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const rest = seconds % 60
  const parts: string[] = []
  if (hours) parts.push(`${hours}h`)
  if (minutes) parts.push(`${minutes}m`)
  if (rest || parts.length === 0) parts.push(`${rest}s`)
  return parts.join(' ')
}
