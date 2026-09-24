// SPDX-License-Identifier: AGPL-3.0-only
// Explicit three-way preview. Stable P3 IDs are the only array merge anchors.
import type { QuoteDocumentData } from './quotes/types'

export interface MergeConflict { path: string; reason: string; mine: unknown; theirs: unknown }
export interface MergePreview { document: QuoteDocumentData | null; conflicts: MergeConflict[]; changed: boolean }
export type ConflictChoices = Record<string, 'mine' | 'theirs'>
const absent = Symbol('absent')
type Value = unknown | typeof absent
const equal = (a: Value, b: Value) => a === absent || b === absent ? a === b : JSON.stringify(a) === JSON.stringify(b)
const object = (v: Value): v is Record<string, unknown> => v !== absent && !!v && typeof v === 'object' && !Array.isArray(v)
const identified = (v: Value): v is Array<{ id: string }> => Array.isArray(v) && v.every(x => x && typeof x === 'object' && typeof x.id === 'string')
const copy = <T>(v: T): T => structuredClone(v)

export function mergeQuote(base: QuoteDocumentData, mine: QuoteDocumentData, theirs: QuoteDocumentData, choices: ConflictChoices = {}): MergePreview {
  const conflicts: MergeConflict[] = []
  if ([base, mine, theirs].some(d => d.schema_version !== 1 || d.minimum_writer_version > 2)) {
    return { document: null, conflicts: [{ path: '$', reason: 'incompatible schema', mine, theirs }], changed: false }
  }
  function resolve(path: string, reason: string, m: Value, t: Value): Value {
    conflicts.push({ path, reason, mine: m === absent ? undefined : copy(m), theirs: t === absent ? undefined : copy(t) })
    return choices[path] === 'theirs' ? t : m
  }
  function walk(path: string, b: Value, m: Value, t: Value): Value {
    if (equal(m,t)) return m
    if (equal(m,b)) return t
    if (equal(t,b)) return m
    if (b === absent || m === absent || t === absent) return resolve(path, 'deletion versus edit or competing addition', m, t)
    if (identified(b) && identified(m) && identified(t)) {
      const bm = new Map(b.map(x => [x.id,x])); const mm = new Map(m.map(x => [x.id,x])); const tm = new Map(t.map(x => [x.id,x]))
      if ([b,m,t].some(items=>new Set(items.map(x=>x.id)).size!==items.length)) return resolve(path, 'duplicate anchor',m,t)
      const baseOrder = b.map(x=>x.id).filter(id=>mm.has(id) && tm.has(id)).join('|')
      const mineOrder = m.map(x=>x.id).filter(id=>bm.has(id) && tm.has(id)).join('|')
      const theirOrder = t.map(x=>x.id).filter(id=>bm.has(id) && mm.has(id)).join('|')
      let order: string[]
      if (mineOrder !== baseOrder && theirOrder !== baseOrder && mineOrder !== theirOrder) {
        const chosen = resolve(`${path}.order`,'competing reorder',m.map(x=>x.id),t.map(x=>x.id))
        order = chosen === absent ? [] : chosen as string[]
      } else order = (mineOrder !== baseOrder ? m : t).map(x=>x.id)
      for (const id of [...mm.keys(),...tm.keys()]) if (!order.includes(id)) order.push(id)
      return order.map(id => walk(`${path}[${id}]`,bm.get(id) ?? absent,mm.get(id) ?? absent,tm.get(id) ?? absent)).filter(x=>x!==absent)
    }
    if (object(b) && object(m) && object(t)) {
      const out: Record<string,unknown> = {}
      for (const key of new Set([...Object.keys(b),...Object.keys(m),...Object.keys(t)])) {
        const v = walk(`${path}.${key}`,key in b ? b[key] : absent,key in m ? m[key] : absent,key in t ? t[key] : absent)
        if (v !== absent) out[key]=v
      }
      return out
    }
    return resolve(path,'same field changed',m,t)
  }
  const result = walk('$',base,mine,theirs) as QuoteDocumentData
  if (conflicts.some(c => !choices[c.path])) return { document: result, conflicts, changed: !equal(result,theirs) }
  const ids = result.sections.flatMap(s => [s.id,...s.nodes.map(n=>n.id)]).concat(result.positions.map(p=>p.id))
  if (new Set(ids).size !== ids.length || result.sections.length>20 || result.positions.length>100 || result.sections.some(s=>s.nodes.length>100)) {
    return { document: null, conflicts: [{ path: '$', reason: 'invalid or duplicate structural anchors', mine, theirs }], changed: false }
  }
  let total = 0n
  for (const position of result.positions) {
    if (!/^[0-9]+(?:\.[0-9]{1,2})?$/.test(position.quantity) || !Number.isSafeInteger(position.unit_price_cents) || position.unit_price_cents<0) {
      return { document: null, conflicts: [{ path: `$.positions[${position.id}]`, reason: 'invalid exact money', mine: position, theirs: position }], changed: false }
    }
    const [whole, fraction=''] = position.quantity.split('.')
    const hundredths = BigInt(whole)*100n+BigInt(fraction.padEnd(2,'0'))
    const cents = (hundredths*BigInt(position.unit_price_cents)+50n)/100n
    if (cents>BigInt(Number.MAX_SAFE_INTEGER)) return { document:null, conflicts:[{path:`$.positions[${position.id}]`,reason:'money exceeds safe range',mine:position,theirs:position}],changed:false }
    position.total_cents=Number(cents); total+=cents
  }
  if (total>BigInt(Number.MAX_SAFE_INTEGER)) return {document:null,conflicts:[{path:'$.net_total_cents',reason:'money exceeds safe range',mine:total.toString(),theirs:total.toString()}],changed:false}
  result.net_total_cents=Number(total)
  return { document:result, conflicts, changed:!equal(result,theirs) }
}
