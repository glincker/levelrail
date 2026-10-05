<script setup lang="ts">
import { landingIcon } from './icons'
import type { LandingCard } from '../landingTypes'

defineProps<{ card: LandingCard }>()
</script>

<template>
  <article class="lp-card">
    <span class="lp-card__icon"><component :is="landingIcon(card.icon)" weight="duotone" :size="22" /></span>
    <h3 class="lp-card__title">{{ card.title }}</h3>
    <p class="lp-card__body">{{ card.body }}</p>
    <div v-if="card.visual" class="lp-card__visual" aria-hidden="true">
      <div v-for="(l, i) in card.visual" :key="i" class="lp-vline" :class="`lp-vline--${l.k}`">{{ l.t }}</div>
    </div>
    <a v-if="card.link" class="lp-card__link" :href="card.link.href">{{ card.link.text }}</a>
  </article>
</template>
