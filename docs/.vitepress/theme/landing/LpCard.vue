<script setup lang="ts">
import { PhArrowUpRight } from '@phosphor-icons/vue'
import { landingIcon } from './icons'
import type { LandingCard } from '../landingTypes'

defineProps<{ card: LandingCard }>()
</script>

<template>
  <article class="lp-card">
    <PhArrowUpRight v-if="card.link" class="lp-card__arrow" weight="bold" aria-hidden="true" />
    <span class="lp-card__icon"><component :is="landingIcon(card.icon)" weight="duotone" :size="22" /></span>
    <h3 class="lp-card__title">{{ card.title }}</h3>
    <p class="lp-card__body">{{ card.body }}</p>
    <div v-if="card.visual" class="lp-card__visual lp-card__visual--terminal" aria-hidden="true">
      <div v-for="(l, i) in card.visual" :key="i" class="lp-vline" :class="`lp-vline--${l.k}`">{{ l.t }}</div>
    </div>
    <ul v-else-if="card.routes" class="lp-card__visual lp-card__visual--routes" aria-hidden="true">
      <li v-for="r in card.routes" :key="r.method + r.path" class="lp-route">
        <span class="lp-route__tag">API</span>
        <span class="lp-route__path">{{ r.path }}</span>
        <span class="lp-route__method" :class="`lp-route__method--${r.method.toLowerCase()}`">{{ r.method }}</span>
      </li>
    </ul>
    <a v-if="card.link" class="lp-card__link" :href="card.link.href">{{ card.link.text }}</a>
  </article>
</template>
