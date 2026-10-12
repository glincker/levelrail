<script setup lang="ts">
import type { LandingData } from '../landingTypes'

defineProps<{ compare: NonNullable<LandingData['compare']> }>()
</script>

<template>
  <div v-reveal class="lp-table" role="table" :aria-label="compare.heading">
    <div class="lp-row lp-row--head" role="row">
      <span class="lp-cell lp-cell--label" role="columnheader"></span>
      <span class="lp-cell" role="columnheader">{{ compare.left }}</span>
      <span class="lp-cell lp-cell--us" role="columnheader">{{ compare.right }}</span>
    </div>
    <div v-for="r in compare.rows" :key="r.label" class="lp-row" role="row">
      <span class="lp-cell lp-cell--label" role="rowheader">{{ r.label }}</span>
      <span class="lp-cell" role="cell">{{ r.left }}</span>
      <span class="lp-cell lp-cell--us" role="cell">{{ r.right }}</span>
    </div>
  </div>
  <a v-if="compare.more" class="lp-more" :href="compare.more.href">{{ compare.more.text }}</a>
</template>
