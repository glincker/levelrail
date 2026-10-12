<script setup lang="ts">
defineProps<{ url: string }>()
</script>

<template>
  <div class="pm-win">
    <div class="pm-win__bar">
      <span class="pm-win__dots"><i></i><i></i><i></i></span>
      <span class="pm-win__url">{{ url }}</span>
    </div>
    <div class="pm-win__body"><slot /></div>
  </div>
</template>

<style scoped>
.pm-win {
  width: 1280px;
  height: 800px;
  display: flex;
  flex-direction: column;
  background: var(--pm-paper);
  border-radius: 10px;
  box-shadow: var(--pm-frame);
  overflow: hidden;
}
.pm-win__bar {
  position: relative;
  flex: none;
  height: 38px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--pm-chrome);
  border-bottom: 1px solid var(--pm-border);
}
.pm-win__dots {
  position: absolute;
  left: 14px;
  display: flex;
  gap: 7px;
}
.pm-win__dots i {
  width: 11px;
  height: 11px;
  border-radius: 50%;
  background: var(--pm-border);
  box-shadow: inset 0 0 0 1px rgba(128, 128, 128, 0.25);
}
.pm-win__url {
  min-width: 320px;
  padding: 4px 16px;
  border-radius: var(--pm-pill);
  background: var(--pm-surface);
  box-shadow: 0 0 0 1px var(--pm-border);
  color: var(--pm-muted);
  font-size: 12px;
  text-align: center;
}
.pm-win__body {
  flex: 1;
  min-height: 0;
  display: flex;
}
</style>
