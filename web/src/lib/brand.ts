// SPDX-License-Identifier: AGPL-3.0-only
import { computed, ref, watch } from 'vue'

// The product's names (brand.json, schema inspr.brand.v1), from GET /api/version.
// Every visible surface reads them here; nothing else in the web app spells the
// product's name. `generation` is a label ("PAIMOS 7"), never a version.
export interface Brand { schema: string; product: string; generation: string; release_name: string; wordmark: string; short_name: string }

// Only until the server answers (it mirrors the repository's brand.json).
export const FALLBACK_BRAND: Brand = {
  schema: 'inspr.brand.v1', product: 'PAIMOS', generation: '7', release_name: 'AEON', wordmark: 'PAIMOS AEON', short_name: 'AEON',
}

export function validBrand(value: unknown): value is Brand {
  if (!value || typeof value !== 'object') return false
  const b = value as Record<string, unknown>
  return b.schema === 'inspr.brand.v1' && ['product', 'generation', 'release_name', 'wordmark', 'short_name'].every(key => typeof b[key] === 'string' && (b[key] as string).trim() !== '')
}

const current = ref<Brand>(FALLBACK_BRAND)
export const brand = computed(() => current.value)
export function setBrand(value: unknown) { if (validBrand(value)) current.value = value }
// "PAIMOS 7": the generation as a label.
export const generationLabel = computed(() => `${current.value.product} ${current.value.generation}`)

// Page titles end with the wordmark; they follow the brand once it arrives.
// A sheet over the page (the release history) names the tab while it is open.
const page = ref('')
const overlay = ref('')
export function setPageTitle(title: string) { page.value = title }
// The page's own name (a customer's, say), for the breadcrumb.
export const pageName = computed(() => page.value)
export function setOverlayTitle(title: string) { overlay.value = title }
watch([page, overlay, current], ([title, over, b]) => { const t = over || title; document.title = t ? `${t} · ${b.wordmark}` : b.wordmark }, { immediate: typeof document !== 'undefined' })
