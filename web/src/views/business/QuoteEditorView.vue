<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { useRouter } from 'vue-router'
import QuoteWorkspace from '../../components/business/QuoteWorkspace.vue'

// A quote on its own page. The workspace is the same one the Quotes list docks
// beside itself; "Show beside the list" goes back there with the same session,
// unsaved work and undo history.
const props = defineProps<{ quoteId: string }>()
const router = useRouter()
function collapse() { void router.push({ path: '/business/quotes', query: { quote: props.quoteId } }) }
function open(id: string) { void router.push(`/business/quotes/${encodeURIComponent(id)}`) }
</script>

<template>
  <QuoteWorkspace :quote-id="quoteId" layout="full" class="quote-editor-page" @collapse="collapse" @open="open" />
</template>
