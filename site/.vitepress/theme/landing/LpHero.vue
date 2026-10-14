<script setup lang="ts">
import LpButton from './LpButton.vue'
import type { LandingData } from '../landingTypes'

defineProps<{ d: LandingData }>()
</script>

<template>
  <section class="lp-hero">
    <p v-if="d.eyebrow" class="lp-eyebrow">{{ d.eyebrow }}</p>
    <h1 class="lp-title">{{ d.headline }}</h1>
    <p class="lp-sub">{{ d.sub }}</p>
    <div class="lp-actions">
      <LpButton :href="d.primary.link" icon="arrow">{{ d.primary.text }}</LpButton>
      <LpButton
        v-if="d.secondary"
        :href="d.secondary.link"
        variant="ghost"
        :icon="d.secondary.link.includes('github.com') ? 'github' : 'none'"
      >{{ d.secondary.text }}</LpButton>
    </div>
    <p v-if="d.install" class="lp-install"><code>{{ d.install }}</code></p>
  </section>
</template>
