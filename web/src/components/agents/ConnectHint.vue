<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { ref } from 'vue'
import AppIcon from '../AppIcon.vue'

// The empty state: how an agent shows up here. Managed agents started by the local
// daemon register themselves; any other harness registers with the aeon CLI.
const origin = window.location.origin
const steps = [
  { label: 'Log the agent in once with its agent key', command: `aeon auth login --url ${origin} --name default --key-file ./agent.key` },
  { label: 'Register the session from inside the harness', command: 'aeon harness register --project KEY --agent NAME --harness claude --host $(hostname) --harness-session-file PATH --worker-lease-file PATH' },
]
const copied = ref(-1)
async function copy(index: number) {
  try { await navigator.clipboard.writeText(steps[index].command); copied.value = index; setTimeout(() => { if (copied.value === index) copied.value = -1 }, 1600) }
  catch { copied.value = -1 }
}
</script>

<template>
  <div class="connect">
    <span class="halo"><AppIcon name="agent" :size="22" /></span>
    <h3>No agent has connected yet</h3>
    <p class="lead">Codex, Claude, Pi, Cursor and Grok sessions show up here while they work: what they are on, how they pace, and what they need from you.</p>
    <ol class="steps">
      <li v-for="(step, index) in steps" :key="index">
        <span class="step-label"><span class="n">{{ index + 1 }}</span>{{ step.label }}</span>
        <span class="command">
          <code>{{ step.command }}</code>
          <button type="button" class="icon-btn sm flat" :aria-label="`Copy: ${step.label}`" :data-tip="copied === index ? 'Copied' : 'Copy'" @click="copy(index)">
            <AppIcon :name="copied === index ? 'check' : 'copy'" :size="13" />
          </button>
        </span>
      </li>
    </ol>
    <p class="fine">Sessions started by <code>paimos-agentd</code> register themselves. Use <code>claude</code>, <code>codex</code>, <code>pi</code>, <code>cursor</code> or <code>grok</code> for <code>--harness</code>.</p>
  </div>
</template>

<style scoped>
.connect { display: grid; justify-items: center; gap: 10px; padding: 40px 28px 44px; border-top: 1px solid var(--line); text-align: center; }
.halo { display: grid; place-items: center; width: 52px; height: 52px; margin-bottom: 4px; border-radius: 16px; background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line), 0 8px 24px -12px var(--teal); color: var(--teal-ink); }
h3 { font-size: 17px; }
.lead { max-width: 56ch; font-size: 13.5px; color: var(--ink-2); }
.steps { display: grid; grid-template-columns: minmax(0, 1fr); gap: 12px; width: min(100%, 720px); margin: 14px 0 4px; padding: 0; list-style: none; text-align: left; }
.step-label { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; font-size: 13px; font-weight: 600; color: var(--ink); }
.n { display: inline-grid; place-items: center; width: 20px; height: 20px; border-radius: 50%; background: var(--chip-teal-bg); color: var(--teal-ink); font: 600 11px/1 var(--mono); }
.command { display: flex; align-items: flex-start; gap: 6px; padding: 8px 6px 8px 12px; border-radius: 10px; background: var(--code-bg); box-shadow: inset 0 0 0 1px var(--line); }
.command code { flex: 1; min-width: 0; padding-top: 5px; font-size: 12px; line-height: 1.55; color: var(--ink); overflow-wrap: anywhere; }
.fine { max-width: 60ch; font-size: 12.5px; color: var(--ink-3); }
.fine code { padding: 1px 5px; border-radius: 5px; background: var(--code-bg); font-size: 11.5px; color: var(--ink-2); }
</style>
