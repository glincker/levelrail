<script setup lang="ts">
defineProps<{
  heading?: string
  lead?: string
  narrow?: boolean
}>()
</script>

<template>
  <section class="lp-section" :class="{ 'lp-section--narrow': narrow }">
    <h2 v-if="heading" v-reveal class="lp-h2">{{ heading }}</h2>
    <p v-if="lead" v-reveal="1" class="lp-lead">{{ lead }}</p>
    <slot />
  </section>
</template>
