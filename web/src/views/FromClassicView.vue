<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../lib/api'
import { toast } from '../lib/toast'

const route = useRoute()
const router = useRouter()
const fallback = 'That old link has moved. We brought you to the closest available page.'

watch(() => route.fullPath, async fullPath => {
  const classicPath = fullPath.split('#', 1)[0].slice('/from-classic'.length) || '/'
  try {
    const response = await api(`/from-classic?path=${encodeURIComponent(classicPath)}`)
    if (response.status === 401) {
      sessionStorage.setItem('aeon.fromClassicReturn', fullPath)
      await router.replace('/signin')
      return
    }
    if (!response.ok) throw new Error('resolution failed')
    const resolved: { path?: unknown; notice?: unknown } = await response.json()
    const target = typeof resolved.path === 'string' && resolved.path.startsWith('/') && !resolved.path.startsWith('//')
      && !resolved.path.startsWith('/from-classic') ? resolved.path : '/'
    await router.replace(target)
    if (typeof resolved.notice === 'string' && resolved.notice) toast(resolved.notice)
    else if (target === '/' && resolved.path !== '/') toast(fallback)
  } catch {
    await router.replace('/')
    toast(fallback)
  }
}, { immediate: true })
</script>

<template>
  <main class="from-classic" role="status" aria-live="polite">Finding your page…</main>
</template>

<style scoped>
.from-classic { display: grid; place-items: center; min-height: 40vh; color: var(--ink-3); }
</style>
