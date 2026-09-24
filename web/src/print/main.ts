// SPDX-License-Identifier: AGPL-3.0-only
// Dedicated offline entry. It mounts the P3 component without app/router/auth.
import { createApp, h, nextTick } from 'vue'
import QuoteDocument from '../components/quotes/editor/QuoteDocument.vue'
import type { QuoteDocumentData } from '../lib/quotes/types'
import '../styles/tokens.css'
import '../styles/base.css'
import './print.css'

interface Payload {
  document: QuoteDocumentData
  offer_no: string
  public_url?: string
  accepted?: { name: string; company?: string; at: string; digest: string }
}

async function render() {
  const response = await fetch('/payload', { credentials: 'omit', cache: 'no-store' })
  if (!response.ok) throw new Error('Print document unavailable')
  const payload = await response.json() as Payload
  if (payload.document.schema_version !== 1) throw new Error('Unsupported document')
  let complete = false
  const app = createApp({ render: () => h(QuoteDocument, {
    document: payload.document, offerNo: payload.offer_no, editable: false, publicLink: payload.public_url ?? '',
    accepted: payload.accepted ?? null,
    onRenderState: (state: { ready: boolean; overflow: string | null }) => {
      if (complete) return
      if (state.overflow) document.documentElement.dataset.pdfOverflow = state.overflow
      else if (state.ready) {
        complete = true
        void finish()
      }
    },
  }) })
  app.mount('#app')
}

async function finish() {
  await document.fonts.ready
  await nextTick()
  await new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
  if (!document.querySelector('.quote-page')) throw new Error('No printed pages')
  document.documentElement.dataset.pdfReady = 'true'
}

void render().catch(() => { document.documentElement.dataset.pdfOverflow = 'Print rendering failed' })
