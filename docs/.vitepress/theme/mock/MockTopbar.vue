<script setup lang="ts">
import { PhBell, PhMagnifyingGlass, PhMonitor, PhQuestion, PhSidebarSimple, PhSparkle } from '@phosphor-icons/vue'
import MockStatusPill from './MockStatusPill.vue'
</script>

<template>
  <header class="pm-top">
    <PhSidebarSimple :size="17" class="pm-top__muted" />
    <span class="pm-top__search">
      <PhMagnifyingGlass :size="14" />
      <span>Search or jump to...</span>
      <kbd>⌘</kbd><kbd>K</kbd>
    </span>
    <span class="pm-top__right">
      <MockStatusPill status="healthy" label="All systems healthy" />
      <i><PhBell :size="15" /></i>
      <i><PhSparkle :size="15" /></i>
      <i><PhQuestion :size="15" /></i>
      <i><PhMonitor :size="15" /></i>
    </span>
  </header>
</template>

<style scoped>
.pm-top {
  flex: none;
  display: flex;
  align-items: center;
  gap: 18px;
  height: 56px;
  padding: 0 24px 0 16px;
  border-bottom: 1px solid var(--pm-border);
  background: var(--pm-surface);
}
.pm-top__muted { color: var(--pm-muted); }
.pm-top__search {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 256px;
  height: 30px;
  padding: 0 10px;
  border-radius: var(--pm-pill);
  box-shadow: 0 0 0 1px var(--pm-border);
  color: var(--pm-muted);
  font-size: 12.5px;
}
.pm-top__search span { flex: 1; }
kbd {
  padding: 0 5px;
  border-radius: 4px;
  background: var(--pm-sunken);
  box-shadow: 0 0 0 1px var(--pm-border);
  font-family: var(--pm-mono);
  font-size: 10px;
}
.pm-top__right { margin-left: auto; display: flex; align-items: center; gap: 8px; }
.pm-top__right i {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: var(--pm-radius);
  box-shadow: 0 0 0 1px var(--pm-border);
  color: var(--pm-head);
}
</style>
