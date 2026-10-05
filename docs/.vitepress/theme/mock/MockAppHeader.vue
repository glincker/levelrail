<script setup lang="ts">
import { PhArrowClockwise, PhCopy, PhHouse, PhRocketLaunch, PhStop, PhTrash, PhCaretRight } from '@phosphor-icons/vue'
import MockButton from './MockButton.vue'
import MockStatusPill from './MockStatusPill.vue'
import { activeApp } from './mockData'

defineProps<{ crumb: string }>()
</script>

<template>
  <div class="pm-ah">
    <div class="pm-ah__crumbs">
      <PhHouse :size="14" /><PhCaretRight :size="10" />{{ activeApp }}<PhCaretRight :size="10" /><b>{{ crumb }}</b>
    </div>
    <div class="pm-ah__row">
      <div class="pm-ah__name">{{ activeApp }}</div>
      <MockStatusPill status="healthy" />
      <span class="pm-ah__actions">
        <MockButton :icon="PhStop">Stop</MockButton>
        <MockButton :icon="PhArrowClockwise">Restart</MockButton>
        <MockButton :icon="PhRocketLaunch">Redeploy</MockButton>
        <MockButton :icon="PhCopy">Clone</MockButton>
        <MockButton variant="danger" :icon="PhTrash">Delete</MockButton>
      </span>
    </div>
  </div>
</template>

<style scoped>
.pm-ah__crumbs {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 18px;
  color: var(--pm-muted);
  font-size: 12.5px;
}
.pm-ah__crumbs b { color: var(--pm-head); font-weight: 500; }
.pm-ah__row { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; }
.pm-ah__name { color: var(--pm-head); font-size: 20px; font-weight: 600; letter-spacing: -0.01em; }
.pm-ah__actions { margin-left: auto; display: flex; gap: 8px; }
</style>
