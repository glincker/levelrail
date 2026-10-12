<script setup lang="ts">
withDefaults(
  defineProps<{
    cols?: number
  }>(),
  { cols: 2 },
)
</script>

<template>
  <div class="vp-card-group" :style="{ '--vp-card-group-cols': cols }">
    <slot />
  </div>
</template>
