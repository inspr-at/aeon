<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import mark from '../assets/brand/aeon-mark.svg'
import { useSession } from '../stores/session'
import { dark, toggleTheme } from '../lib/theme'
import AppIcon from './AppIcon.vue'

const session = useSession()
const router = useRouter()
const open = ref(false)
const busy = ref(false)
const error = ref('')
const account = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
const signOutButton = ref<HTMLButtonElement>()
async function toggleMenu() {
  open.value = !open.value
  if (open.value) { await nextTick(); signOutButton.value?.focus() }
}
function closeMenu(restoreFocus = false) {
  open.value = false
  if (restoreFocus) trigger.value?.focus()
}
function outside(event: PointerEvent) {
  if (event.target instanceof Node && !account.value?.contains(event.target)) closeMenu()
}
function focusOut(event: FocusEvent) {
  if (event.relatedTarget instanceof Node && !account.value?.contains(event.relatedTarget)) closeMenu()
}
async function signOut() {
  busy.value = true
  error.value = ''
  try {
    await session.signOut()
    closeMenu()
    await router.replace('/signin')
  } catch { error.value = 'Sign out didn’t complete. Please try again.' }
  finally { busy.value = false }
}
onMounted(() => document.addEventListener('pointerdown', outside))
onBeforeUnmount(() => document.removeEventListener('pointerdown', outside))
</script>

<template>
  <header class="app-header">
    <RouterLink class="lockup" to="/" aria-label="PAIMOS AEON home">
      <span class="mark-backing"><img :src="mark" width="34" height="34" alt="" /></span>
      <span class="wordmark">PAIMOS<sup>AEON</sup></span>
    </RouterLink>
    <div class="header-actions">
      <button class="icon-button" type="button" :aria-label="dark ? 'Switch to light theme' : 'Switch to dark theme'" :title="dark ? 'Light theme' : 'Dark theme'" @click="toggleTheme">
        <AppIcon :name="dark ? 'sun' : 'moon'" />
      </button>
      <div v-if="session.identity" ref="account" class="account" @keydown.esc.stop.prevent="closeMenu(true)" @focusout="focusOut">
        <button ref="trigger" class="account-trigger" type="button" :aria-expanded="open" aria-controls="account-panel" :aria-label="`Account for ${session.identity.principal.name}`" @click="toggleMenu">
          <span class="avatar"><AppIcon name="user" /></span>
          <span class="user-name">{{ session.identity.principal.name }}</span>
          <AppIcon name="chevron" />
        </button>
        <div v-if="open" id="account-panel" class="account-panel glass-card">
          <p class="eyebrow">Signed in as</p>
          <p class="account-name">{{ session.identity.principal.name }}</p>
          <p class="account-tenant">{{ session.identity.tenant.name }}</p>
          <button ref="signOutButton" class="sign-out" type="button" :disabled="busy" @click="signOut"><AppIcon name="logout" />{{ busy ? 'Signing out…' : 'Sign out' }}</button>
          <p v-if="error" class="error" role="alert">{{ error }}</p>
        </div>
      </div>
    </div>
  </header>
</template>

<style scoped>
.app-header { height: 64px; padding: 0 28px; display: flex; align-items: center; justify-content: space-between; gap: 16px; background: var(--glass-2); border-bottom: 1px solid var(--glass-edge); box-shadow: 0 1px 0 var(--line); backdrop-filter: blur(16px) saturate(1.2); z-index: 5; }
.lockup { display: inline-flex; align-items: center; gap: 11px; min-height: 44px; color: var(--ink); flex-shrink: 0; }
.mark-backing { display: grid; place-items: center; height: 40px; width: 40px; background: #f7f6f2; border-radius: 10px; }
.mark-backing img { display: block; }
.wordmark { font: 600 14px/1 var(--mono); letter-spacing: .19em; white-space: nowrap; }
.wordmark sup { position: relative; top: -.1em; margin-left: 7px; font: 500 8px/1 var(--mono); letter-spacing: .12em; color: var(--teal-ink); }
.header-actions { display: flex; align-items: center; gap: 14px; }
.account { position: relative; }
.account-trigger { display: flex; align-items: center; gap: 9px; height: 44px; padding: 0 8px 0 0; background: transparent; border: 1px solid transparent; border-radius: 24px; }
.account-trigger:hover { background: var(--glass); }
.avatar { display: grid; place-items: center; width: 38px; height: 38px; border: 1px solid var(--glass-rim); border-radius: 50%; background: linear-gradient(145deg, var(--surface), var(--aqua-2)); color: var(--teal-ink); }
.user-name { font-size: 13px; max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.account-panel { position: absolute; right: 0; top: 52px; width: min(290px, calc(100vw - 32px)); padding: 20px; background: var(--surface); }
.account-name { margin-top: 8px; color: var(--ink); font-weight: 600; overflow-wrap: anywhere; }
.account-tenant { font-size: 13px; overflow-wrap: anywhere; }
.sign-out { display: flex; gap: 12px; align-items: center; width: 100%; margin-top: 16px; min-height: 44px; border: 1px solid var(--line); border-radius: var(--radius-s); background: var(--glass); padding: 8px 12px; }
.sign-out:hover { background: var(--aqua-3); }
.error { margin-top: 12px; }
@media (max-width: 600px) { .app-header { padding: 0 16px; gap: 8px; } .header-actions { gap: 8px; } .user-name, .account-trigger > svg { display: none; } .account-trigger { width: 44px; padding: 0; justify-content: center; } .wordmark { font-size: 12px; } .lockup { gap: 8px; } }
</style>
