<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import mark from '../assets/brand/aeon-mark.svg'
import { useSession } from '../stores/session'
import { useProjects } from '../stores/projects'
import { dark, setTheme, themeChoice, toggleTheme, type ThemeChoice } from '../lib/theme'
import { canWrite } from '../lib/activity'
import { command, consume, run } from '../lib/commands'
import { initials } from '../lib/work'
import { accountEmail, accountName } from '../lib/api'
import AppIcon from './AppIcon.vue'
import CommandPalette from './CommandPalette.vue'
import VersionDisplay from './VersionDisplay.vue'

const session = useSession()
const projects = useProjects()
const route = useRoute()
const router = useRouter()
const open = ref(false)
const busy = ref(false)
const error = ref('')
const account = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
const panel = ref<HTMLElement>()
const palette = ref<InstanceType<typeof CommandPalette>>()
const themes: { value: ThemeChoice; label: string; icon: 'sun' | 'moon' | 'monitor' }[] = [{ value: 'light', label: 'Light', icon: 'sun' }, { value: 'dark', label: 'Dark', icon: 'moon' }, { value: 'system', label: 'System', icon: 'monitor' }]
const writable = computed(() => canWrite(session.identity?.principal.roles))
const name = computed(() => session.identity ? accountName(session.identity) : '')
const email = computed(() => session.identity ? accountEmail(session.identity) : '')
const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)

// The legacy workspace view keeps its own search; everywhere else search is global.
const globalSearch = computed(() => !!session.identity && !route.meta.legacySearch)
const projectKey = computed(() => typeof route.params.projectKey === 'string' ? route.params.projectKey : '')
const project = computed(() => projectKey.value ? projects.byRouteKey(projectKey.value) : undefined)
const onProjects = computed(() => route.path === '/')
// Full-page tickets get their own crumb; the side panel keeps the list as the page.
const fullTicket = computed(() => typeof route.params.ticketKey === 'string' && route.query.view === 'full' ? route.params.ticketKey.toUpperCase() : '')
const pageTitle = computed(() => !onProjects.value && !projectKey.value && route.path !== '/signin' ? String(route.meta.title ?? '') : '')

