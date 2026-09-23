<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppHeader from './components/AppHeader.vue'
import ToastHost from './components/ToastHost.vue'
import TooltipHost from './components/TooltipHost.vue'
import VersionDisplay from './components/VersionDisplay.vue'
import { useSession } from './stores/session'

const session = useSession()
const route = useRoute()
const router = useRouter()
const main = ref<HTMLElement>()
const retrying = ref(false)
async function retry() {
  retrying.value = true
  await session.refresh()
  if (!session.error) await router.replace(session.identity ? '/' : '/signin')
  retrying.value = false
}
// Move focus to the page on real page changes; opening a ticket panel within a
// project keeps the list's focus and scroll position.
watch(() => [route.path, route.params.projectKey, route.params.ticketKey] as const, async ([path, project], old) => {
  if (old && project && project === old[1]) return
  if (old && path === old[0]) return
  await nextTick()
  main.value?.focus({ preventScroll: true })
})
</script>

<template>
  <div class="app-shell">
    <a class="skip-link" href="#main">Skip to content</a>
    <AppHeader />
    <main id="main" ref="main" tabindex="-1">
      <section v-if="session.error" class="center-stage" aria-labelledby="connection-title">
        <div class="connection-error">
          <p class="eyebrow">Connection interrupted</p>
          <h1 id="connection-title">Let’s try that again.</h1>
          <p role="alert">{{ session.error }}</p>
          <button class="button" :disabled="retrying" @click="retry">{{ retrying ? 'Connecting…' : 'Try again' }}</button>
        </div>
      </section>
      <RouterView v-else />
    </main>
    <footer class="app-footer">
      <span class="footer-name">PAIMOS AEON</span>
      <VersionDisplay />
    </footer>
    <ToastHost />
    <TooltipHost />
  </div>
</template>

<style scoped>
.app-shell { height: 100%; display: grid; grid-template-rows: var(--header-h) minmax(0, 1fr) var(--footer-h); }
main { position: relative; min-height: 0; overflow: auto; outline: none; scroll-padding-top: 96px; }
main:focus-visible { box-shadow: none; }
/* The vendored version renderer nudges separators with transforms; clip them to the rail. */
.app-footer {
  display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 0 22px; overflow: clip;
  box-shadow: inset 0 1px 0 var(--line); background: var(--glass-2); color: var(--ink-2);
  backdrop-filter: blur(16px) saturate(1.2); -webkit-backdrop-filter: blur(16px) saturate(1.2);
}
.footer-name { font: 600 10.5px/1.5 var(--mono); letter-spacing: .22em; color: var(--ink-2); }
.connection-error { max-width: 420px; text-align: center; }
.connection-error h1 { margin: 14px 0; }
.connection-error .button { margin-top: 24px; }
.skip-link { position: fixed; z-index: 90; top: 8px; left: 16px; padding: 10px 16px; border-radius: 999px; background: var(--surface-raised); box-shadow: var(--shadow-pop); transform: translateY(-160%); }
.skip-link:focus { transform: translateY(0); }
@media (max-width: 600px) { .app-footer { padding: 0 14px; } .footer-name { font-size: 9.5px; letter-spacing: .18em; } }
</style>
