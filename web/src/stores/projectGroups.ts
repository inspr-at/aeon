// SPDX-License-Identifier: AGPL-3.0-only
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { APIError, assignProjectGroup, createProjectGroup, deleteProjectGroup, getProjectGroups, undoGroupEvent, updateProjectGroup, type SharedProjectGroup } from '../lib/api'
import { usePreference } from '../lib/preferences'
import {
  ARCHIVED, NO_GROUP, addGroup, groupDefs, hiddenOf, isPersonal, isShared, newGroupId, placeOf, placementsOf, planMove, readPrefs, removeGroup,
  renameGroup, reorderGroup, replaceGroup, restoreGroup, setHidden, sharedId, sharedIndex, stepGroup, toggleCollapsed, uuidOf, withPlacements,
  type GroupDef, type GroupPrefs, type MoveItem,
} from '../lib/projectGroups'
import { useSession } from './session'
import { useProjects, type Project } from './projects'

// A change that can be taken back: the toast's Undo runs it.
export type Undo = () => Promise<void>
export interface MoveOutcome { moved: string[]; undo: Undo }

// The Projects page's groups: the person's own (the "project-groups" preference)
// and the workspace's shared ones (/api/project-groups), read together. Every
// change is optimistic and answers how to undo it. Servers without shared groups
// (the route missing) simply have none.
export const useProjectGroups = defineStore('projectGroups', () => {
  const session = useSession()
  const projects = useProjects()
  const pref = usePreference<GroupPrefs>('project-groups')
  const shared = ref<SharedProjectGroup[]>([])
  const sharedReady = ref(false)
  const sharedAvailable = ref(true)
  let request: Promise<void> | undefined
  let loadedAt = 0

  const prefs = computed<GroupPrefs>(() => readPrefs(pref.value.value))
  const defs = computed<GroupDef[]>(() => groupDefs(prefs.value, shared.value))
  const known = computed(() => new Set(defs.value.map(d => d.id)))
  const index = computed(() => sharedIndex(shared.value))
  const hidden = computed(() => hiddenOf(prefs.value))
  const collapsed = computed(() => new Set(prefs.value.collapsed ?? []))
  const admin = computed(() => session.identity?.principal.roles?.includes('admin') ?? false)
  const ready = computed(() => sharedReady.value)

  function where(project: Pick<Project, 'id' | 'archived'>): string { return placeOf(project, prefs.value, index.value, known.value) }
  // The shared group a project belongs to for everyone ('s:…'), whatever this person placed.
  function sharedOf(id: string): string | null { return index.value.get(id) ?? null }
  function def(id: string): GroupDef | undefined { return defs.value.find(d => d.id === id) }
  function save(next: GroupPrefs) { pref.save(next as GroupPrefs, 0) }

  function load(force = false): Promise<void> {
    if (request) return request
    if (sharedReady.value && !force && Date.now() - loadedAt < 60_000) return Promise.resolve()
    request = (async () => {
      try {
        const [groups] = await Promise.all([getProjectGroups().catch(error => {
          if (error instanceof APIError && (error.status === 404 || error.status === 405 || error.status === 501)) { sharedAvailable.value = false; return { items: [] } }
          throw error
        }), pref.ready])
        shared.value = groups.items
        loadedAt = Date.now()
      } catch {
        // Shared groups could not be read: the person's own groups still work.
      } finally {
        sharedReady.value = true
        request = undefined
      }
    })()
    return request
  }

  // ---------- Groups ----------
  function create(name: string): string {
    const id = newGroupId()
    save(addGroup(prefs.value, { id, name: name.trim() }, defs.value))
    return id
  }
  async function rename(id: string, name: string): Promise<void> {
    const clean = name.trim()
    if (isPersonal(id)) { save(renameGroup(prefs.value, id, clean)); return }
    if (!isShared(id)) return
    const before = shared.value
    shared.value = shared.value.map(g => g.id === uuidOf(id) ? { ...g, name: clean } : g)
    try {
      const out = await updateProjectGroup(uuidOf(id), { name: clean })
      if (out.group) shared.value = shared.value.map(g => g.id === out.group!.id ? out.group! : g)
    } catch (error) { shared.value = before; throw error }
  }
  // A group goes; its projects fall back to No group (or their shared group).
  async function remove(id: string): Promise<Undo> {
    const before = prefs.value
    if (isPersonal(id)) {
      save(removeGroup(before, id))
      // Undo brings back this group only; anything changed since stays.
      return async () => { save(restoreGroup(prefs.value, before, id)) }
    }
    const groupsBefore = shared.value
    shared.value = shared.value.filter(g => g.id !== uuidOf(id))
    save(removeGroup(before, id))
    try {
      const out = await deleteProjectGroup(uuidOf(id))
      return async () => {
        if (out.event_id != null) await undoGroupEvent(out.event_id)
        save(before)
        await load(true)
      }
    } catch (error) { shared.value = groupsBefore; save(before); throw error }
  }
  // Admins share one of their groups: the server takes its projects, and the
  // group's place in the person's order, visibility and folding carry over.
  async function share(id: string, projectIds: string[]): Promise<Undo> {
    const group = def(id)
    if (!group || !isPersonal(id)) throw new Error('Only your own groups can be shared.')
    const before = prefs.value
    const position = defs.value.filter(d => d.kind === 'shared').length
    const out = await createProjectGroup({ name: group.name, project_ids: projectIds, position })
    if (!out.group) throw new Error('The group could not be shared.')
    shared.value = [...shared.value, out.group]
    save(replaceGroup(before, id, sharedId(out.group.id)))
    return async () => {
      if (out.event_id != null) await undoGroupEvent(out.event_id)
      save(before)
      await load(true)
    }
  }
  // Admins stop sharing: it becomes their own group again, with the same projects.
  async function unshare(id: string, projectIds: string[]): Promise<Undo> {
    const group = def(id)
    if (!group || !isShared(id)) throw new Error('This group is not shared.')
    const before = prefs.value
    const groupsBefore = shared.value
    const local = newGroupId()
    let next = addGroup(before, { id: local, name: group.name }, defs.value)
    next = reorderGroup(next, groupDefs(next, shared.value), local, id)
    next = withPlacements(next, Object.fromEntries(projectIds.map(project => [project, local])))
    if (hidden.value.has(id)) next = setHidden(next, [local], true)
    shared.value = shared.value.filter(g => g.id !== uuidOf(id))
    save(removeGroup(next, id))
    try {
      const out = await deleteProjectGroup(uuidOf(id))
      return async () => {
        if (out.event_id != null) await undoGroupEvent(out.event_id)
        save(before)
        await load(true)
      }
    } catch (error) { shared.value = groupsBefore; save(before); throw error }
  }

  // ---------- Visibility, folding, order (the person's own view) ----------
  function setVisible(ids: string[], visible: boolean) { save(setHidden(prefs.value, ids, !visible)) }
  function toggleVisible(id: string) { setVisible([id], hidden.value.has(id)) }
  function toggleFold(id: string) { save(toggleCollapsed(prefs.value, id)) }
  function reorder(id: string, before: string | null) { save(reorderGroup(prefs.value, defs.value, id, before)) }
  function step(id: string, delta: -1 | 1) { save(stepGroup(prefs.value, defs.value, id, delta)) }

  // ---------- Moving projects ----------
  // Moves projects into `target`: placements at once, then any shared membership
  // and archive changes on the server. A refusal throws its reason.
  async function move(list: Project[], target: string): Promise<MoveOutcome> {
    const items: MoveItem[] = list.map(p => ({ id: p.id, archived: p.archived, shared: index.value.get(p.id) ?? null, current: where(p) }))
    const plan = planMove(items, target, admin.value)
    if ('refused' in plan) throw new Error(plan.refused)
    const before = prefs.value
    // Undo and failure put back only these projects' placements.
    const inverse = placementsOf(before, Object.keys(plan.place))
    const states = new Map(list.map(p => [p.id, p.state]))
    const groupsBefore = shared.value
    if (Object.keys(plan.place).length) save(withPlacements(before, plan.place))
    let eventId: number | null = null
    const archived: string[] = [], restored: string[] = []
    try {
      if (plan.assign) {
        const members = new Set(plan.assign.projects)
        const target = plan.assign.group
        shared.value = shared.value.map(g => ({ ...g, project_ids: [...g.project_ids.filter(id => !members.has(id)), ...(g.id === target ? [...members] : [])] }))
        const out = await assignProjectGroup(plan.assign.group, plan.assign.projects)
        eventId = out.event_id
      }
      for (const id of plan.archive) { await projects.setState(id, ARCHIVED); archived.push(id) }
      for (const id of plan.restore) { await projects.setState(id, 'active'); restored.push(id) }
    } catch (error) {
      save(withPlacements(prefs.value, inverse))
      shared.value = groupsBefore
      if (eventId != null) await undoGroupEvent(eventId).catch(() => undefined)
      for (const id of [...archived, ...restored]) await projects.setState(id, states.get(id) ?? 'active').catch(() => undefined)
      throw error
    }
    const undo: Undo = async () => {
      save(withPlacements(prefs.value, inverse))
      if (eventId != null) { shared.value = groupsBefore; await undoGroupEvent(eventId) }
      for (const id of [...plan.archive, ...plan.restore]) await projects.setState(id, states.get(id) ?? 'active')
      if (eventId != null) await load(true)
    }
    return { moved: plan.moved, undo }
  }

  return {
    prefs, defs, shared, hidden, collapsed, admin, ready, sharedAvailable, known,
    where, sharedOf, def, load, create, rename, remove, share, unshare, setVisible, toggleVisible, toggleFold, reorder, step, move,
  }
})

export { ARCHIVED, NO_GROUP }
