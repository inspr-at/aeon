<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import AppIcon from '../components/AppIcon.vue'
</script>

<template>
  <section class="center-stage" aria-labelledby="not-found-title">
    <div class="not-found">
      <p class="eyebrow">404 / Page not found</p>
      <h1 id="not-found-title">A little off the path.</h1>
      <p>This page isn’t here. Your workspace is just a step away.</p>
      <RouterLink class="button" to="/">Back to workspace<AppIcon name="arrow" /></RouterLink>
    </div>
  </section>
</template>

<style scoped>
.not-found { max-width: 480px; text-align: center; }
h1 { margin: 16px 0; }
.button { margin-top: 28px; }
</style>
