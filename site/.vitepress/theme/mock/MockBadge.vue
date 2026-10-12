<script setup lang="ts">
withDefaults(defineProps<{
  tone?: 'neutral' | 'ok' | 'info'
  mono?: boolean
  globe?: boolean
}>(), { tone: 'neutral' })
</script>

<template>
  <span class="pm-badge" :class="[`pm-badge--${tone}`, { 'pm-badge--mono': mono }]"><slot /></span>
</template>

<style scoped>
.pm-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 1px 8px;
  border-radius: var(--pm-pill);
  background: var(--pm-sunken);
  box-shadow: 0 0 0 1px var(--pm-border);
  color: var(--pm-muted);
  font-size: 11px;
  line-height: 1.5;
  white-space: nowrap;
}
.pm-badge--ok {
  background: var(--pm-ok-bg);
  color: var(--pm-ok);
  box-shadow: none;
}
.pm-badge--info {
  background: var(--pm-info-bg);
  color: var(--pm-info);
  box-shadow: none;
}
.pm-badge--mono {
  font-family: var(--pm-mono);
  font-size: 11px;
}
</style>
