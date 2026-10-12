<script setup lang="ts">
import type { Component } from 'vue'

withDefaults(defineProps<{
  variant?: 'default' | 'primary' | 'danger' | 'ghost'
  icon?: Component
  pressed?: boolean
  small?: boolean
}>(), { variant: 'default' })
</script>

<template>
  <span class="pm-btn" :class="[`pm-btn--${variant}`, { 'pm-btn--pressed': pressed, 'pm-btn--sm': small }]">
    <component :is="icon" v-if="icon" :size="14" />
    <slot />
  </span>
</template>

<style scoped>
.pm-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 30px;
  padding: 0 11px;
  border-radius: var(--pm-radius);
  background: var(--pm-surface);
  box-shadow: 0 0 0 1px var(--pm-border);
  color: var(--pm-head);
  font-size: 12.5px;
  font-weight: 500;
  white-space: nowrap;
  transition: transform 0.12s ease, opacity 0.12s ease;
}
.pm-btn--sm {
  height: 26px;
  padding: 0 9px;
  font-size: 12px;
}
.pm-btn--primary {
  background: var(--pm-primary);
  color: var(--pm-primary-fg);
  box-shadow: none;
}
.pm-btn--danger {
  background: var(--pm-bad-bg);
  color: var(--pm-bad);
  box-shadow: none;
}
.pm-btn--ghost {
  background: transparent;
  box-shadow: none;
  color: var(--pm-muted);
}
.pm-btn--pressed {
  transform: scale(0.94);
  opacity: 0.8;
}
</style>
