// SPDX-License-Identifier: AGPL-3.0-only
// Vite-served browser harness: the same Vue runtime instance mounts the isolated editor.
import { createApp, h, ref } from 'vue'
import QuoteDocument from '../src/components/quotes/editor/QuoteDocument.vue'
import QuoteText from '../src/components/quotes/editor/QuoteText.vue'
import type { QuoteDocumentData } from '../src/lib/quotes/types'
export function mountQuoteEditor(document: QuoteDocumentData) {
  const host = globalThis.document.createElement('div')
  host.id = 'quote-editor-test-root'
  globalThis.document.querySelector('#app')?.setAttribute('hidden', '')
  globalThis.document.body.appendChild(host)
  const model = ref(document)
  ;(window as unknown as { quoteModel: typeof model }).quoteModel = model
  createApp({ render: () => h(QuoteDocument, { document: model.value, editable: true, offerNo: 'Q-1', 'onUpdate:document': (value: QuoteDocumentData) => { model.value = value } }) }).mount(host)
}
export function mountWrappedText() {
  const host = document.createElement('div')
  host.id = 'wrapped-test'
  host.style.width = '115px'
  host.style.font = '16px sans-serif'
  document.body.appendChild(host)
  const value = ref('alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima')
  createApp({ render: () => h(QuoteText, { modelValue: value.value, editable: true, label: 'Wrapped test', tag: 'div', 'onUpdate:modelValue': (text: string) => { value.value = text } }) }).mount(host)
}
