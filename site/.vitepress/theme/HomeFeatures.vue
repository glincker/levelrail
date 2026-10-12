<script setup lang="ts">
import LpCard from './landing/LpCard.vue'
import { homeFeatures } from './homeFeatures'
</script>

<template>
  <section class="lp-scope home-features" aria-label="Features">
    <div class="lp-cards home-features__grid">
      <LpCard v-for="(card, i) in homeFeatures" :key="card.title" v-reveal="i % 3" :card="card" />
    </div>
  </section>
</template>
