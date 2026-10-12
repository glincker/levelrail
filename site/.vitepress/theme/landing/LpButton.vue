<script setup lang="ts">
import { PhArrowRight, PhGithubLogo } from '@phosphor-icons/vue'

withDefaults(
  defineProps<{
    href: string
    variant?: 'primary' | 'ghost'
    icon?: 'arrow' | 'github' | 'none'
  }>(),
  { variant: 'primary', icon: 'none' },
)

const isExternal = (href: string) => href.startsWith('http')
</script>

<template>
  <a
    class="lp-btn"
    :class="`lp-btn--${variant}`"
    :href="href"
    :target="isExternal(href) ? '_blank' : undefined"
    :rel="isExternal(href) ? 'noreferrer' : undefined"
  >
    <PhGithubLogo v-if="icon === 'github'" weight="fill" class="lp-btn__icon" />
    <slot />
    <PhArrowRight v-if="icon === 'arrow'" weight="bold" class="lp-btn__icon" />
  </a>
</template>
