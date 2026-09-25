// SPDX-License-Identifier: AGPL-3.0-only
// Project groups on the Projects page (AEON-136). Every project shows in exactly
// one group for the person looking: one of their own groups, a group an admin
// shared with the workspace, "No group", or "Archived" (the archived projects).
// Groups are personal by default and live in the person's "project-groups"
// preference; a shared group is a server entity (/api/project-groups) whose
// members are the same for everyone. A person's own placement of a project wins
// over the shared one, for them only. Free of Vue for unit tests.
import { sha256FirstByte } from './avatar.ts'

export const NO_GROUP = 'none'
export const ARCHIVED = 'archived'
export const MAX_GROUP_NAME = 60
export type GroupKind = 'personal' | 'shared' | 'none' | 'archived'

// The hues a group marker (a small dot, never an edge) can take.
export const GROUP_HUES = ['teal', 'gold', 'iris', 'rose', 'sage', 'ocean', 'clay', 'plum'] as const
export type GroupHue = typeof GROUP_HUES[number]

export interface PersonalGroup { id: string; name: string }
export interface SharedGroup { id: string; name: string; position: number; project_ids: string[] }
// The person's preference. `hidden` absent means the default: Archived hidden.
export interface GroupPrefs {
  groups?: PersonalGroup[]
  order?: string[]
  hidden?: string[]
  collapsed?: string[]
  place?: Record<string, string>
}
export interface GroupDef { id: string; name: string; kind: GroupKind; hue: GroupHue | null }
// What grouping needs to know about a project.
export interface Groupable { id: string; archived: boolean }

export const sharedId = (uuid: string) => `s:${uuid}`
export const isShared = (id: string) => id.startsWith('s:')
export const isPersonal = (id: string) => id.startsWith('g:')
export const uuidOf = (id: string) => id.slice(2)
export const isUserGroup = (id: string) => isShared(id) || isPersonal(id)

export function hueOf(id: string): GroupHue {
  return GROUP_HUES[sha256FirstByte(id) % GROUP_HUES.length]
}
// A new personal group id: 'g:' and eight base-36 characters.
export function newGroupId(random: () => number = Math.random): string {
  let out = ''
  for (let i = 0; i < 8; i++) out += Math.floor(random() * 36).toString(36)
  return `g:${out}`
}

// ---------- Reading the preference ----------

// Tolerates a hand-edited or older preference: unknown shapes read as empty.
export function readPrefs(raw: unknown): GroupPrefs {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {}
  const value = raw as Record<string, unknown>
  const strings = (list: unknown) => Array.isArray(list) ? [...new Set(list.filter((x): x is string => typeof x === 'string'))] : undefined
  const groups = Array.isArray(value.groups)
    ? value.groups.flatMap(g => g && typeof g === 'object' && typeof (g as PersonalGroup).id === 'string' && isPersonal((g as PersonalGroup).id) && typeof (g as PersonalGroup).name === 'string'
      ? [{ id: (g as PersonalGroup).id, name: (g as PersonalGroup).name }] : [])
    : undefined
  const place: Record<string, string> = {}
  if (value.place && typeof value.place === 'object' && !Array.isArray(value.place)) {
    for (const [project, group] of Object.entries(value.place as Record<string, unknown>)) if (typeof group === 'string') place[project] = group
  }
  const out: GroupPrefs = {}
  if (groups) out.groups = groups
  const order = strings(value.order), hidden = strings(value.hidden), collapsed = strings(value.collapsed)
  if (order) out.order = order
  if (hidden) out.hidden = hidden
  if (collapsed) out.collapsed = collapsed
  if (Object.keys(place).length) out.place = place
  return out
}

export function hiddenOf(prefs: GroupPrefs): Set<string> {
  return new Set(prefs.hidden ?? [ARCHIVED])
}

