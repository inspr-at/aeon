<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { brand } from '../../lib/brand'
import { accountEmail, accountName } from '../../lib/api'
import { run } from '../../lib/commands'
import { getProfile, patchProfile, type Profile } from '../../lib/settings'
import { setTheme, themeChoice, type ThemeChoice } from '../../lib/theme'
import { toast } from '../../lib/toast'
import { initials } from '../../lib/work'
import { useSession } from '../../stores/session'
import AppIcon from '../AppIcon.vue'
import SettingsCard from './SettingsCard.vue'

// Everyone's own settings: who you are signed in as (profile editing arrives next),
// the theme, the greeting, and the keys.
const session = useSession()
const name = computed(() => session.identity ? accountName(session.identity) : '')
const email = computed(() => session.identity ? accountEmail(session.identity) : '')
const roles = computed(() => session.identity?.principal.roles ?? [])
const ROLE: Record<string, string> = { admin: 'Admin', member: 'Member', viewer: 'Viewer' }

const themes: { value: ThemeChoice; label: string; icon: 'sun' | 'moon' | 'monitor' }[] = [
  { value: 'light', label: 'Light', icon: 'sun' }, { value: 'dark', label: 'Dark', icon: 'moon' }, { value: 'system', label: 'System', icon: 'monitor' },
]
const themeGroup = ref<HTMLElement>()
function themeKeys(event: KeyboardEvent) {
  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
  event.preventDefault()
  const index = themes.findIndex(theme => theme.value === themeChoice.value)
  setTheme(themes[(index + (event.key === 'ArrowRight' ? 1 : -1) + themes.length) % themes.length].value)
  void nextTick(() => themeGroup.value?.querySelector<HTMLElement>('[aria-checked="true"]')?.focus())
}

// ---------- Greeting (profile.greeting_enabled) ----------
const profile = ref<Profile | null>(null)
const profileError = ref('')
const saving = ref(false)
async function load() {
  profileError.value = ''
  try { profile.value = await getProfile() } catch { profileError.value = 'Your greeting setting could not be loaded.' }
}
async function setGreeting(on: boolean) {
  if (!profile.value || saving.value) return
  const before = profile.value.greeting_enabled
  profile.value = { ...profile.value, greeting_enabled: on }
  saving.value = true
  try {
    profile.value = await patchProfile({ greeting_enabled: on })
    toast(on ? 'The greeting is on.' : 'The greeting is off.')
  } catch {
    profile.value = { ...profile.value, greeting_enabled: before }
    toast('Your greeting setting could not be saved. Please try again.', { tone: 'error' })
  } finally { saving.value = false }
}
onMounted(load)

const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)
const KEYS: { keys: string[][]; label: string }[] = [
  { keys: [[mac ? '⌘' : 'Ctrl', 'K']], label: 'Search everything' },
  { keys: [['g', 'p'], ['g', 'a'], ['g', 'b']], label: 'Go to Projects, Agents or Business' },
  { keys: [['?']], label: 'All shortcuts on the page you are on' },
]
</script>

