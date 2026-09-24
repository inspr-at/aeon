// SPDX-License-Identifier: AGPL-3.0-only
import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { APIError } from './api'
import { byPosition, deleteAttachment, findRemovalEvent, listAttachments, patchAttachment, positionBetween, positionOf, undoEvent, uploadAttachment, type Attachment } from './attachments'
import { dismiss, toast } from './toast'

// One ticket's attachments. Uploads show at once with their progress (images with
// a local preview); a delete reaches the server at once and its toast offers Undo
// for six seconds, which undoes the removal event on the server. Reorder and
// captions use the attachment's precondition.
export interface Upload { key: string; name: string; size: number; type: string; progress: number; preview: string | null; error: string; file: File; controller: AbortController }
const UNDO_MS = 6000

export function useAttachments(nodeId: Ref<string | null>) {
  const items = ref<Attachment[]>([])
  const uploads = ref<Upload[]>([])
  const loading = ref(false)
  const error = ref('')
  // Deletes still on their way, so an early Undo waits for them.
  const deleting = new Map<string, Promise<boolean>>()
  let generation = 0

  async function load() {
    const id = nodeId.value
    const current = ++generation
    if (!id) { items.value = []; return }
    loading.value = true; error.value = ''
    try {
      const list = await listAttachments(id)
      if (current === generation) items.value = list.filter(item => !deleting.has(item.id))
    } catch (e) {
      if (current !== generation) return
      // A server without the attachments surface simply shows none.
      if (e instanceof APIError && (e.status === 404 || e.status === 405)) items.value = []
      else error.value = e instanceof Error ? e.message : 'Attachments could not be loaded'
    } finally { if (current === generation) loading.value = false }
  }
  watch(nodeId, () => { uploads.value = []; void load() }, { immediate: true })

  // Resolves with the new attachment's id, or null when the upload failed.
  function upload(file: File): Promise<Attachment | null> {
    const id = nodeId.value
    if (!id) return Promise.resolve(null)
    const entry: Upload = {
      key: `u${Date.now()}-${Math.random().toString(36).slice(2, 7)}`, name: file.name || 'Screenshot.png', size: file.size, type: file.type, progress: 0,
      preview: file.type.startsWith('image/') ? URL.createObjectURL(file) : null, error: '', file, controller: new AbortController(),
    }
    uploads.value = [...uploads.value, entry]
    const update = (patch: Partial<Upload>) => { uploads.value = uploads.value.map(u => u.key === entry.key ? { ...u, ...patch } : u) }
    return uploadAttachment(id, file, { onProgress: progress => update({ progress }), signal: entry.controller.signal })
      .then(created => {
        if (nodeId.value === id) items.value = [...items.value.filter(item => item.id !== created.id), created].sort(byPosition)
        finish(entry.key)
        return created
      })
      .catch(e => {
        if (e instanceof DOMException && e.name === 'AbortError') { finish(entry.key); return null }
        update({ error: e instanceof Error ? e.message : 'Upload failed' })
        return null
      })
  }
  function finish(key: string) {
    const entry = uploads.value.find(u => u.key === key)
    if (entry?.preview) URL.revokeObjectURL(entry.preview)
    uploads.value = uploads.value.filter(u => u.key !== key)
  }
  function retry(key: string) {
    const entry = uploads.value.find(u => u.key === key)
    if (!entry) return
    finish(key)
    void upload(entry.file)
  }
  function cancel(key: string) { uploads.value.find(u => u.key === key)?.controller.abort(); finish(key) }
  async function add(files: File[]) { return Promise.all(files.map(upload)) }

  async function setCaption(item: Attachment, caption: string) {
    if (caption === item.caption) return true
    const before = item.caption
    items.value = items.value.map(x => x.id === item.id ? { ...x, caption } : x)
    try {
      const saved = await patchAttachment(item, { caption })
      items.value = items.value.map(x => x.id === item.id ? { ...x, ...saved } : x)
      return true
    } catch (e) {
      items.value = items.value.map(x => x.id === item.id ? { ...x, caption: before } : x)
      toast(e instanceof APIError && e.status === 412 ? `${item.name} was changed elsewhere; the newer caption is shown.` : `The caption was not saved: ${e instanceof Error ? e.message : 'unknown error'}`, { tone: 'error' })
      void load()
      return false
    }
  }
  // Move `id` to index `to` among the current items.
  async function reorder(id: string, to: number) {
    const from = items.value.findIndex(x => x.id === id)
    if (from === -1 || from === to) return
    const list = items.value.filter(x => x.id !== id)
    const item = items.value[from]
    const target = Math.max(0, Math.min(list.length, to))
    const before = list[target - 1], after = list[target]
    const position = positionBetween(before ? positionOf(before) : undefined, after ? positionOf(after) : undefined)
    list.splice(target, 0, { ...item, position })
    items.value = list
    try {
      const saved = await patchAttachment(item, { position })
      items.value = items.value.map(x => x.id === id ? { ...x, ...saved } : x)
    } catch (e) {
      toast(e instanceof APIError && e.status === 412 ? 'The attachments changed elsewhere; the current order is shown.' : 'The new order was not saved.', { tone: 'error' })
      void load()
    }
  }

  function remove(item: Attachment) {
    const node = nodeId.value
    items.value = items.value.filter(x => x.id !== item.id)
    const done = deleteAttachment(item.id).then(() => true, e => {
      if (e instanceof APIError && e.status === 404) return true
      toast(`${item.name} could not be deleted.`, { tone: 'error' })
      if (nodeId.value === node) void load()
      return false
    }).finally(() => deleting.delete(item.id))
    deleting.set(item.id, done)
    const toastId = toast(`Deleted ${item.name}`, { timeout: UNDO_MS, action: { label: 'Undo', run: () => void undo(item, node, done, toastId) } })
  }
  // Undo puts the attachment back at once and undoes the removal on the server.
  async function undo(item: Attachment, node: string | null, done: Promise<boolean>, toastId?: number) {
    if (toastId !== undefined) dismiss(toastId)
    const here = () => nodeId.value === node
    if (here()) items.value = [...items.value.filter(x => x.id !== item.id), item].sort(byPosition)
    try {
      if (!(await done)) return
      const event = node ? await findRemovalEvent(node, item.id) : null
      if (event === null) throw new Error('no removal to undo')
      const restored = await undoEvent(event)
      if (here() && restored) items.value = items.value.map(x => x.id === item.id ? { ...x, ...restored } : x)
    } catch {
      toast(`${item.name} could not be restored.`, { tone: 'error' })
      if (here()) void load()
    }
  }
  onBeforeUnmount(() => { for (const u of uploads.value) if (u.preview) URL.revokeObjectURL(u.preview) })

  const count = computed(() => items.value.length + uploads.value.length)
  const images = computed(() => items.value.filter(item => item.content_type.startsWith('image/')))
  return { items, uploads, loading, error, count, images, load, add, upload, retry, cancel, setCaption, reorder, remove }
}