// Every group in the person's order. Groups not ordered yet join before No group
// and Archived: their own in creation order, then shared ones by position.
export function groupDefs(prefs: GroupPrefs, shared: SharedGroup[]): GroupDef[] {
  const defs = new Map<string, GroupDef>()
  for (const g of prefs.groups ?? []) defs.set(g.id, { id: g.id, name: g.name, kind: 'personal', hue: hueOf(g.id) })
  for (const g of [...shared].sort((a, b) => a.position - b.position || a.name.localeCompare(b.name))) {
    const id = sharedId(g.id)
    defs.set(id, { id, name: g.name, kind: 'shared', hue: hueOf(id) })
  }
  defs.set(NO_GROUP, { id: NO_GROUP, name: 'No group', kind: 'none', hue: null })
  defs.set(ARCHIVED, { id: ARCHIVED, name: 'Archived', kind: 'archived', hue: null })
  const ordered = (prefs.order ?? []).filter(id => defs.has(id))
  const missing = [...defs.keys()].filter(id => !ordered.includes(id))
  const users = missing.filter(isUserGroup), fixed = missing.filter(id => !isUserGroup(id))
  const at = joinAt(ordered)
  const ids = [...ordered.slice(0, at), ...users, ...ordered.slice(at), ...fixed]
  return ids.map(id => defs.get(id)!)
}
// Where a new group joins an order: after the last group of the person's or
// shared one, else before No group and Archived.
function joinAt(order: string[]): number {
  for (let i = order.length - 1; i >= 0; i--) if (isUserGroup(order[i]!)) return i + 1
  const fixed = order.findIndex(id => !isUserGroup(id))
  return fixed === -1 ? order.length : fixed
}

// project id -> shared group id ('s:…') from the server's membership lists.
export function sharedIndex(shared: SharedGroup[]): Map<string, string> {
  const out = new Map<string, string>()
  for (const g of shared) for (const project of g.project_ids) out.set(project, sharedId(g.id))
  return out
}

// Where a project shows: Archived when archived, else the person's own placement
// (while that group exists), else its shared group, else No group.
export function placeOf(project: Groupable, prefs: GroupPrefs, index: Map<string, string>, known: Set<string>): string {
  if (project.archived) return ARCHIVED
  const own = prefs.place?.[project.id]
  if (own && (own === NO_GROUP || known.has(own))) return own
  return index.get(project.id) ?? NO_GROUP
}

export interface Section<T> { group: GroupDef; items: T[] }
// Projects bucketed by group, in group order; `sort` orders each bucket.
export function bucket<T extends Groupable>(projects: T[], defs: GroupDef[], where: (project: T) => string, sort?: (a: T, b: T) => number): Section<T>[] {
  const buckets = new Map<string, T[]>(defs.map(d => [d.id, []]))
  for (const project of projects) (buckets.get(where(project)) ?? buckets.get(NO_GROUP)!).push(project)
  return defs.map(group => ({ group, items: sort ? buckets.get(group.id)!.sort(sort) : buckets.get(group.id)! }))
}

// Section headers show once anything beyond the plain list is in play: a group
// of the person's (or a shared one), or archived projects shown.
export function showsHeaders(defs: GroupDef[], hidden: Set<string>, archivedCount: number): boolean {
  return defs.some(d => isUserGroup(d.id)) || (!hidden.has(ARCHIVED) && archivedCount > 0)
}

// ---------- Names ----------

export function nameProblem(raw: string, defs: GroupDef[], except?: string): string {
  const name = raw.trim()
  if (!name) return 'A group needs a name.'
  if ([...name].length > MAX_GROUP_NAME) return `A group name can be up to ${MAX_GROUP_NAME} characters.`
  if (defs.some(d => d.id !== except && d.name.toLowerCase() === name.toLowerCase())) return `There is already a group called “${name}”.`
  return ''
}

// ---------- Changing the preference (pure: each returns the next value) ----------

