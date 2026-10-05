<script setup lang="ts">
defineProps<{ lines: string[]; context: string }>()
</script>

<template>
  <section class="lp-statement">
    <p class="lp-statement__text"><template v-for="(l, i) in lines" :key="l">{{ l }}<br v-if="i < lines.length - 1" /></template></p>
    <p class="lp-statement__context">{{ context }}</p>
  </section>
</template>
