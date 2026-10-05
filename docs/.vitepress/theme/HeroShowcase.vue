<script setup lang="ts">
import ProductMock from './mock/ProductMock.vue'

defineProps<{
  src?: string
  mock?: 'apps' | 'overview' | 'deploys' | 'logs'
  alt: string
  url: string
}>()
</script>

<template>
  <div class="hero-showcase" aria-hidden="false">
    <div class="hero-showcase__glow" aria-hidden="true"></div>
    <div class="hero-showcase__frame" :class="{ 'hero-showcase__frame--mock': mock }">
      <ProductMock v-if="mock" :view="mock" theme="dark" animate :label="alt" />
      <template v-else>
        <div class="hero-showcase__chrome" aria-hidden="true">
          <span class="hero-showcase__dot"></span>
          <span class="hero-showcase__dot"></span>
          <span class="hero-showcase__dot"></span>
          <span class="hero-showcase__url">{{ url }}</span>
        </div>
        <img class="hero-showcase__img" :src="src" :alt="alt" width="1280" height="900" fetchpriority="high" />
      </template>
    </div>
  </div>
</template>
