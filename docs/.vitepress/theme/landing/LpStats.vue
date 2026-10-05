<script setup lang="ts">
defineProps<{
  stats: { value: string; label: string }[]
  link?: { text: string; href: string }
}>()
</script>

<template>
  <section class="lp-stats-wrap">
    <div class="lp-stats">
      <div v-for="s in stats" :key="s.label" class="lp-stat">
        <span class="lp-stat__value">{{ s.value }}</span>
        <span class="lp-stat__label">{{ s.label }}</span>
      </div>
    </div>
    <a v-if="link" class="lp-more lp-stats__link" :href="link.href">{{ link.text }}</a>
  </section>
</template>
