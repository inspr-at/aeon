<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import type { Handoff } from '../../lib/journey'
defineProps<{ handoff: Handoff }>()
</script>
<template><section class="j-card"><div class="eyebrow">{{ handoff.plugin_id }} · attempt {{ handoff.attempt }}</div><h2>{{ handoff.operation }} · {{ handoff.state }}</h2><dl class="j-kv"><dt>Authority epoch</dt><dd>{{ handoff.authority_epoch }}</dd><dt>Expires</dt><dd>{{ new Date(handoff.expires_at).toLocaleString() }}</dd><template v-if="handoff.result"><dt>Result</dt><dd>{{ handoff.result.outcome }}</dd><dt>Observed</dt><dd>{{ new Date(handoff.result.completed_at).toLocaleString() }}</dd><template v-if="handoff.result.blocker_code"><dt>Blocker</dt><dd class="error">{{ handoff.result.blocker_code.replaceAll('_',' ') }}</dd></template></template></dl><p v-if="!handoff.result" class="j-note">Waiting for verified terminal evidence.</p></section></template>
