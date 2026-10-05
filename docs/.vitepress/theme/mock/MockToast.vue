<script setup lang="ts">
import { PhCheckCircle } from '@phosphor-icons/vue'

defineProps<{ show: boolean }>()
</script>

<template>
  <div class="pm-toast" :class="{ 'is-show': show }">
    <PhCheckCircle :size="16" weight="fill" class="pm-toast__icon" />
    <span><b>Deploy a91d3e4 is live</b><em>marketing-site passed its readiness check</em></span>
  </div>
</template>

<style scoped>
.pm-toast {
  position: absolute;
  top: 54px;
  right: 24px;
  z-index: 6;
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 11px 16px 11px 12px;
  border-radius: 10px;
  background: var(--pm-surface);
  box-shadow: 0 0 0 1px var(--pm-border), 0 10px 30px rgba(0, 0, 0, 0.35);
  opacity: 0;
  transform: translate3d(0, -10px, 0);
  transition: opacity var(--pm-dur-toast) ease, transform var(--pm-dur-toast) var(--pm-ease-glide);
}
.pm-toast.is-show { opacity: 1; transform: none; }
.pm-toast__icon { flex: none; margin-top: 1px; color: var(--pm-ok); }
.pm-toast b { display: block; color: var(--pm-head); font-size: 13px; font-weight: 500; }
.pm-toast em { display: block; margin-top: 2px; color: var(--pm-muted); font-size: 11.5px; font-style: normal; }
</style>
