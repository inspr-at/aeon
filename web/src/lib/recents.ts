// SPDX-License-Identifier: AGPL-3.0-only
import { reactive } from 'vue'

// Recently opened tickets and projects, for the command palette's empty state.
// Memory only: they last for the session, never in device storage.
export interface RecentTicket { type: 'ticket'; key: string; title: string; state: string; kind: string; projectKey: string }
export interface RecentProject { type: 'project'; key: string; title: string }
export type Recent = RecentTicket | RecentProject
export const recents = reactive<Recent[]>([])
const LIMIT = 8

export function remember(item: Recent) {
  const index = recents.findIndex(entry => entry.type === item.type && entry.key === item.key)
  if (index !== -1) recents.splice(index, 1)
  recents.unshift(item)
  if (recents.length > LIMIT) recents.splice(LIMIT)
}
