<script setup lang="ts">
defineProps<{
  title?: string
}>()
</script>

<template>
  <div class="vp-step">
    <h4 v-if="title" class="vp-step__title">{{ title }}</h4>
    <div class="vp-step__body">
      <slot />
    </div>
  </div>
</template>
