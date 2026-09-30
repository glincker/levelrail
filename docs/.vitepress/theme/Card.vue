<script setup lang="ts">
// Ported from unmint's card.tsx, minus its SVG icon map: pass an emoji
// (or nothing) into #icon instead.
withDefaults(
  defineProps<{
    title: string
    href?: string
  }>(),
  {},
)
</script>

<template>
  <component :is="href ? 'a' : 'div'" :href="href" class="vp-card">
    <div v-if="$slots.icon" class="vp-card__icon">
      <slot name="icon" />
    </div>
    <h3 class="vp-card__title">{{ title }}</h3>
    <div v-if="$slots.default" class="vp-card__body">
      <slot />
    </div>
  </component>
</template>
