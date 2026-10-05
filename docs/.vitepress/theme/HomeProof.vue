<script setup lang="ts">
import LpStats from './landing/LpStats.vue'
import LpCard from './landing/LpCard.vue'
import { pathCards, proofLink, proofStats } from './homeData'
</script>

<template>
  <div class="lp-scope home-proof">
    <LpStats :stats="proofStats" :link="proofLink" />
    <section class="home-paths" aria-label="Choose your path">
      <h2 class="lp-h2 home-paths__title">Choose your path</h2>
      <div class="lp-cards home-paths__grid">
        <LpCard v-for="c in pathCards" :key="c.title" :card="c" />
      </div>
    </section>
  </div>
</template>
