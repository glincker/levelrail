<script setup lang="ts">
defineProps<{ steps: { title: string; body: string }[] }>()
</script>

<template>
  <ol class="lp-steps">
    <li v-for="(s, i) in steps" :key="s.title" v-reveal="i" class="lp-step">
      <span class="lp-step__n">{{ i + 1 }}</span>
      <div>
        <h3 class="lp-step__title">{{ s.title }}</h3>
        <p class="lp-step__body">{{ s.body }}</p>
      </div>
    </li>
  </ol>
</template>
