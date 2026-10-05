<script setup lang="ts">
import { PhCaretRight } from '@phosphor-icons/vue'

defineProps<{ items: { q: string; a: string }[] }>()
</script>

<template>
  <div class="lp-faq">
    <details v-for="(f, i) in items" :key="f.q" class="lp-faq__item" :open="i === 0">
      <summary class="lp-faq__q">
        <span>{{ f.q }}</span>
        <PhCaretRight class="lp-faq__chev" weight="bold" aria-hidden="true" />
      </summary>
      <p class="lp-faq__a">{{ f.a }}</p>
    </details>
  </div>
</template>
