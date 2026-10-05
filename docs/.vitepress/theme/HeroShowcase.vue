<script setup lang="ts">
defineProps<{ src: string; alt: string; url: string }>()
</script>

<template>
  <div class="hero-showcase" aria-hidden="false">
    <div class="hero-showcase__glow" aria-hidden="true"></div>
    <div class="hero-showcase__frame">
      <div class="hero-showcase__chrome" aria-hidden="true">
        <span class="hero-showcase__dot"></span>
        <span class="hero-showcase__dot"></span>
        <span class="hero-showcase__dot"></span>
        <span class="hero-showcase__url">{{ url }}</span>
      </div>
      <img class="hero-showcase__img" :src="src" :alt="alt" width="1280" height="900" fetchpriority="high" />
    </div>
  </div>
</template>
