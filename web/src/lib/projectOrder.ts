// SPDX-License-Identifier: AGPL-3.0-only
// The person's own order of projects, the Custom sort (AEON-174): one list of
// project ids kept in their `projects` preference. A group shows its projects in
// that list's order; projects missing from it (new ones) follow the ordered ones.

/** Compares two projects by their place in `rank`; 0 when neither has one. */
export function byRank(rank: ReadonlyMap<string, number>) {
  const at = (id: string) => rank.get(id) ?? Number.MAX_SAFE_INTEGER
  return (a: { id: string }, b: { id: string }) => at(a.id) - at(b.id)
}

/**
 * Moves `ids` (keeping their relative order) so they sit at `index` among the
 * other `members` of one group, and returns the new full order. `order` holds
 * every project; `members` are the group's projects as shown.
 */
export function place(order: readonly string[], members: readonly string[], ids: readonly string[], index: number): string[] {
  const moving = new Set(ids)
  const others = members.filter(id => !moving.has(id))
  const block = order.filter(id => moving.has(id))
  const rest = order.filter(id => !moving.has(id))
  if (!others.length || !block.length) return [...order]
  const at = Math.max(0, Math.min(others.length, index))
  const anchor = at < others.length ? rest.indexOf(others[at]!) : rest.indexOf(others[others.length - 1]!) + 1
  if (anchor < 0) return [...order]
  return [...rest.slice(0, anchor), ...block, ...rest.slice(anchor)]
}

/** The same order, or not: cheap equality for id lists. */
export function sameOrder(a: readonly string[], b: readonly string[]) {
  return a.length === b.length && a.every((id, i) => id === b[i])
}

/**
 * Where a dragged card lands: the cell of the group's grid nearest the pointer.
 * Cells are layout boxes and do not depend on which card fills them, so the
 * answer is stable while the other cards glide aside.
 */
export function nearestCell(cells: readonly { left: number; top: number; right: number; bottom: number }[], x: number, y: number): number {
  let best = -1, distance = Infinity
  cells.forEach((cell, i) => {
    const dx = x < cell.left ? cell.left - x : x > cell.right ? x - cell.right : 0
    const dy = y < cell.top ? cell.top - y : y > cell.bottom ? y - cell.bottom : 0
    const d = dx * dx + dy * dy
    if (d < distance) { distance = d; best = i }
  })
  return best
}
