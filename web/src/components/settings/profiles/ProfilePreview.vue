<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { previewSnapshot } from '../../../lib/quotes/profile'
import { SAMPLE_OFFER_NO, sampleDocument } from '../../../lib/quotes/sampleDocument'
import { clone, type Locale } from '../../../lib/quotes/profileForm'
import type { QuoteDocumentData, QuoteLayout, QuoteProfileDefinition, QuoteSender } from '../../../lib/quotes/types'
import type { PaginationResult } from '../../../lib/quotes/layout'
import AppIcon from '../../AppIcon.vue'
import QuoteDocument from '../../quotes/editor/QuoteDocument.vue'

// The live A4 preview beside the profile's form: the real QuoteDocument (the same
// component that prints the PDF) laying out an invented quote for Beispiel Stahl
// GmbH with the profile as it stands, saved or not. It follows edits after a
// short pause, fits the pane's width or a whole page, and can show the margins.
const props = defineProps<{ definition: QuoteProfileDefinition; profileKey: string; sender?: QuoteSender | null; layout?: QuoteLayout | null; focus?: 'cover' | 'table' | 'end' }>()
const desk = ref<HTMLElement>()
const fit = ref<'width' | 'page'>('width')
const guides = ref(false)
const size = ref({ width: 800, height: 900 })
let sizer: ResizeObserver | undefined
watch(desk, el => { sizer?.disconnect(); if (el) { size.value = { width: el.clientWidth, height: el.clientHeight }; sizer = new ResizeObserver(([entry]) => { size.value = { width: entry!.contentRect.width, height: entry!.contentRect.height } }); sizer.observe(el) } })
onBeforeUnmount(() => sizer?.disconnect())
const PAGE_W = 793.7, PAGE_H = 1122.5
const scale = computed(() => {
  const byWidth = (size.value.width - 40) / PAGE_W
  const value = fit.value === 'page' ? Math.min(byWidth, (size.value.height - 40) / PAGE_H) : byWidth
  return Math.max(0.2, Math.min(1.25, value))
})

// Edits settle for a moment before the paper lays itself out again.
const settled = ref<QuoteProfileDefinition>(clone(props.definition))
let timer: ReturnType<typeof setTimeout> | undefined
watch(() => JSON.stringify(props.definition), () => {
  clearTimeout(timer)
  timer = setTimeout(() => { settled.value = clone(props.definition) }, 140)
})
watch(() => props.profileKey, () => { clearTimeout(timer); settled.value = clone(props.definition) })
onBeforeUnmount(() => clearTimeout(timer))
const document = computed<QuoteDocumentData>(() => ({
  ...sampleDocument(settled.value.locale as Locale, props.sender, props.layout),
  profile: previewSnapshot(settled.value, props.profileKey),
}))
const state = ref<PaginationResult | null>(null)
const pages = computed(() => state.value?.pages.length ?? 0)
const classic = computed(() => settled.value.layout_variant === 'classic-v1')
// Following the form: the cover, the positions table, or the totals, signatures and footer.
function follow() {
  const root = desk.value
  if (!root || !state.value?.ready) return
  const target = props.focus === 'table' ? root.querySelector<HTMLElement>('.quote-page .quote-positions')
    : props.focus === 'end' ? root.querySelector<HTMLElement>('.quote-page:last-of-type .quote-page-footer')
    : null
  const top = target ? target.getBoundingClientRect().top - root.getBoundingClientRect().top + root.scrollTop - (props.focus === 'end' ? root.clientHeight * 0.55 : 24) : 0
  root.scrollTo({ top: Math.max(0, top), behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
}
watch(() => props.focus, follow)
watch(() => state.value?.ready, ready => { if (ready && props.focus !== 'cover') follow() })
</script>

<template>
  <section class="profile-preview" aria-label="Preview">
    <header class="preview-bar">
      <div class="preview-title">
        <h2>Preview</h2>
        <p>Sample quote for Beispiel Stahl GmbH<span v-if="pages"> · {{ pages }} {{ pages === 1 ? 'page' : 'pages' }}</span></p>
      </div>
      <div class="preview-tools">
        <button v-if="classic" type="button" class="btn sm ghost" :aria-pressed="guides" data-tip="Show the page margins" @click="guides = !guides"><AppIcon name="layers" :size="14" />Margins</button>
        <div class="seg" role="group" aria-label="Fit">
          <button type="button" :aria-pressed="fit === 'width'" @click="fit = 'width'">Width</button>
          <button type="button" :aria-pressed="fit === 'page'" @click="fit = 'page'">Page</button>
        </div>
      </div>
    </header>
    <div ref="desk" class="preview-desk" tabindex="0" role="group" aria-label="Sample pages, scroll to read them">
      <div class="preview-inner" :class="{ guides: guides && classic }">
        <!-- A visual aid: the form says everything in words, so the sample stays out of the page's outline. -->
        <div class="preview-stack" :style="{ zoom: scale }" inert aria-hidden="true">
          <QuoteDocument :document="document" :offer-no="SAMPLE_OFFER_NO" @render-state="value => state = value" />
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
/* Its width comes from the layout, never from the paper inside (contain), so a
   wide page never pushes the editor past the window. */
.profile-preview { display: flex; flex-direction: column; min-width: 0; min-height: 0; contain: inline-size; background: var(--surface-sunken); }
.preview-bar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 12px; min-height: 52px; padding: 8px 16px; border-bottom: 1px solid var(--line-2); background: var(--surface-raised-2); }
.preview-title { min-width: 0; }
.preview-title h2 { font: 600 14px/1.3 var(--font); color: var(--ink); }
.preview-title p { margin-top: 1px; font-size: 12.5px; color: var(--ink-2); }
.preview-tools { display: flex; align-items: center; gap: 8px; }
.preview-desk { flex: 1; min-height: 0; overflow: auto; overscroll-behavior: contain; outline: none; }
.preview-desk:focus-visible { box-shadow: inset var(--focus-ring); }
.preview-inner { padding: 20px; }
.preview-stack { width: 210mm; margin: 0 auto; }
/* The margins as a hairline frame on each page (the classic layout sets them). */
.guides :deep(.quote-page)::after { content: ''; position: absolute; inset: var(--quote-top) var(--quote-right) var(--quote-bottom) var(--quote-left); outline: 1px dashed rgba(40, 127, 120, .55); pointer-events: none; }
@media (max-width: 720px) { .preview-inner { padding: 12px; } }
</style>
