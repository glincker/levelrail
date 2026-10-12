<script setup lang="ts">
import { PhArrowUpRight } from '@phosphor-icons/vue'
import { navIcons } from './navIcons'
import type { NavItem } from './navItems'

defineProps<{ item: NavItem }>()
defineEmits<{ navigate: [] }>()
</script>

<template>
  <a
    class="nav-item"
    data-nav-item
    :href="item.link"
    :target="item.external ? '_blank' : undefined"
    :rel="item.external ? 'noopener noreferrer' : undefined"
    @click="$emit('navigate')"
  >
    <span class="nav-item__icon" aria-hidden="true">
      <component :is="navIcons[item.icon]" :size="18" weight="duotone" />
    </span>
    <span class="nav-item__body">
      <span class="nav-item__title">
        {{ item.text }}
        <PhArrowUpRight v-if="item.external" class="nav-item__ext" :size="12" weight="bold" aria-hidden="true" />
        <span v-if="item.external" class="nav-item__sr">(opens in a new tab)</span>
      </span>
      <span class="nav-item__desc">{{ item.description }}</span>
    </span>
  </a>
</template>
