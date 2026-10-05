<script setup lang="ts">
defineProps<{ items: { q: string; a: string }[] }>()
</script>

<template>
  <div class="lp-faq">
    <details v-for="(f, i) in items" :key="f.q" class="lp-faq__item" :open="i === 0">
      <summary>{{ f.q }}</summary>
      <p>{{ f.a }}</p>
    </details>
  </div>
</template>
