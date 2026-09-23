<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppHeader from './components/AppHeader.vue'
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
watch(() => route.path, async () => { await nextTick(); main.value?.focus() })
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
  </div>
</template>

<style scoped>
.app-shell { height: 100%; display: grid; grid-template-rows: 64px minmax(0, 1fr) 52px; }
main { min-height: 0; overflow: auto; outline: none; }
.app-footer { display: flex; justify-content: space-between; align-items: center; padding: 0 28px; gap: 12px; border-top: 1px solid var(--line); }
.footer-name { font: 10px/1.5 var(--mono); letter-spacing: .12em; color: var(--ink-2); }
.connection-error { max-width: 420px; text-align: center; }
.connection-error h1 { margin: 14px 0; }
.connection-error .button { margin-top: 24px; }
.skip-link { position: fixed; z-index: 10; top: 8px; left: 16px; padding: 12px 18px; border-radius: var(--radius-s); background: var(--surface); transform: translateY(-160%); }
.skip-link:focus { transform: translateY(0); }
@media (max-width: 600px) { .app-footer { padding: 0 20px; } .footer-name { font-size: 9px; } }
</style>
