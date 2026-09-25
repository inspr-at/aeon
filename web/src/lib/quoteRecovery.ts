// SPDX-License-Identifier: AGPL-3.0-only
// Local, same-account recovery for unsaved quote content. No credentials or
// public capabilities are stored. Call clearAccountRecovery on secure sign-out.
import type { QuoteDocumentData } from './quotes/types'

export interface RecoveryDraft {
  tenantId: string; principalId: string; quoteId: string; sessionId: string
  baseRevision: number; base: QuoteDocumentData; mine: QuoteDocumentData
  // The version the draft was branched from (0 for a quote's first draft): a copy
  // from another version's draft never belongs to this one.
  baseVersion?: number
  pendingMutationId?: string; savedAt: number; schemaVersion: 1
}
// A copy is worth offering only when it holds work the saved draft does not.
export const holdsWork = (r: Pick<RecoveryDraft, 'base' | 'mine'>) => JSON.stringify(r.base) !== JSON.stringify(r.mine)
const database = 'aeon-quote-recovery-v1'
const storeName = 'drafts'
const retentionMs = 30*24*60*60*1000
const key = (r: Pick<RecoveryDraft,'tenantId'|'principalId'|'quoteId'|'sessionId'>) => [r.tenantId,r.principalId,r.quoteId,r.sessionId].join(':')
function open(): Promise<IDBDatabase> {
  return new Promise((resolve,reject) => {
    const req = indexedDB.open(database,1)
    req.onupgradeneeded = () => { if (!req.result.objectStoreNames.contains(storeName)) req.result.createObjectStore(storeName) }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}
async function operate<T>(mode: IDBTransactionMode, action: (s: IDBObjectStore,done:(v:T)=>void,fail:(e:unknown)=>void)=>void): Promise<T> {
  const db = await open()
  try { return await new Promise<T>((resolve,reject) => {
    const tx=db.transaction(storeName,mode); const s=tx.objectStore(storeName)
    let value:T
    tx.oncomplete=()=>resolve(value)
    tx.onerror=()=>reject(tx.error)
    tx.onabort=()=>reject(tx.error)
    action(s,v=>{value=v},reject)
  }) } finally { db.close() }
}
export async function saveRecovery(r: RecoveryDraft): Promise<boolean> {
  try { await operate<void>('readwrite',(s,done,fail)=>{const req=s.put({...r,savedAt:Date.now(),schemaVersion:1},key(r));req.onsuccess=()=>done();req.onerror=()=>fail(req.error) }); return true } catch { return false }
}
export async function loadRecovery(scope: Pick<RecoveryDraft,'tenantId'|'principalId'|'quoteId'|'sessionId'>): Promise<RecoveryDraft|null> {
  try { const value=await operate<RecoveryDraft|undefined>('readonly',(s,done,fail)=>{const req=s.get(key(scope));req.onsuccess=()=>done(req.result as RecoveryDraft|undefined);req.onerror=()=>fail(req.error)})
    if (!value || value.tenantId!==scope.tenantId || value.principalId!==scope.principalId || value.quoteId!==scope.quoteId || value.sessionId!==scope.sessionId || value.schemaVersion!==1) return null
    if (Date.now()-value.savedAt>retentionMs) { await clearRecovery(scope); return null }
    return value
  } catch { return null }
}
export async function clearRecovery(scope: Pick<RecoveryDraft,'tenantId'|'principalId'|'quoteId'|'sessionId'>): Promise<void> {
  await operate<void>('readwrite',(s,done,fail)=>{const req=s.delete(key(scope));req.onsuccess=()=>done();req.onerror=()=>fail(req.error)})
}
export async function clearAccountRecovery(tenantId:string,principalId:string): Promise<void> {
  await operate<void>('readwrite',(s,done,fail)=>{const req=s.openCursor();req.onsuccess=()=>{const cursor=req.result;if(!cursor){done();return} if(String(cursor.key).startsWith(`${tenantId}:${principalId}:`)) cursor.delete();cursor.continue()};req.onerror=()=>fail(req.error)})
}
export function downloadRecovery(r: RecoveryDraft): void {
  const blob=new Blob([JSON.stringify(r,null,2)],{type:'application/json'})
  const url=URL.createObjectURL(blob); const link=document.createElement('a'); link.href=url; link.download=`quote-recovery-${r.quoteId}.json`; link.click(); setTimeout(()=>URL.revokeObjectURL(url),1000)
}
