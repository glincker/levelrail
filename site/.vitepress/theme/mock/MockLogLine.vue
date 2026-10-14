<script setup lang="ts">
import type { MockLog } from './mockData'

defineProps<{ line: MockLog }>()
</script>

<template>
  <div class="pm-log" :class="`pm-log--${line.level}`">
    <span class="pm-log__time">{{ line.time }}</span>
    <span class="pm-log__text">{{ line.text }}</span>
  </div>
</template>

<style scoped>
.pm-log {
  display: flex;
  gap: 16px;
  height: 20px;
  align-items: center;
  font-family: var(--pm-mono);
  font-size: 11.5px;
  color: var(--pm-console-text);
  white-space: nowrap;
}
.pm-log__time { flex: none; color: #7c7f86; }
.pm-log--debug .pm-log__text { color: #7c7f86; }
.pm-log--info .pm-log__text:not(:empty) { overflow: hidden; text-overflow: ellipsis; }
</style>
