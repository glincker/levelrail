<script setup lang="ts">
defineProps<{
  src: string
  alt: string
  small?: boolean
  eager?: boolean
  url?: string
}>()
</script>

<template>
  <div class="lp-frame" :class="{ 'lp-frame--sm': small }">
    <div v-if="url" class="lp-frame__chrome" aria-hidden="true">
      <span class="lp-frame__dot"></span><span class="lp-frame__dot"></span><span class="lp-frame__dot"></span>
      <span class="lp-frame__url">{{ url }}</span>
    </div>
    <img :src="src" :alt="alt" width="1280" height="800" :loading="eager ? 'eager' : 'lazy'" />
  </div>
</template>
