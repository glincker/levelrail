<script setup lang="ts">
import { computed } from 'vue'
import type { MockStatus } from './mockData'

const props = withDefaults(defineProps<{ status: MockStatus; label?: string; plain?: boolean }>(), { plain: false })

const text = computed(() => props.label ?? { healthy: 'Healthy', running: 'Running', deploying: 'Deploying', failed: 'Failed', idle: 'Idle' }[props.status])
const tone = computed(() => ({ healthy: 'ok', running: 'ok', deploying: 'warn', failed: 'bad', idle: 'idle' }[props.status]))
</script>

<template>
  <span class="pm-pill" :class="[`pm-pill--${tone}`, { 'pm-pill--plain': plain }]">
    <i class="pm-pill__dot"></i>{{ text }}
  </span>
</template>

<style scoped>
.pm-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 9px;
  border-radius: var(--pm-pill);
  font-size: 12px;
  font-weight: 500;
  line-height: 1.4;
  white-space: nowrap;
}
.pm-pill__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}
.pm-pill--ok { color: var(--pm-ok); background: var(--pm-ok-bg); }
.pm-pill--warn { color: var(--pm-warn); background: var(--pm-warn-bg); }
.pm-pill--idle { color: var(--pm-muted); background: var(--pm-sunken); }
.pm-pill--bad { color: var(--pm-bad); background: var(--pm-bad-bg); }
.pm-pill--plain { background: none; padding: 0; color: var(--pm-head); }
.pm-pill--plain .pm-pill__dot { background: var(--pm-ok); }
</style>
