<script setup lang="ts">
import { PhArrowRight, PhChartLineUp, PhGauge, PhMemory, PhStack } from '@phosphor-icons/vue'
import type { Component } from 'vue'
import CountUp from '../CountUp.vue'

defineProps<{
  stats: { value: string; label: string }[]
  link?: { text: string; href: string }
  caption?: string
}>()

function iconFor(s: { value: string; label: string }): Component {
  if (s.value.includes('%')) return PhGauge
  if (/\b(MB|RSS|RAM|memory)\b/i.test(`${s.value} ${s.label}`)) return PhMemory
  if (/\bapps?\b/i.test(s.label)) return PhStack
  return PhChartLineUp
}
</script>

<template>
  <section class="lp-stats-wrap">
    <p v-if="caption" v-reveal class="lp-stats__caption">{{ caption }}</p>
    <div class="lp-stats">
      <div v-for="(s, i) in stats" :key="s.label" v-reveal="i" class="lp-stat">
        <component :is="iconFor(s)" class="lp-stat__icon" :size="16" weight="duotone" aria-hidden="true" />
        <span class="lp-stat__value"><CountUp :value="s.value" :delay="i * 70" /></span>
        <span class="lp-stat__rule" aria-hidden="true"></span>
        <span class="lp-stat__label">{{ s.label }}</span>
      </div>
    </div>
    <a v-if="link" v-reveal="stats.length" class="lp-stats__link" :href="link.href">
      {{ link.text }}<PhArrowRight :size="14" weight="bold" aria-hidden="true" />
    </a>
  </section>
</template>
