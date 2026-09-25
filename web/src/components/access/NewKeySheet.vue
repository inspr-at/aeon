<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { KEY_PRESETS, KEY_SCOPE_COUNT, KEY_SCOPE_GROUPS, MAX_KEY_SCOPES, createAgentKey, type Agent, type AgentKeyCreated } from '../../lib/access'
import { refreshPermissions } from '../../lib/authz'
import { absoluteTime } from '../../lib/work'
import AppIcon from '../AppIcon.vue'
import AccessSheet from './AccessSheet.vue'
import { problem } from './accessText'

// A new key for an agent: what it may call (its scopes; none means nothing) and
// how long it works. It never does more than the agent's role allows. Its secret
// is shown once, here, to copy into the agent's configuration.
const props = defineProps<{ agent: Agent }>()
const emit = defineEmits<{ close: []; created: [] }>()
const LIFETIMES = [{ days: 30, label: '30 days' }, { days: 90, label: '90 days' }, { days: 365, label: 'A year' }, { days: 0, label: 'Until revoked' }]
const days = ref(90)
const busy = ref(false)
const error = ref('')
const created = ref<AgentKeyCreated | null>(null)
const scopes = ref(new Set<string>())
const tried = ref(false)
function toggle(key: string) { const next = new Set(scopes.value); if (next.has(key)) next.delete(key); else next.add(key); scopes.value = next }
function preset(keys: string[]) { scopes.value = new Set(keys) }
const presetOn = (keys: string[]) => keys.length === scopes.value.size && keys.every(k => scopes.value.has(k))
const scopeProblem = computed(() => !scopes.value.size ? 'Choose at least one thing it may do; a key without scopes can do nothing.'
  : scopes.value.size > MAX_KEY_SCOPES ? `A key holds at most ${MAX_KEY_SCOPES} scopes; clear ${scopes.value.size - MAX_KEY_SCOPES}.` : '')
const copied = ref(false)
const role = computed(() => props.agent.workspace_role?.name)
async function create() {
  tried.value = true
  if (scopeProblem.value) { document.getElementById('key-scopes')?.focus(); return }
  busy.value = true
  error.value = ''
  try {
    const expires = days.value ? new Date(Date.now() + days.value * 86_400_000).toISOString() : null
    created.value = await createAgentKey(props.agent.name, expires, [...scopes.value])
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
      <p class="note"><AppIcon name="shield" :size="14" /><span>The key does only what you tick below, and never more than {{ agent.name }}’s role{{ role ? ` (${role})` : '' }} allows. Revoking it stops it at once.</span></p>
      <fieldset class="lifetimes">
        <legend class="label">Works for</legend>
        <div class="seg" role="radiogroup" aria-label="Key works for">
          <button v-for="l in LIFETIMES" :key="l.days" type="button" role="radio" :aria-checked="days === l.days" :data-autofocus="days === l.days ? '' : undefined" @click="days = l.days">{{ l.label }}</button>
        </div>
      </fieldset>
      <fieldset id="key-scopes" class="scopes" tabindex="-1" :aria-invalid="tried && !!scopeProblem" :aria-describedby="tried && scopeProblem ? 'key-scopes-error' : undefined">
        <legend class="label">What it may do <span class="count">{{ scopes.size }} of {{ KEY_SCOPE_COUNT }}</span></legend>
        <div class="presets">
          <button v-for="p in KEY_PRESETS" :key="p.id" type="button" class="chip-btn" :aria-pressed="presetOn(p.scopes)" @click="preset(p.scopes)">{{ p.label }}</button>
          <button type="button" class="chip-btn" :disabled="!scopes.size" @click="preset([])">Clear</button>
        </div>
        <div v-for="group in KEY_SCOPE_GROUPS" :key="group.label" class="scope-group" role="group" :aria-label="group.label">
          <p class="group-h">{{ group.label }}</p>
          <label v-for="scope in group.scopes" :key="scope.key" class="scope-row">
            <input type="checkbox" :checked="scopes.has(scope.key)" @change="toggle(scope.key)" />
            <span class="scope-text"><span>{{ scope.label }}</span><span class="mono key">{{ scope.key }}</span></span>
          </label>
        </div>
        <p v-if="tried && scopeProblem" id="key-scopes-error" class="field-error" role="alert"><AppIcon name="alert" :size="12" />{{ scopeProblem }}</p>
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
.scopes { display: grid; gap: 10px; margin: 0; padding: 0; border: 0; border-radius: 10px; }
.scopes:focus-visible { box-shadow: var(--focus-ring); }
.count { margin-left: 6px; letter-spacing: .04em; color: var(--ink-3); }
.presets { display: flex; flex-wrap: wrap; gap: 6px; }
.chip-btn { display: inline-flex; align-items: center; height: 30px; padding: 0 12px; border: 0; border-radius: 999px; background: var(--chip-bg); box-shadow: inset 0 0 0 1px var(--chip-line); color: var(--ink-2); font-size: 12.5px; font-weight: 600; }
@media (hover: hover) { .chip-btn:not(:disabled):hover { color: var(--ink); background: var(--row-hover); } }
.chip-btn[aria-pressed="true"] { background: var(--chip-teal-bg); box-shadow: inset 0 0 0 1px var(--chip-teal-line); color: var(--teal-ink); }
.chip-btn:disabled { opacity: .55; }
.chip-btn:focus-visible { box-shadow: var(--focus-ring); }
.scope-group { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 12px; }
.group-h { grid-column: 1 / -1; margin: 4px 0 2px; font: 600 10.5px/1.5 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--ink-3); font-variant-ligatures: none; }
.scope-row { display: grid; grid-template-columns: 16px minmax(0, 1fr); align-items: start; gap: 8px; padding: 5px 0; cursor: pointer; }
.scope-row input { width: 16px; height: 16px; margin: 2px 0 0; accent-color: var(--teal); }
.scope-text { display: grid; gap: 1px; font-size: 13px; line-height: 1.35; color: var(--ink); }
.scope-text .key { font-size: 11px; color: var(--ink-3); }
.field-error { display: flex; align-items: center; gap: 6px; font-size: 12.5px; color: var(--danger); }
.token .field { font-size: 12.5px; }
@media (max-width: 600px) { .seg { grid-template-columns: repeat(2, 1fr); border-radius: 16px; } .seg button { height: 44px; } .token { grid-template-columns: 1fr; } .token .btn { height: 44px; } .scope-group { grid-template-columns: minmax(0, 1fr); } .scope-row { min-height: 44px; align-items: center; } .scope-row input { margin: 0; } .chip-btn { height: 44px; } }
</style>
