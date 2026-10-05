<script setup lang="ts">
import LpFrame from './LpFrame.vue'

defineProps<{ items: { src: string; alt: string; caption: string }[] }>()
</script>

<template>
  <div class="lp-gallery">
    <figure v-for="g in items" :key="g.src" class="lp-figure">
      <LpFrame :src="g.src" :alt="g.alt" small />
      <figcaption>{{ g.caption }}</figcaption>
    </figure>
  </div>
</template>
