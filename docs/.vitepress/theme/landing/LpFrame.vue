<script setup lang="ts">
defineProps<{
  src: string
  alt: string
  small?: boolean
  eager?: boolean
}>()
</script>

<template>
  <div class="lp-frame" :class="{ 'lp-frame--sm': small }">
    <img :src="src" :alt="alt" width="1280" height="800" :loading="eager ? 'eager' : 'lazy'" />
  </div>
</template>
