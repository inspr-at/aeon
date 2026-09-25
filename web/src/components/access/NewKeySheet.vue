<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { createAgentKey, type Agent, type AgentKeyCreated } from '../../lib/access'
import { refreshPermissions } from '../../lib/authz'
import { absoluteTime } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import AccessSheet from './AccessSheet.vue'
import { problem } from './accessText'

// A new key for an agent. The key can do what the agent's role allows, never
// more. Its secret is shown once, here, to copy into the agent's configuration.
const props = defineProps<{ agent: Agent }>()
const emit = defineEmits<{ close: []; created: [] }>()
const LIFETIMES = [{ days: 30, label: '30 days' }, { days: 90, label: '90 days' }, { days: 365, label: 'A year' }, { days: 0, label: 'Until revoked' }]
const days = ref(90)
const busy = ref(false)
const error = ref('')
const created = ref<AgentKeyCreated | null>(null)
const copied = ref(false)
const role = computed(() => props.agent.workspace_role?.name)
async function create() {
  busy.value = true
  error.value = ''
  try {
    const expires = days.value ? new Date(Date.now() + days.value * 86_400_000).toISOString() : null
    created.value = await createAgentKey(props.agent.name, expires)
    emit('created')
    void refreshPermissions()
    await nextTick()
    document.querySelector<HTMLElement>('.token-copy')?.focus()
  } catch (e) { error.value = problem(e, 'The key was not created') }
  finally { busy.value = false }
}
async function copy() { if (!created.value) return; try { await navigator.clipboard.writeText(created.value.token); copied.value = true } catch { copied.value = false } }
</script>

<template>
  <AccessSheet :title="created ? 'Key ready' : `New key for ${agent.name}`" size="center" @close="emit('close')">
    <div v-if="!created" class="body">
      <p class="note"><AppIcon name="shield" :size="14" /><span>The key can do what {{ agent.name }}’s role {{ role ? `(${role})` : '' }} allows, never more. Revoking it stops it at once.</span></p>
      <fieldset class="lifetimes">
        <legend class="label">Works for</legend>
        <div class="seg" role="radiogroup" aria-label="Key works for">
          <button v-for="l in LIFETIMES" :key="l.days" type="button" role="radio" :aria-checked="days === l.days" :data-autofocus="days === l.days ? '' : undefined" @click="days = l.days">{{ l.label }}</button>
        </div>
      </fieldset>
      <p v-if="error" class="set-note error" role="alert"><AppIcon name="alert" :size="14" />{{ error }}</p>
    </div>
    <div v-else class="body">
      <p class="once"><AppIcon name="info" :size="14" /><span>This key is shown only now. Copy it into {{ agent.name }}’s configuration; afterwards only its prefix, aeon_{{ created.prefix }}_…, is shown.{{ created.expires_at ? ` It works until ${absoluteTime(created.expires_at)}.` : '' }}</span></p>
      <div class="token">
        <input class="field mono" readonly :value="created.token" aria-label="New agent key" @focus="($event.target as HTMLInputElement).select()" />
        <button type="button" class="btn primary token-copy" @click="copy"><AppIcon :name="copied ? 'check' : 'copy'" :size="14" />{{ copied ? 'Copied' : 'Copy key' }}</button>
      </div>
    </div>
    <template #foot>
      <template v-if="!created">
        <button type="button" class="btn" @click="emit('close')">Cancel</button>
        <button type="button" class="btn primary" :disabled="busy" @click="create"><AppIcon name="key" :size="13" />{{ busy ? 'Creating…' : 'Create key' }}</button>
      </template>
      <button v-else type="button" class="btn primary" @click="emit('close')">Done</button>
    </template>
  </AccessSheet>
</template>

<style scoped>
.body { display: grid; gap: 14px; }
.note, .once { display: grid; grid-template-columns: 14px 1fr; gap: 8px; padding: 10px 12px; border-radius: 10px; background: var(--surface-2); font-size: 13px; line-height: 1.5; color: var(--ink-2); }
.note svg, .once svg { margin-top: 3px; color: var(--teal-ink); }
.lifetimes { display: grid; gap: 8px; margin: 0; padding: 0; border: 0; }
.label { padding: 0; font: 500 10.5px/1.4 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.seg { display: grid; grid-template-columns: repeat(4, 1fr); }
.token { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px; }
.token .field { font-size: 12.5px; }
@media (max-width: 600px) { .seg { grid-template-columns: repeat(2, 1fr); } .seg button { height: 44px; } .token { grid-template-columns: 1fr; } .token .btn { height: 44px; } }
</style>
