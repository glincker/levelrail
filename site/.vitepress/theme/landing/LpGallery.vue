<script setup lang="ts">
import LpFrame from './LpFrame.vue'

defineProps<{
  items: { src: string; alt: string; caption: string; url?: string }[]
  eager?: boolean
}>()
</script>

<template>
  <div class="lp-gallery">
    <figure v-for="(g, i) in items" :key="g.src" v-reveal="i % 2" class="lp-figure">
      <LpFrame :src="g.src" :alt="g.alt" :url="g.url" :eager="eager" small />
      <figcaption>{{ g.caption }}</figcaption>
    </figure>
  </div>
</template>
