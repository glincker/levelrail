<script setup lang="ts">
import { computed } from 'vue'
import { PhArrowRight } from '@phosphor-icons/vue'
import { data as releases } from '../../changelog/releases.data.mts'

const latest = computed(() => releases[0])
</script>

<template>
  <a class="announce-pill" href="/changelog/">
    <span class="announce-pill__tag">{{ latest ? latest.tag : 'Pre-release' }}</span>
    <span class="announce-pill__text">See what's new</span>
    <PhArrowRight class="announce-pill__icon" weight="bold" aria-hidden="true" />
  </a>
</template>