export function addGroup(prefs: GroupPrefs, group: PersonalGroup, defs: GroupDef[]): GroupPrefs {
  const order = defs.map(d => d.id)
  order.splice(joinAt(order), 0, group.id)
  return { ...prefs, groups: [...(prefs.groups ?? []), group], order }
}
export function renameGroup(prefs: GroupPrefs, id: string, name: string): GroupPrefs {
  return { ...prefs, groups: (prefs.groups ?? []).map(g => g.id === id ? { ...g, name } : g) }
}
// A group goes: its projects fall back to their shared group or No group.
export function removeGroup(prefs: GroupPrefs, id: string): GroupPrefs {
  const place = Object.fromEntries(Object.entries(prefs.place ?? {}).filter(([, g]) => g !== id))
  const without = (list?: string[]) => list?.filter(x => x !== id)
  return { ...prefs, groups: (prefs.groups ?? []).filter(g => g.id !== id), order: without(prefs.order), hidden: without(prefs.hidden), collapsed: without(prefs.collapsed), place }
}
// One group id stands for another (a personal group that became shared, or back).
export function replaceGroup(prefs: GroupPrefs, from: string, to: string): GroupPrefs {
  const swap = (list?: string[]) => list?.map(x => x === from ? to : x)
  const place = Object.fromEntries(Object.entries(prefs.place ?? {}).filter(([, g]) => g !== from))
  return { ...prefs, groups: (prefs.groups ?? []).filter(g => g.id !== from), order: swap(prefs.order), hidden: swap(prefs.hidden), collapsed: swap(prefs.collapsed), place }
}
export function setHidden(prefs: GroupPrefs, ids: string[], hide: boolean): GroupPrefs {
  const hidden = hiddenOf(prefs)
  for (const id of ids) { if (hide) hidden.add(id); else hidden.delete(id) }
  return { ...prefs, hidden: [...hidden] }
}
export function toggleCollapsed(prefs: GroupPrefs, id: string): GroupPrefs {
  const collapsed = new Set(prefs.collapsed ?? [])
  if (collapsed.has(id)) collapsed.delete(id); else collapsed.add(id)
  return { ...prefs, collapsed: [...collapsed] }
}
// Moves a group to sit before `before` (or last when null), in the full order.
export function reorderGroup(prefs: GroupPrefs, defs: GroupDef[], id: string, before: string | null): GroupPrefs {
  const order = defs.map(d => d.id).filter(x => x !== id)
  const at = before === null ? order.length : order.indexOf(before)
  order.splice(at === -1 ? order.length : at, 0, id)
  return { ...prefs, order }
}
export function stepGroup(prefs: GroupPrefs, defs: GroupDef[], id: string, step: -1 | 1): GroupPrefs {
  const order = defs.map(d => d.id)
  const index = order.indexOf(id), to = index + step
  if (index === -1 || to < 0 || to >= order.length) return prefs
  ;[order[index], order[to]] = [order[to], order[index]]
  return { ...prefs, order }
}
export function withPlacements(prefs: GroupPrefs, changes: Record<string, string | null>): GroupPrefs {
  const place = { ...(prefs.place ?? {}) }
  for (const [project, group] of Object.entries(changes)) { if (group === null) delete place[project]; else place[project] = group }
  return { ...prefs, place }
}

// ---------- Moving projects ----------

// What a move needs to know about each project.
export interface MoveItem { id: string; archived: boolean; shared: string | null; current: string }
export interface MovePlan {
  // Changes to the person's placements; null removes one.
  place: Record<string, string | null>
  // A change to shared membership (admins): the shared group's uuid, or null for none.
  assign: { group: string | null; projects: string[] } | null
  archive: string[]
  restore: string[]
  moved: string[]
}
// How projects get to `target`:
// - Archived archives them; leaving Archived restores them.
// - Your own group: placed there, for you (shared membership stays).
// - A shared group: admins change its membership for everyone; others can only
//   return a project to the shared group it is already in.
// - No group: admins take projects out of their shared group; for others it is
//   a placement of their own.
export function planMove(items: MoveItem[], target: string, admin: boolean): MovePlan | { refused: string } {
  const plan: MovePlan = { place: {}, assign: null, archive: [], restore: [], moved: [] }
  const moving = items.filter(item => item.current !== target)
  if (!moving.length) return { refused: items.length === 1 ? 'It is already in that group.' : 'They are already in that group.' }
  if (target === ARCHIVED) {
    for (const item of moving) plan.archive.push(item.id)
    plan.moved = plan.archive
    return plan
  }
  for (const item of moving) if (item.archived) plan.restore.push(item.id)
  if (isShared(target)) {
    const uuid = uuidOf(target)
    const outsiders = moving.filter(item => item.shared !== target)
    if (outsiders.length && !admin) return { refused: 'Only a workspace admin can add projects to a shared group.' }
    if (outsiders.length) plan.assign = { group: uuid, projects: outsiders.map(item => item.id) }
    for (const item of moving) plan.place[item.id] = null
  } else if (target === NO_GROUP) {
    const inShared = moving.filter(item => item.shared)
    if (admin && inShared.length) plan.assign = { group: null, projects: inShared.map(item => item.id) }
    for (const item of moving) plan.place[item.id] = !admin && item.shared ? NO_GROUP : null
  } else {
    for (const item of moving) plan.place[item.id] = target
  }
  plan.moved = moving.map(item => item.id)
  return plan
}
