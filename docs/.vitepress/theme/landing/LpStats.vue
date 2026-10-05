<script setup lang="ts">
defineProps<{ stats: { value: string; label: string }[] }>()
</script>

<template>
  <section class="lp-stats">
    <div v-for="s in stats" :key="s.label" class="lp-stat">
      <span class="lp-stat__value">{{ s.value }}</span>
      <span class="lp-stat__label">{{ s.label }}</span>
    </div>
  </section>
</template>
