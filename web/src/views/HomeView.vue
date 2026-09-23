<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { useSession } from '../stores/session'
import AppIcon from '../components/AppIcon.vue'
const session = useSession()
</script>

<template>
  <section class="home-view" aria-labelledby="home-title">
    <div class="workspace-heading">
      <p class="eyebrow">Your workspace</p>
      <h1 id="home-title">{{ session.identity?.tenant.name }}</h1>
      <p>A place for people, agents, and the work between.</p>
    </div>
    <div class="empty-state glass-card">
      <div class="tree-emblem"><AppIcon name="tree" /></div>
      <p class="eyebrow">Room to grow</p>
      <h2>Your work will take shape here.</h2>
      <p class="empty-description">The work tree arrives with R1.<br />One shared place to connect what comes next.</p>
      <div class="release-note"><span class="release-label">R1</span><span>Work tree</span></div>
    </div>
    <p class="home-note">A clear view. A little more possibility.</p>
  </section>
</template>

<style scoped>
.home-view { width: min(1040px, 100%); min-height: 100%; margin: auto; padding: 44px 32px 24px; display: flex; flex-direction: column; }
.workspace-heading h1 { margin-top: 8px; overflow-wrap: anywhere; }
.workspace-heading > p:last-child { margin-top: 12px; font-size: 14px; }
.empty-state { width: 100%; flex: 1; min-height: 300px; margin-top: 32px; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; padding: 28px; }
.tree-emblem { width: 68px; height: 68px; display: grid; place-items: center; border-radius: 20px; color: var(--teal); background: linear-gradient(145deg, var(--surface), var(--aqua-2)); box-shadow: inset 0 0 0 1px var(--glass-rim), 0 10px 25px -18px var(--teal); margin-bottom: 22px; }
.tree-emblem svg { width: 30px; height: 30px; }
h2 { margin-top: 10px; font-size: clamp(22px, 3vw, 28px); font-weight: 400; letter-spacing: -.025em; line-height: 1.25; text-wrap: balance; }
.empty-description { margin-top: 14px; font-size: 14px; line-height: 1.7; }
.release-note { display: flex; align-items: center; gap: 10px; margin-top: 26px; color: var(--ink-2); font-size: 12px; }
.release-label { border: 1px solid var(--gold); border-radius: 5px; padding: 2px 6px; font: 11px/1.4 var(--mono); color: var(--ink-2); }
.home-note { text-align: center; margin-top: 20px; font-size: 12px; }
@media (max-width: 600px) { .home-view { padding: 30px 20px 20px; } .workspace-heading h1 { font-size: 32px; } .empty-state { margin-top: 26px; padding: 28px 18px; } .home-note { max-width: 250px; align-self: center; } }
</style>
