<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { computed, ref } from 'vue'
import type { MergePreview, ConflictChoices } from '../../../lib/quoteMerge'
const props=defineProps<{ open:boolean; preview:MergePreview|null; durableRecovery:boolean }>()
const emit=defineEmits<{ resolve:[choices:ConflictChoices]; export:[]; discard:[]; close:[] }>()
const choices=ref<ConflictChoices>({})
const ready=computed(()=>!!props.preview?.document && props.preview.conflicts.every(c=>!!choices.value[c.path]))
</script>
<template>
  <div v-if="open" class="scrim" role="presentation" @click.self="emit('close')">
    <section role="dialog" aria-modal="true" aria-labelledby="quote-conflict-title" class="dialog" @keydown.esc="emit('close')">
      <h2 id="quote-conflict-title">Review quote changes</h2>
      <p>Your copy and the server copy are both preserved. Review each overlap before saving against the latest revision.</p>
      <p v-if="!durableRecovery" class="warning">Local recovery storage is unavailable. Export your copy before closing this tab.</p>
      <p v-if="preview?.conflicts.length===0">The changes are in separate fields. Preview the merged document before saving.</p>
      <ol v-if="preview?.conflicts.length" class="conflicts">
        <li v-for="conflict in preview.conflicts" :key="conflict.path">
          <strong>{{ conflict.path }}</strong><span>{{ conflict.reason }}</span>
          <fieldset><legend>Keep which change?</legend><label><input v-model="choices[conflict.path]" type="radio" :name="conflict.path" value="mine"> My copy</label><label><input v-model="choices[conflict.path]" type="radio" :name="conflict.path" value="theirs"> Server copy</label></fieldset>
        </li>
      </ol>
      <div class="actions"><button type="button" @click="emit('export')">Download my copy</button><button type="button" @click="emit('close')">Keep editing</button><button type="button" class="danger" @click="emit('discard')">Discard my changes and reload</button><button type="button" class="primary" :disabled="!ready" @click="emit('resolve',choices)">Save reviewed result</button></div>
    </section>
  </div>
</template>
<style scoped>
.scrim{position:fixed;inset:0;z-index:100;display:grid;place-items:center;padding:16px;background:var(--scrim)}.dialog{width:min(620px,100%);max-height:90vh;overflow:auto;padding:24px;border:1px solid var(--line-2);border-radius:14px;background:var(--surface-raised);color:var(--ink);box-shadow:var(--shadow-pop)}h2{margin:0 0 8px}p{color:var(--ink-2)}.warning{padding:10px;border-radius:8px;background:var(--danger-bg);color:var(--danger)}.conflicts{max-height:42vh;overflow:auto;padding-left:22px}.conflicts li{margin:12px 0;padding:10px;border:1px solid var(--line-2);border-radius:8px}.conflicts strong,.conflicts span{display:block}.conflicts span{font-size:12px;color:var(--ink-2)}fieldset{display:flex;gap:15px;border:0;padding:7px 0 0}legend{font-size:12px}.actions{display:flex;flex-wrap:wrap;justify-content:flex-end;gap:8px;margin-top:18px}button{padding:8px 12px;border:1px solid var(--line-2);border-radius:8px;background:var(--surface);color:var(--ink);cursor:pointer}button:focus-visible{outline:2px solid var(--teal);outline-offset:2px}button:disabled{opacity:.5;cursor:default}.primary{background:var(--teal);color:var(--surface)}.danger{color:var(--danger)}
@media print{.scrim{display:none}}
</style>