<template>
  <div class="section">
    <SettingsCard title="Profile" icon="user" anchor="profile">
      <template #lead>How you appear to people and agents in this workspace.</template>
      <div class="who">
        <span class="avatar" aria-hidden="true">{{ initials(name) }}</span>
        <div class="who-text">
          <p class="who-name">{{ name }}</p>
          <p v-if="email" class="who-email">{{ email }}</p>
          <p class="who-roles">
            <span class="chip">{{ session.identity?.tenant.name }}</span>
            <span v-for="role in roles" :key="role" class="chip">{{ ROLE[role] ?? role }}</span>
          </p>
        </div>
      </div>
      <p class="set-note next"><AppIcon name="info" :size="14" />Editing your name, photo, time zone and language arrives here next.</p>
    </SettingsCard>

    <SettingsCard title="Appearance" icon="sun" anchor="appearance">
      <template #lead>Light, dark, or whatever your system uses. It applies to this browser session.</template>
      <template #aside>
        <div ref="themeGroup" class="seg" role="radiogroup" aria-label="Theme" @keydown="themeKeys">
          <button v-for="option in themes" :key="option.value" type="button" role="radio" :aria-checked="themeChoice === option.value" :tabindex="themeChoice === option.value ? 0 : -1" @click="setTheme(option.value)">
            <AppIcon :name="option.icon" :size="13" />{{ option.label }}
          </button>
        </div>
      </template>
    </SettingsCard>

    <SettingsCard title="Greeting" icon="sparkle" anchor="greeting">
      <template #lead>A short, personal line when you open {{ brand.short_name }}. Only you see it.</template>
      <template #aside>
        <span v-if="!profile && !profileError" class="skeleton switch-skeleton" role="status" aria-label="Loading" />
        <button v-else-if="profileError" type="button" class="btn sm" @click="load"><AppIcon name="refresh" :size="12" />Try again</button>
        <label v-else class="switch">
          <input type="checkbox" :checked="profile!.greeting_enabled" :disabled="saving" aria-labelledby="greeting-title greeting-state" @change="setGreeting(($event.target as HTMLInputElement).checked)" />
          <span id="greeting-state">{{ profile!.greeting_enabled ? 'On' : 'Off' }}</span>
        </label>
      </template>
      <p v-if="profileError" class="error-line" role="alert"><AppIcon name="alert" :size="13" />{{ profileError }}</p>
    </SettingsCard>

    <SettingsCard title="Keyboard" icon="keyboard" anchor="keys">
      <template #lead>Most of {{ brand.short_name }} works from the keyboard.</template>
      <template #aside><button type="button" class="btn sm" aria-keyshortcuts="?" @click="run({ name: 'shortcuts' })">All shortcuts<kbd class="keycap" aria-hidden="true">?</kbd></button></template>
      <dl class="keys">
        <div v-for="row in KEYS" :key="row.label">
          <dt><template v-for="(combo, i) in row.keys" :key="i"><span v-if="i" class="or" aria-hidden="true">·</span><span class="combo"><kbd v-for="k in combo" :key="k" class="keycap">{{ k }}</kbd></span></template></dt>
          <dd>{{ row.label }}</dd>
        </div>
      </dl>
    </SettingsCard>
  </div>
</template>

<style scoped>
.section { display: grid; gap: 14px; }
.who { display: flex; align-items: center; gap: 14px; }
.avatar { display: grid; place-items: center; flex-shrink: 0; width: 52px; height: 52px; border-radius: 50%; background: var(--avatar-bg); color: var(--teal-ink); box-shadow: 0 0 0 1px var(--glass-rim); font: 700 16px/1 var(--mono); font-variant-ligatures: none; }
.who-text { min-width: 0; display: grid; gap: 2px; }
.who-name { font-weight: 650; font-size: 15px; color: var(--ink); overflow-wrap: anywhere; }
.who-email { font-size: 13px; color: var(--ink-2); overflow-wrap: anywhere; }
.who-roles { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
.next { margin-top: 14px; }
.seg button { height: 30px; }
.switch-skeleton { width: 72px; height: 20px; }
.error-line { display: flex; align-items: center; gap: 6px; font-size: 13px; color: var(--danger); }
.keys { display: grid; margin: 0; }
.keys > div { display: grid; grid-template-columns: minmax(150px, max-content) 1fr; align-items: center; gap: 16px; min-height: 38px; border-top: 1px solid var(--line); }
.keys dt { display: flex; flex-wrap: wrap; align-items: center; gap: 4px; }
.combo { display: inline-flex; gap: 3px; }
.or { color: var(--ink-3); padding: 0 2px; }
.keys dd { margin: 0; font-size: 13px; color: var(--ink-2); }
.btn .keycap { margin-right: -4px; }
@media (max-width: 600px) {
  .keys > div { grid-template-columns: minmax(0, 1fr); gap: 4px; padding: 8px 0; }
  .seg { width: 100%; display: grid; grid-template-columns: repeat(3, 1fr); }
  .seg button { height: 40px; }
}
</style>
