<script setup lang="ts">
import { PhCursor } from '@phosphor-icons/vue'

defineProps<{ x: number; y: number; show: boolean; click: boolean }>()
</script>

<template>
  <div class="pm-cursor" :class="{ 'is-show': show, 'is-click': click }" :style="{ '--cx': `${x}px`, '--cy': `${y}px` }">
    <span class="pm-cursor__ripple"></span>
    <PhCursor :size="22" weight="fill" class="pm-cursor__icon" />
  </div>
</template>

<style scoped>
.pm-cursor {
  position: absolute;
  top: 0;
  left: 0;
  z-index: 5;
  opacity: 0;
  transform: translate3d(var(--cx), var(--cy), 0);
  transition: transform var(--pm-dur-glide) var(--pm-ease-glide), opacity var(--pm-dur-fade) ease;
  will-change: transform;
}
.pm-cursor.is-show { opacity: 1; }
.pm-cursor__icon { display: block; color: var(--pm-head); filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.45)); }
.pm-cursor__ripple {
  position: absolute;
  top: -11px;
  left: -11px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--pm-info);
  opacity: 0;
}
.pm-cursor.is-click .pm-cursor__ripple { animation: pm-ripple var(--pm-dur-ripple) ease-out; }
@keyframes pm-ripple {
  from { opacity: 0.45; transform: scale(0.3); }
  to { opacity: 0; transform: scale(1.8); }
}
</style>
