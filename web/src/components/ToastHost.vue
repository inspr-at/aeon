<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { dismiss, toasts } from '../lib/toast'
import AppIcon from './AppIcon.vue'
function act(id: number, run: () => void) { dismiss(id); run() }
</script>

<template>
  <div class="toast-host" aria-live="polite">
    <TransitionGroup name="toast">
      <div v-for="item in toasts" :key="item.id" class="toast" :class="item.tone">
        <AppIcon v-if="item.tone === 'error'" name="alert" :size="14" />
        <span>{{ item.message }}</span>
        <button v-if="item.action" class="toast-action" type="button" @click="act(item.id, item.action.run)">{{ item.action.label }}</button>
        <button class="toast-close" type="button" aria-label="Dismiss" @click="dismiss(item.id)"><AppIcon name="close" :size="12" /></button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toast-host { position: fixed; z-index: 60; left: 50%; bottom: calc(var(--footer-h) + 16px); transform: translateX(-50%); display: flex; flex-direction: column; align-items: center; gap: 8px; pointer-events: none; width: max-content; max-width: calc(100vw - 32px); }
.toast {
  display: flex; align-items: center; gap: 10px; min-height: 40px; padding: 6px 8px 6px 18px; border-radius: 999px; pointer-events: auto;
  background: var(--tip-bg); color: var(--tip-ink); font-size: 13.5px; box-shadow: 0 0 0 1px var(--glass-rim), 0 18px 36px -14px rgba(0, 0, 0, .45);
  backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px);
}
.toast.error svg { color: #f3b0a8; }
.toast span { min-width: 0; }
.toast-action { height: 28px; padding: 0 12px; border: 0; border-radius: 999px; background: rgba(164, 229, 223, .18); color: #bff0eb; font-size: 12.5px; font-weight: 700; }
.toast-action:hover { background: rgba(164, 229, 223, .3); }
.toast-close { display: grid; place-items: center; width: 28px; height: 28px; border: 0; border-radius: 50%; background: transparent; color: inherit; opacity: .7; }
.toast-close:hover { opacity: 1; background: rgba(255, 255, 255, .08); }
@media (prefers-reduced-motion: no-preference) {
  .toast-enter-active, .toast-leave-active { transition: opacity .2s ease, transform .2s ease; }
  .toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(8px); }
}
@media (max-width: 600px) { .toast-host { bottom: calc(var(--footer-h) + 12px); } .toast { font-size: 13px; } }
</style>
