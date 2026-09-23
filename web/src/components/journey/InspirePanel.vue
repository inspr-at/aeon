<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import AithemaPanel from './AithemaPanel.vue'
import { useJourney } from '../../stores/journey'
defineProps<{ canEdit: boolean }>()
const store = useJourney()
</script>
<template><div class="j-columns one"><div class="j-scroll"><AithemaPanel :intake="store.intake" :busy="store.busy || store.stale" :can-edit="canEdit" @accept="store.accept"><slot /></AithemaPanel></div></div></template>
