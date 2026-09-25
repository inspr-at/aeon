<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getQuoteSettings, saveQuoteSettings } from '../../lib/settings'
import { archiveProfile, defaultProfile, listProfiles, saveProfile, undoProfile, uploadProfileAsset, type QuoteProfile } from '../../lib/quotes/profile'
import type { QuoteProfileDefinition } from '../../lib/quotes/types'
import SettingsCard from './SettingsCard.vue'

const profiles = ref<QuoteProfile[]>([])
const selected = ref<QuoteProfile | null>(null)
const name = ref('')
const definition = ref(JSON.stringify(defaultProfile(), null, 2))
const defaultID = ref('')
const busy = ref(false)
const message = ref('')
const error = ref('')
async function load() {
  try {
    profiles.value = await listProfiles()
    defaultID.value = (await getQuoteSettings()).default_profile_id || ''
  } catch (e) { error.value = e instanceof Error ? e.message : 'Profiles could not be loaded.' }
}
function choose(profile: QuoteProfile | null) {
  selected.value = profile
  name.value = profile?.name || ''
  definition.value = JSON.stringify(profile?.definition || defaultProfile(), null, 2)
  error.value = ''; message.value = ''
}
async function save() {
  if (busy.value) return
  let parsed: QuoteProfileDefinition
  try { parsed = JSON.parse(definition.value) as QuoteProfileDefinition } catch { error.value = 'Profile JSON is invalid.'; return }
  busy.value = true; error.value = ''; message.value = ''
  try {
    const saved = await saveProfile(name.value.trim(), parsed, selected.value || undefined)
    await load(); choose(saved); message.value = `Saved revision ${saved.revision}.`
  } catch (e) { error.value = e instanceof Error ? e.message : 'Profile was not saved.' }
  finally { busy.value = false }
}
async function archive() {
  if (!selected.value || busy.value) return
  busy.value = true; error.value = ''
  try { await archiveProfile(selected.value.id); await load(); choose(null); message.value = 'Profile archived.' }
  catch (e) { error.value = e instanceof Error ? e.message : 'Archive failed.' }
  finally { busy.value = false }
}
async function undo() {
  if (!selected.value || busy.value) return
  busy.value = true; error.value = ''
  try { const saved = await undoProfile(selected.value); await load(); choose(saved); message.value = `Restored as revision ${saved.revision}.` }
  catch (e) { error.value = e instanceof Error ? e.message : 'Restore failed.' }
  finally { busy.value = false }
}
async function setDefault() {
  if (!selected.value || busy.value) return
  busy.value = true; error.value = ''
  try {
    const settings = await getQuoteSettings()
    await saveQuoteSettings(settings, { sender: settings.sender, default_currency: settings.default_currency, numbering_time_zone: settings.numbering_time_zone, default_profile_id: selected.value.id })
    defaultID.value = selected.value.id; message.value = 'New quotes will use this profile.'
  } catch (e) { error.value = e instanceof Error ? e.message : 'Default was not saved.' }
  finally { busy.value = false }
}
async function upload(event: Event, role: 'font' | 'mark') {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file || busy.value) return
  busy.value = true; error.value = ''
  try {
    const asset = await uploadProfileAsset(file)
    const parsed = JSON.parse(definition.value) as QuoteProfileDefinition
    if (role === 'mark') parsed.footer.asset_id = asset.id
    else parsed.fonts.push({ role: 'body', family: file.name.replace(/\.[^.]+$/, ''), weight: 400, style: 'normal', asset_id: asset.id })
    definition.value = JSON.stringify(parsed, null, 2)
    message.value = 'Asset uploaded. Save the profile to use it.'
  } catch (e) { error.value = e instanceof Error ? e.message : 'Upload failed.' }
  finally { busy.value = false; input.value = '' }
}
onMounted(load)
</script>
<template>
  <SettingsCard title="Quote document profiles" icon="document" anchor="quote-profiles">
    <template #lead>Save a document design as an immutable revision. New quotes can use the selected default; issued versions keep their own snapshot.</template>
    <div class="profiles">
      <div class="profile-list">
        <label for="profile-select">Profile</label>
        <select id="profile-select" class="field" :value="selected?.id || ''" @change="choose(profiles.find(p => p.id === ($event.target as HTMLSelectElement).value) || null)">
          <option value="">New profile</option>
          <option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{ profile.name }} · r{{ profile.revision }}{{ profile.archived ? ' · archived' : '' }}{{ defaultID === profile.id ? ' · default' : '' }}</option>
        </select>
        <label for="profile-name">Name</label>
        <input id="profile-name" v-model="name" class="field" maxlength="100" placeholder="Company quote" />
        <div class="upload-row">
          <label class="btn sm ghost">Upload font<input type="file" accept=".ttf,.otf,.woff2" class="visually-hidden" @change="upload($event, 'font')" /></label>
          <label class="btn sm ghost">Upload footer mark<input type="file" accept=".svg,.png" class="visually-hidden" @change="upload($event, 'mark')" /></label>
        </div>
        <p class="hint">Fonts are listed in the JSON with role, weight and style. Assets are served from this workspace when quotes print.</p>
      </div>
      <div class="definition">
        <label for="profile-definition">Profile definition</label>
        <textarea id="profile-definition" v-model="definition" class="field json" rows="20" spellcheck="false" aria-describedby="profile-json-help" />
        <p id="profile-json-help" class="hint">Edit colors, type scale, A4 margins, cover, sections, table, totals, payment terms, signatures, footer and labels. Values in millimetres or points are exact decimal strings.</p>
      </div>
    </div>
    <p v-if="error" class="set-note error" role="alert">{{ error }}</p>
    <p v-if="message" class="set-note" role="status">{{ message }}</p>
    <div class="actions">
      <button type="button" class="btn sm primary" :disabled="busy || !name.trim()" @click="save">Save profile</button>
      <button v-if="selected && !selected.archived" type="button" class="btn sm" :disabled="busy || defaultID === selected.id" @click="setDefault">Use for new quotes</button>
      <button v-if="selected" type="button" class="btn sm ghost" :disabled="busy" @click="undo">Undo last change</button>
      <button v-if="selected && !selected.archived" type="button" class="btn sm ghost" :disabled="busy" @click="archive">Archive</button>
    </div>
  </SettingsCard>
</template>
<style scoped>
.profiles { display: grid; grid-template-columns: minmax(190px, 260px) minmax(0, 1fr); gap: 18px; }
.profile-list,.definition { display: grid; align-content: start; gap: 8px; min-width: 0; }
.profile-list > label:not(.btn),.definition > label { font-size: 12px; font-weight: 600; }
.json { width: 100%; resize: vertical; font: 12px/1.45 var(--mono); white-space: pre; }
.upload-row,.actions { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
.hint { font-size: 12px; color: var(--ink-3); line-height: 1.45; }
.visually-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; opacity: 0; }
@media (max-width: 700px) { .profiles { grid-template-columns: 1fr; } }
</style>
