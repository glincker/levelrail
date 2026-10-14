<script setup lang="ts">
import LpButton from './LpButton.vue'

defineProps<{
  heading: string
  sub: string
  primary: { text: string; link: string }
}>()
</script>

<template>
  <section v-reveal class="lp-cta">
    <h2 class="lp-cta__title">{{ heading }}</h2>
    <p class="lp-cta__sub">{{ sub }}</p>
    <div class="lp-actions">
      <LpButton :href="primary.link" icon="arrow">{{ primary.text }}</LpButton>
      <LpButton href="https://github.com/glincker/levelrail" variant="ghost" icon="github">Star on GitHub</LpButton>
    </div>
  </section>
</template>
