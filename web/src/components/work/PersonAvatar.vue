<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import Avatar from '../Avatar.vue'
// A person in a chip, a row or a menu: the shared Avatar (their picture, else
// their initials on their colour).
withDefaults(defineProps<{ name: string; id?: string | null; size?: number }>(), { id: null, size: 20 })
</script>

<template>
  <Avatar :id="id" :name="name" :size="size" />
</template>
