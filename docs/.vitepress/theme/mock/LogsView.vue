<script setup lang="ts">
import { PhArrowsOutSimple, PhCopy, PhDownloadSimple, PhMagnifyingGlass, PhScroll, PhTerminal } from '@phosphor-icons/vue'
import MockAppHeader from './MockAppHeader.vue'
import MockButton from './MockButton.vue'
import MockLogLine from './MockLogLine.vue'
import { logs } from './mockData'

defineProps<{ animate?: boolean }>()
const levels = ['All', 'Errors', 'Warnings', 'Info', 'Debug']
</script>

<template>
  <div class="pm-lg">
    <MockAppHeader crumb="Logs" />
    <div class="pm-lg__tabs">
      <b class="on"><PhTerminal :size="14" />Live</b><b><PhMagnifyingGlass :size="14" />Search</b><b><PhScroll :size="14" />Archive</b>
    </div>
    <div class="pm-lg__meta"><span>{{ logs.length }} lines (recent context, then live)</span><em><i class="pm-live-dot"></i>Live</em></div>
    <div class="pm-lg__bar">
      <span class="pm-lg__search"><PhMagnifyingGlass :size="14" />Filter lines</span>
      <b v-for="(l, i) in levels" :key="l" :class="{ on: i === 0 }">{{ l }}</b>
      <MockButton :icon="PhCopy">Copy</MockButton>
      <MockButton :icon="PhDownloadSimple">Download</MockButton>
    </div>
    <div class="pm-lg__console">
      <PhArrowsOutSimple :size="13" class="pm-lg__expand" />
      <MockLogLine v-for="(l, i) in logs" :key="i" :line="l" />
    </div>
  </div>
</template>

<style scoped>
.pm-lg { flex: 1; min-width: 0; padding: 20px 24px 0; overflow: hidden; }
.pm-lg__tabs { display: inline-flex; gap: 2px; padding: 3px; margin-bottom: 14px; border-radius: 8px; background: var(--pm-sunken); }
.pm-lg__tabs b { display: inline-flex; align-items: center; gap: 6px; padding: 5px 11px; border-radius: 6px; color: var(--pm-muted); font-size: 12.5px; font-weight: 500; }
.pm-lg__tabs b.on { background: var(--pm-surface); color: var(--pm-head); box-shadow: 0 0 0 1px var(--pm-border); }
.pm-lg__meta { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; color: var(--pm-muted); font-size: 12px; }
.pm-lg__meta em { display: inline-flex; align-items: center; gap: 6px; padding: 2px 10px; border-radius: var(--pm-pill); background: var(--pm-ok-bg); color: var(--pm-ok); font-style: normal; font-size: 12px; }
.pm-live-dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.pm-lg__bar { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; }
.pm-lg__bar b { padding: 4px 11px; border-radius: var(--pm-pill); box-shadow: 0 0 0 1px var(--pm-border); color: var(--pm-muted); font-size: 12px; font-weight: 400; }
.pm-lg__bar b.on { background: var(--pm-primary); color: var(--pm-primary-fg); box-shadow: none; }
.pm-lg__search { display: flex; align-items: center; gap: 8px; flex: 1; height: 30px; padding: 0 10px; border-radius: var(--pm-radius); box-shadow: 0 0 0 1px var(--pm-border); background: var(--pm-surface); color: var(--pm-muted); font-size: 12.5px; }
.pm-lg__console { position: relative; height: 440px; padding: 14px 20px; border-radius: 10px 10px 0 0; background: var(--pm-console); overflow: hidden; box-shadow: 0 0 0 1px var(--pm-border); }
.pm-lg__expand { position: absolute; top: 14px; right: 14px; color: #7c7f86; }
</style>
