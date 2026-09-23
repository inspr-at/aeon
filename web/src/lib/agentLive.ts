// SPDX-License-Identifier: AGPL-3.0-only
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { message } from './agents.ts'

// SSE is a wake hint: always re-read authorized projections. Polling also covers
// newly introduced event names until the coordinator aligns all R2 writers.
export function useAgentLive<T>(read: () => Promise<T>) {
  const data = ref<T>()
  const error = ref('')
  const busy = ref(false)
  const live = ref(false)
  let disposed = false
  let revision = 0
  let stream: EventSource | undefined
  let debounce: ReturnType<typeof setTimeout> | undefined
  let poll: ReturnType<typeof setInterval> | undefined
  async function refresh() {
    if (disposed) return
    const current = ++revision
    busy.value = true
    try {
      const result = await read()
      if (!disposed && current === revision) { data.value = result; error.value = '' }
    } catch (cause) {
      if (!disposed && current === revision) error.value = message(cause)
    } finally { if (!disposed && current === revision) busy.value = false }
  }
  function changed() {
    if (disposed || debounce) return
    debounce = setTimeout(() => { debounce = undefined; void refresh() }, 150)
  }
  onMounted(() => {
    void refresh()
    stream = new EventSource('/api/events/stream')
    stream.onopen = () => { live.value = true; changed() }
    stream.onerror = () => { live.value = false }
    stream.onmessage = changed
    for (const resource of ['run', 'agent_run', 'work_order', 'approval', 'agent_account', 'account', 'allowance', 'allowance_window']) {
      for (const action of ['created', 'updated', 'claimed', 'telemetry', 'started', 'finished', 'proposed', 'approved', 'denied', 'revoked', 'registered', 'probed', 'reserved', 'settled', 'released']) {
        stream.addEventListener(`${resource}.${action}`, changed)
      }
    }
    poll = setInterval(() => void refresh(), 30_000)
  })
  onBeforeUnmount(() => {
    disposed = true
    revision++
    stream?.close()
    clearTimeout(debounce)
    clearInterval(poll)
  })
  return { data, error, busy, live, refresh }
}