async function toggleMenu() {
  open.value = !open.value
  if (open.value) { await nextTick(); focusTheme() }
}
function focusTheme() { panel.value?.querySelector<HTMLElement>('[role="radio"][aria-checked="true"]')?.focus() }
// Up and down walk the menu's rows (the theme group is one stop); left and right choose a theme.
function menuKeys(event: KeyboardEvent) {
  // Keys pressed inside the menu belong to it, not to the page underneath (Escape and ⌘K still pass).
  if (event.key !== 'Escape' && !event.metaKey && !event.ctrlKey) event.stopPropagation()
  if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
  const items = [...(panel.value?.querySelectorAll<HTMLElement>('button:not(:disabled):not([role="radio"][aria-checked="false"]), [role="button"]') ?? [])]
  const index = items.indexOf(document.activeElement as HTMLElement)
  event.preventDefault()
  items[(index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length]?.focus()
}
function themeKeys(event: KeyboardEvent) {
  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
  event.preventDefault()
  const index = themes.findIndex(theme => theme.value === themeChoice.value)
  setTheme(themes[(index + (event.key === 'ArrowRight' ? 1 : -1) + themes.length) % themes.length].value)
  void nextTick(focusTheme)
}
function showShortcuts() { closeMenu(); run({ name: 'shortcuts' }) }
watch(command, value => { if (value?.command.name === 'palette') { consume(); palette.value?.open() } })
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
function typing(target: EventTarget | null) {
  return target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
}
// Pages with their own list search keep '/'; everywhere else it opens the palette.
const pageOwnsSlash = computed(() => route.path === '/' || (!!projectKey.value && route.query.view !== 'full'))
function shortcut(event: KeyboardEvent) {
  if (!globalSearch.value) return
  if ((event.metaKey || event.ctrlKey) && !event.altKey && event.key.toLowerCase() === 'k') { event.preventDefault(); palette.value?.open(); return }
  if (event.metaKey || event.ctrlKey || event.altKey || event.defaultPrevented || typing(event.target) || document.querySelector('dialog[open], .floating')) return
  if (event.key === '/' && !pageOwnsSlash.value) { event.preventDefault(); palette.value?.open() }
  else if (event.key === '?') { event.preventDefault(); run({ name: 'shortcuts' }) }
}
onMounted(() => { document.addEventListener('pointerdown', outside); window.addEventListener('keydown', shortcut) })
onBeforeUnmount(() => { document.removeEventListener('pointerdown', outside); window.removeEventListener('keydown', shortcut) })
</script>

<template>
  <header class="app-header">
    <RouterLink class="lockup" to="/" aria-label="PAIMOS AEON home" :class="{ compact: !!projectKey || !!pageTitle }">
      <span class="mark-backing"><img :src="mark" width="26" height="26" alt="" /></span>
      <span class="wordmark">PAIMOS<sup>AEON</sup></span>
    </RouterLink>
    <nav v-if="session.identity" class="crumbs" :class="{ deep: !!projectKey || !!pageTitle }" aria-label="Breadcrumb">
      <RouterLink class="crumb" to="/" :aria-current="onProjects ? 'page' : undefined">Projects</RouterLink>
      <template v-if="projectKey">
        <span class="sep" aria-hidden="true">/</span>
        <RouterLink class="crumb project-crumb" :to="`/p/${encodeURIComponent(project?.routeKey ?? projectKey)}`" :aria-current="fullTicket ? undefined : 'page'">
          <span class="key-badge">{{ project?.routeKey ?? projectKey.toUpperCase() }}</span>
          <span class="crumb-name">{{ project?.title ?? '' }}</span>
        </RouterLink>
        <template v-if="fullTicket">
          <span class="sep" aria-hidden="true">/</span>
          <span class="crumb current mono-crumb" aria-current="page">{{ fullTicket }}</span>
        </template>
      </template>
      <template v-else-if="pageTitle">
        <span class="sep" aria-hidden="true">/</span>
        <span class="crumb current" aria-current="page">{{ pageTitle }}</span>
      </template>
    </nav>
    <span class="spacer" />
    <button v-if="globalSearch" class="search-pill" type="button" aria-label="Search everything" aria-keyshortcuts="Control+K Meta+K" @click="palette?.open()">
      <AppIcon name="search" :size="15" />
      <span class="pill-text">Search</span>
      <span class="pill-keys"><kbd class="keycap">{{ mac ? '⌘' : 'Ctrl' }}</kbd><kbd class="keycap">K</kbd></span>
    </button>
    <button class="icon-btn header-btn" type="button" :aria-label="dark ? 'Switch to light theme' : 'Switch to dark theme'" :data-tip="dark ? 'Light theme' : 'Dark theme'" @click="toggleTheme">
      <AppIcon :name="dark ? 'sun' : 'moon'" />
    </button>
    <div v-if="session.identity" ref="account" class="account" @keydown.esc.stop.prevent="closeMenu(true)" @focusout="focusOut">
      <button ref="trigger" class="avatar-btn header-btn" type="button" :aria-expanded="open" aria-controls="account-panel" :aria-label="`Account for ${name}`" @click="toggleMenu">
        {{ initials(name) }}
      </button>
      <div v-if="open" id="account-panel" ref="panel" class="account-panel pop" role="dialog" aria-label="Account" @keydown="menuKeys">
        <div class="who">
          <span class="who-avatar" aria-hidden="true">{{ initials(name) }}</span>
          <div class="who-text">
            <p class="account-name">{{ name }}</p>
            <p v-if="email" class="account-email">{{ email }}</p>
            <p class="account-tenant" title="Workspace"><AppIcon name="folder" :size="12" /><span class="sr-only">Workspace: </span>{{ session.identity.tenant.name }}</p>
          </div>
        </div>
        <div class="menu-block">
          <p class="eyebrow">Theme</p>
          <div class="seg theme-seg" role="radiogroup" aria-label="Theme" @keydown="themeKeys">
            <button v-for="option in themes" :key="option.value" type="button" role="radio" :aria-checked="themeChoice === option.value" :tabindex="themeChoice === option.value ? 0 : -1" @click="setTheme(option.value)"><AppIcon :name="option.icon" :size="13" />{{ option.label }}</button>
          </div>
        </div>
        <button class="menu-row" type="button" aria-keyshortcuts="?" @click="showShortcuts"><AppIcon name="keyboard" />Keyboard shortcuts<kbd class="keycap row-key" aria-hidden="true">?</kbd></button>
        <button class="menu-row" type="button" :disabled="busy" @click="signOut"><AppIcon name="logout" />{{ busy ? 'Signing out…' : 'Sign out' }}</button>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <div class="menu-version"><span class="eyebrow">PAIMOS AEON</span><VersionDisplay /></div>
      </div>
    </div>
    <CommandPalette v-if="globalSearch" ref="palette" :can-write="writable" />
  </header>
</template>

<style scoped>
.app-header {
  position: relative; z-index: 20; height: var(--header-h); padding: 0 28px; display: flex; align-items: center; gap: 14px;
  background: var(--glass-2); border-bottom: 1px solid var(--glass-edge); box-shadow: 0 1px 0 var(--line);
  backdrop-filter: blur(16px) saturate(1.2); -webkit-backdrop-filter: blur(16px) saturate(1.2);
}
.lockup { display: inline-flex; align-items: center; gap: 10px; min-height: 40px; padding-right: 4px; color: var(--ink); flex-shrink: 0; border-radius: 10px; }
.lockup:focus-visible { box-shadow: var(--focus-ring); }
.mark-backing { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 9px; background: #f7f6f2; box-shadow: 0 0 0 1px var(--glass-rim); }
.mark-backing img { display: block; }
.wordmark { font: 600 13px/1 var(--mono); letter-spacing: .28em; white-space: nowrap; font-variant-ligatures: none; }
.wordmark sup { position: relative; top: -.15em; margin-left: 2px; font: 600 8px/1 var(--mono); letter-spacing: .16em; color: var(--teal-ink); }
.crumbs { display: flex; align-items: center; gap: 10px; flex: 0 1 auto; min-width: 0; overflow: hidden; padding-left: 16px; border-left: 1px solid var(--line-2); height: 24px; font-size: 13.5px; }
.crumb { display: inline-flex; align-items: center; gap: 8px; min-width: 0; height: 30px; padding: 0 8px; margin: 0 -8px; border-radius: 8px; color: var(--ink-2); font-weight: 600; white-space: nowrap; }
.crumb:hover { color: var(--teal-ink); background: var(--row-hover); }
.crumb:active { background: var(--row-selected); }
.crumb:focus-visible { box-shadow: var(--focus-ring); }
.crumb[aria-current="page"] { color: var(--ink); }
.crumb.current { color: var(--ink); cursor: default; }
.crumb.current:hover { background: transparent; }
.crumb-name { overflow: hidden; text-overflow: ellipsis; color: var(--ink); }
.project-crumb { min-width: 0; }
.sep { color: var(--ink-3); font-weight: 300; font-size: 16px; }
.mono-crumb { font: 500 12px/1 var(--mono); letter-spacing: .02em; font-variant-ligatures: none; }
.spacer { flex: 1 1 0; min-width: 0; }
.search-pill, .header-btn, .account { flex-shrink: 0; }
.search-pill {
  display: inline-flex; align-items: center; gap: 9px; width: 240px; height: 34px; padding: 0 6px 0 12px; border: 1px solid var(--glass-edge); border-radius: 999px;
  background: var(--field-bg); box-shadow: var(--field-inset), 0 0 0 1px var(--line); color: var(--ink-3); font-size: 13px;
}
.search-pill:hover { color: var(--ink-2); box-shadow: var(--field-inset), 0 0 0 1px var(--glass-rim); }
.search-pill:active { background: var(--row-selected); }
.search-pill:focus-visible { box-shadow: var(--focus-ring); }
.pill-text { flex: 1; text-align: left; }
.pill-keys { display: inline-flex; gap: 3px; }
.account { position: relative; }
.avatar-btn {
  display: grid; place-items: center; width: 34px; height: 34px; padding: 0; border: 0; border-radius: 50%;
  background: var(--avatar-bg); color: var(--teal-ink); box-shadow: 0 0 0 1px var(--glass-rim), 0 2px 6px rgba(32, 60, 61, .12);
  font: 700 12px/1 var(--mono); letter-spacing: .02em; font-variant-ligatures: none;
}
.avatar-btn:hover, .avatar-btn[aria-expanded="true"] { box-shadow: 0 0 0 1px var(--teal), 0 2px 8px rgba(32, 60, 61, .18); }
.avatar-btn:active { filter: brightness(.96); }
.avatar-btn:focus-visible { box-shadow: var(--focus-ring); }
.account-panel { position: absolute; right: 0; top: 44px; width: min(300px, calc(100vw - 24px)); padding: 8px; }
.who { display: flex; align-items: center; gap: 12px; padding: 10px 10px 12px; margin-bottom: 4px; border-bottom: 1px solid var(--line); }
.who-avatar { display: grid; place-items: center; flex-shrink: 0; width: 40px; height: 40px; border-radius: 50%; background: var(--avatar-bg); color: var(--teal-ink); box-shadow: 0 0 0 1px var(--glass-rim); font: 700 13px/1 var(--mono); font-variant-ligatures: none; }
.who-text { min-width: 0; }
.account-name { color: var(--ink); font-weight: 650; font-size: 14px; overflow-wrap: anywhere; }
.account-email { font-size: 12.5px; color: var(--ink-2); overflow-wrap: anywhere; }
.account-tenant { display: inline-flex; align-items: center; gap: 5px; margin-top: 4px; padding: 2px 8px 2px 6px; border-radius: 999px; background: var(--code-bg); font-size: 11.5px; color: var(--ink-2); }
.menu-block { display: grid; gap: 6px; padding: 8px 10px 10px; }
.menu-block .eyebrow { margin: 0; }
.theme-seg { display: grid; grid-template-columns: repeat(3, 1fr); }
.theme-seg button { height: 30px; gap: 5px; padding: 0 6px; }
.menu-row { display: flex; align-items: center; gap: 10px; width: 100%; height: 36px; padding: 0 10px; border: 0; border-radius: 8px; background: transparent; color: var(--ink); font-size: 13.5px; text-align: left; }
.menu-row svg { color: var(--ink-2); }
@media (hover: hover) { .menu-row:hover:not(:disabled) { background: var(--row-hover); } }
.menu-row:active:not(:disabled) { background: var(--row-selected); }
.menu-row:focus-visible { background: var(--row-selected); box-shadow: inset 0 0 0 1px var(--glass-rim); }
.row-key { margin-left: auto; }
.error { margin: 8px 10px 4px; }
.menu-version { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-top: 6px; padding: 0 10px; border-top: 1px solid var(--line); }
.menu-version .eyebrow { letter-spacing: .2em; }
.menu-version .eyebrow { margin: 0; }
@media (max-width: 900px) { .search-pill { width: 180px; } }
@media (max-width: 600px) {
  .app-header { gap: 8px; padding: 0 12px; }
  .lockup { min-height: 44px; min-width: 44px; justify-content: center; }
  .lockup.compact .wordmark { display: none; }
  .wordmark { font-size: 11.5px; letter-spacing: .22em; }
  .crumbs { padding-left: 10px; gap: 8px; }
  .crumbs.deep > .crumb:first-child, .crumbs.deep > .sep { display: none; }
  .crumb.current { overflow: hidden; text-overflow: ellipsis; }
  .crumb { height: 44px; }
  .crumb-name { display: none; }
  .search-pill { width: 44px; height: 44px; padding: 0; justify-content: center; }
  .pill-text, .pill-keys { display: none; }
  .header-btn, .avatar-btn { width: 44px; height: 44px; }
}
</style>
