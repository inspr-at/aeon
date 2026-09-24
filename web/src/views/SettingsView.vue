<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, watch, type Component } from 'vue'
import { useRoute } from 'vue-router'
import '../styles/settings.css'
import { isTenantAdmin } from '../components/business/catalog'
import AppIcon from '../components/AppIcon.vue'
import BizIcon, { type BizIconName } from '../components/business/BizIcon.vue'
import BusinessSection from '../components/settings/BusinessSection.vue'
import PersonalSection from '../components/settings/PersonalSection.vue'
import ProjectsSection from '../components/settings/ProjectsSection.vue'
import WorkspaceSection from '../components/settings/WorkspaceSection.vue'
import { SETTINGS_SECTIONS, sectionOf, visibleSections, type SectionId } from '../lib/settings'
import { useSession } from '../stores/session'

// Settings: Personal for everyone; Workspace, Business and Projects for admins.
// /settings/<section>#<card> deep-links to one card, which is ringed on arrival.
const route = useRoute()
const session = useSession()
const admin = computed(() => isTenantAdmin(session.identity))
const sections = computed(() => visibleSections(admin.value))
const current = computed(() => sectionOf(route.params.section))
const meta = computed(() => SETTINGS_SECTIONS.find(section => section.id === current.value)!)
const allowed = computed(() => !meta.value.admin || admin.value)
const VIEW: Record<SectionId, Component> = { personal: PersonalSection, workspace: WorkspaceSection, business: BusinessSection, projects: ProjectsSection }
const ICON: Record<SectionId, BizIconName> = { personal: 'user', workspace: 'users', business: 'briefcase', projects: 'layers' }

// A deep link scrolls to its card once the section has rendered it.
let arrival: ReturnType<typeof setTimeout> | undefined
watch(() => [current.value, route.hash] as const, async ([, hash]) => {
  if (!hash) return
  for (let tries = 0; tries < 20; tries++) {
    await nextTick()
    const target = document.getElementById(decodeURIComponent(hash.slice(1)))
    if (target) {
      target.scrollIntoView({ block: 'start', behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
      const card = target.classList.contains('settings-card') ? target : target.querySelector<HTMLElement>('.settings-card, .setup')
      card?.classList.add('arrived')
      clearTimeout(arrival)
      arrival = setTimeout(() => card?.classList.remove('arrived'), 1800)
      return
    }
    await new Promise(resolve => setTimeout(resolve, 50))
  }
}, { immediate: true })
</script>

<template>
  <section class="settings-page" aria-labelledby="settings-title">
    <header class="page-head">
      <p class="eyebrow">{{ session.identity?.tenant.name ?? 'Workspace' }}</p>
      <h1 id="settings-title">Settings</h1>
      <p class="summary">{{ admin ? 'Your own preferences, and the workspace’s for admins.' : 'Your own preferences.' }}</p>
    </header>
    <!-- One grid for everyone: with only Personal to show, the nav still holds its column. -->
    <div class="layout" :class="{ single: sections.length < 2 }">
      <nav class="section-nav" aria-label="Settings sections">
        <RouterLink v-for="section in sections" :key="section.id" :to="`/settings/${section.id}`" class="section-link" :aria-current="section.id === current ? 'page' : undefined">
          <span class="link-icon" aria-hidden="true"><BizIcon :name="ICON[section.id]" :size="15" /></span>
          <span class="link-text"><span class="link-label">{{ section.label }}</span><span class="link-summary">{{ section.summary }}</span></span>
          <span v-if="section.admin" class="admin-mark" role="img" aria-label="Admins only" data-tip="Only workspace admins see this"><AppIcon name="shield" :size="12" /></span>
        </RouterLink>
      </nav>
      <div class="body">
        <component :is="VIEW[current]" v-if="allowed" :key="current" />
        <div v-else class="gate glass-card">
          <span class="gate-icon"><AppIcon name="shield" :size="18" /></span>
          <h2>{{ meta.label }} settings are for workspace admins</h2>
          <p>A workspace admin can change these. Your own settings are under Personal.</p>
          <RouterLink class="btn" to="/settings/personal">Personal settings</RouterLink>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.settings-page { width: 100%; max-width: 1180px; margin: 0 auto; padding: 22px 28px 40px; }
.page-head { margin-bottom: 20px; }
.page-head h1 { margin-top: 6px; }
.summary { margin-top: 6px; font-size: 13.5px; color: var(--ink-2); }
.layout { display: grid; grid-template-columns: 240px minmax(0, 1fr); gap: 28px; align-items: start; }
.section-nav { position: sticky; top: 16px; display: grid; gap: 4px; }
.section-link { display: grid; grid-template-columns: 30px minmax(0, 1fr) auto; align-items: center; gap: 10px; min-height: 52px; padding: 8px 10px; border-radius: 12px; color: var(--ink); text-decoration: none; }
@media (hover: hover) { .section-link:hover { background: var(--row-hover); } }
.section-link:focus-visible { box-shadow: var(--focus-ring); }
/* The current section: a raised card, like the active place. */
.section-link[aria-current="page"] { background: var(--surface-raised); box-shadow: 0 0 0 1px var(--line), 0 6px 18px -12px rgba(32, 60, 61, .4); }
.link-icon { display: grid; place-items: center; width: 30px; height: 30px; border-radius: 9px; background: var(--surface-2); color: var(--ink-2); }
.section-link[aria-current="page"] .link-icon { background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.link-text { display: grid; min-width: 0; }
.link-label { font-weight: 600; font-size: 13.5px; }
.link-summary { font-size: 12px; color: var(--ink-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.admin-mark { display: grid; place-items: center; width: 22px; height: 22px; border-radius: 50%; color: var(--ink-3); }
.body { min-width: 0; }
.gate { display: grid; justify-items: center; gap: 8px; max-width: 560px; margin: 0 auto; padding: 40px 28px; text-align: center; }
.gate h2 { font-size: 17px; }
.gate p { font-size: 13.5px; max-width: 46ch; }
.gate .btn { margin-top: 8px; }
.gate-icon { display: grid; place-items: center; width: 44px; height: 44px; margin-bottom: 4px; border-radius: 50%; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
@media (max-width: 900px) {
  .layout { grid-template-columns: minmax(0, 1fr); gap: 16px; }
  /* Narrow: the sections become a two-by-two grid above the page. */
  .section-nav { position: static; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; }
  .section-link { grid-template-columns: 28px minmax(0, 1fr) auto; min-height: 48px; padding: 6px 10px; background: var(--glass); box-shadow: 0 0 0 1px var(--line); }
  .link-summary { display: none; }
  /* Narrow, a single section needs no nav above it. */
  .single .section-nav { display: none; }
}
@media (max-width: 600px) {
  .settings-page { padding: 14px 16px 28px; }
  .page-head { margin-bottom: 14px; }
}
</style>
