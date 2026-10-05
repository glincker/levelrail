<script setup lang="ts">
import ProductMock from '../mock/ProductMock.vue'

defineProps<{
  src?: string
  mock?: 'apps' | 'overview' | 'deploys' | 'logs'
  alt: string
  small?: boolean
  eager?: boolean
}>()
</script>

<template>
  <div class="lp-frame" :class="{ 'lp-frame--sm': small, 'lp-frame--mock': mock }">
    <ProductMock v-if="mock" :view="mock" theme="auto" animate :label="alt" />
    <img v-else :src="src" :alt="alt" width="1280" height="800" :loading="eager ? 'eager' : 'lazy'" />
  </div>
</template>
