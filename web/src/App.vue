<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppHeader from './components/AppHeader.vue'
import ConfirmHost from './components/ConfirmHost.vue'
import ToastHost from './components/ToastHost.vue'
import TooltipHost from './components/TooltipHost.vue'
import VersionDisplay from './components/VersionDisplay.vue'
import ErrorPage from './components/ErrorPage.vue'
import StatusPage from './components/StatusPage.vue'
import ShortcutSheet from './components/work/ShortcutSheet.vue'
import AppIcon from './components/AppIcon.vue'
import { command, consume } from './lib/commands'
import { clearFatal, fatal } from './lib/fatal'
import { useSession } from './stores/session'

const session = useSession()
const route = useRoute()
const router = useRouter()
const main = ref<HTMLElement>()
const shortcuts = ref<InstanceType<typeof ShortcutSheet>>()
const retrying = ref(false)
// Sign-in is bare: it has no header or footer and shows connection problems itself.
const bare = computed(() => !!route.meta.bare && !fatal.value)
watch(command, value => { if (value?.command.name === 'shortcuts') { consume(); shortcuts.value?.open() } })
// A new page clears an earlier page error.
watch(() => route.fullPath, (_path, old) => { if (old !== undefined && fatal.value?.kind !== 'update') clearFatal() })
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
  <div class="app-shell" :class="{ bare }">
    <a class="skip-link" href="#main">Skip to content</a>
    <AppHeader v-if="!bare" />
    <main id="main" ref="main" tabindex="-1">
      <!-- The footer ends the page flow; it never floats over content. -->
      <div class="page-flow" :class="{ fill: route.meta.fill && !session.error && !fatal }">
        <ErrorPage v-if="fatal" :error="fatal" />
        <StatusPage v-else-if="session.error && !bare" eyebrow="Connection interrupted" title="Let’s try that again." tone="problem">
          <p role="alert">{{ session.error }}</p>
          <template #actions>
            <button class="btn primary" type="button" :disabled="retrying" @click="retry"><AppIcon name="refresh" :size="14" />{{ retrying ? 'Connecting…' : 'Try again' }}</button>
          </template>
        </StatusPage>
        <RouterView v-else />
        <footer v-if="!bare && (!route.meta.fill || session.error || fatal)" class="app-footer">
          <span class="footer-name">PAIMOS AEON</span>
          <VersionDisplay />
        </footer>
      </div>
    </main>
    <ToastHost />
    <ConfirmHost />
    <ShortcutSheet ref="shortcuts" />
    <TooltipHost />
  </div>
</template>

<style scoped>
.app-shell { height: 100%; display: grid; grid-template-rows: var(--header-h) minmax(0, 1fr); }
.app-shell.bare { grid-template-rows: minmax(0, 1fr); }
/* The gutter is reserved so a scrollbar appearing as content loads never shifts the page sideways. */
main { position: relative; min-height: 0; overflow: auto; scrollbar-gutter: stable; outline: none; scroll-padding-top: 96px; }
main:focus-visible { box-shadow: none; }
.page-flow { display: flex; flex-direction: column; min-height: 100%; }
.page-flow > :first-child { flex: 1 0 auto; }
/* While a page still shows its loading skeleton the footer waits, so it never jumps down as content arrives. */
.page-flow:has(.head-skeleton, .skeleton-body) > .app-footer { visibility: hidden; }
.page-flow.fill { height: 100%; }
.page-flow.fill > :first-child { flex: 1 1 auto; min-height: 0; }
/* The vendored version renderer nudges separators with transforms; clip them to the bar. */
.app-footer {
  display: flex; flex-shrink: 0; justify-content: space-between; align-items: center; gap: 12px; height: 44px; padding: 0 28px; overflow: clip;
  box-shadow: inset 0 1px 0 var(--line); color: var(--ink-2);
}
.footer-name { font: 600 10.5px/1.5 var(--mono); letter-spacing: .22em; color: var(--ink-2); }
.skip-link { position: fixed; z-index: 90; top: 8px; left: 16px; padding: 10px 16px; border-radius: 999px; background: var(--surface-raised); box-shadow: var(--shadow-pop); transform: translateY(-160%); }
.skip-link:focus { transform: translateY(0); }
@media (max-width: 600px) { .app-footer { padding: 0 16px; } .footer-name { font-size: 9.5px; letter-spacing: .18em; } }
</style>
