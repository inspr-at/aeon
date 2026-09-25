// SPDX-License-Identifier: AGPL-3.0-only
// Shared words for the Access screens: errors in a sentence, and what a
// deactivation does.
import { AccessError } from '../../lib/access'

// "<what did not happen>: <the server's reason>." A reason names the field when
// there is one, so forms can show it beside that field instead.
export function problem(error: unknown, what: string): string {
  const reason = error instanceof AccessError ? error.message : error instanceof Error ? error.message : ''
  return reason ? `${what}: ${reason.replace(/\.?$/, '.')}` : `${what}. Please try again.`
}
export const fieldOf = (error: unknown) => error instanceof AccessError ? error.field : null
export function deactivatePoints(name: string): string[] {
  const first = name.split(' ')[0]
  return [
    `${first} is signed out everywhere and cannot sign in again.`,
    'Their agent keys are revoked at once.',
    `Their tickets, comments and history stay, still shown as ${first}’s.`,
    'You can reactivate them later; their roles and project access come back as they were.',
  ]
}
