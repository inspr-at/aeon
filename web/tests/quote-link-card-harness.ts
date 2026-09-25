// SPDX-License-Identifier: AGPL-3.0-only
import { createApp, h } from 'vue'
import QuoteLinkCard from '../src/components/quotes/details/QuoteLinkCard.vue'

export function mountQuoteLinkCard() {
  document.querySelector('#app')?.setAttribute('hidden', '')
  const host = document.createElement('div')
  document.body.append(host)
  createApp({ render: () => h(QuoteLinkCard, { quoteId: '11111111-1111-4111-8111-111111111111', version: 1, admin: true, acceptable: true, accepted: false }) }).mount(host)
}
