<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import type { SessionView } from '../../../lib/quoteSession'
import type { PresenceSnapshot } from '../../../lib/quotePresence'
import { collaboratorColor } from '../../../lib/quotePresence'

const props=defineProps<{ view:SessionView; presence:PresenceSnapshot|null; principalId:string; actorName?:string }>()
const emit=defineEmits<{ reload:[]; review:[]; retry:[] }>()
const open=ref(false)
const people=computed(()=>{const map=new Map<string,{id:string;name:string;mode:string;sessions:number}>()
  for(const s of props.presence?.sessions??[]){if(s.principal_id===props.principalId)continue;const old=map.get(s.principal_id);if(old){old.sessions++;if(s.mode==='editing')old.mode='editing'}else map.set(s.principal_id,{id:s.principal_id,name:s.name,mode:s.mode,sessions:1})}
  return [...map.values()]})
const saveLabel=computed(()=>({loading:'Loading',clean:'Saved',dirty:'Unsaved changes',saving:'Saving',offline:'Not saved — offline',failed:'Not saved',conflict:'Save conflict','read-only':'Read only'}[props.view.local]))
</script>
<template>
  <div class="collab-bar" aria-label="Quote collaboration status">
    <div class="save" role="status" aria-live="polite">
      <span>{{ saveLabel }}</span>
      <template v-if="view.remote==='newer'">
        <span class="remote">Changed by {{ actorName || 'another editor' }}</span>
        <button type="button" @click="view.local==='clean' ? emit('reload') : emit('review')">{{ view.local==='clean' ? 'Update' : 'Review changes' }}</button>
      </template>
      <span v-else-if="view.remote==='unavailable'">Server status unavailable</span>
      <button v-if="view.local==='failed'||view.local==='offline'" type="button" @click="emit('retry')">Retry save</button>
    </div>
    <div class="people-wrap">
      <button type="button" class="people-toggle" :aria-expanded="open" aria-controls="quote-collaborators" @click="open=!open">
        <span v-for="person in people.slice(0,3)" :key="person.id" class="avatar" :style="{backgroundColor:collaboratorColor(person.id)}" aria-hidden="true">{{ person.name.trim().slice(0,1).toUpperCase() }}</span>
        <span>{{ people.length ? `${people.length} collaborators` : 'Only you here' }}</span>
      </button>
      <ul v-if="open" id="quote-collaborators" class="people" aria-label="Collaborators">
        <li v-for="person in people" :key="person.id"><span class="avatar" :style="{backgroundColor:collaboratorColor(person.id)}" aria-hidden="true">{{ person.name.trim().slice(0,1).toUpperCase() }}</span><span>{{ person.name }} · {{ person.mode }}<span v-if="person.sessions>1"> · {{ person.sessions }} tabs</span></span></li>
        <li v-if="!people.length">No other collaborators are present.</li>
      </ul>
    </div>
  </div>
</template>
<style scoped>
.collab-bar{display:flex;align-items:center;justify-content:space-between;gap:12px;color:var(--ink-2);font-size:12px}
.save{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.save button,.people-toggle{border:1px solid var(--line-2);border-radius:8px;background:var(--surface-raised);color:var(--ink);padding:5px 8px;cursor:pointer}.save button:focus-visible,.people-toggle:focus-visible{outline:2px solid var(--teal);outline-offset:2px}
.remote{font-weight:650;color:var(--gold-ink)}.people-wrap{position:relative}.people-toggle{display:flex;align-items:center;gap:5px}.avatar{display:inline-grid;place-items:center;flex:none;width:22px;height:22px;border-radius:50%;color:var(--surface);font-weight:700;font-size:11px}.people{position:absolute;right:0;top:100%;z-index:20;min-width:220px;padding:8px;margin:4px 0 0;list-style:none;border:1px solid var(--line-2);border-radius:10px;background:var(--surface-raised);box-shadow:var(--shadow-pop)}.people li{display:flex;align-items:center;gap:8px;padding:5px;white-space:nowrap}
@media print{.collab-bar{display:none}}
</style>
