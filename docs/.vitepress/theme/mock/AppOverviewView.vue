<script setup lang="ts">
import { PhActivity, PhArrowClockwise } from '@phosphor-icons/vue'
import MockAppHeader from './MockAppHeader.vue'
import MockAreaChart from './MockAreaChart.vue'
import MockButton from './MockButton.vue'
import { computed, ref, watch } from 'vue'
import { deployMarkers, timeRanges, xTicks } from './mockData'
import type { ChartMarker } from './MockAreaChart.vue'
import { useLiveCharts } from './mockLive'
import { useMotion } from './useMockLoop'

defineProps<{ animate?: boolean }>()

const motion = useMotion()
const { charts, advance } = useLiveCharts()
const STEP = 1 / 39
let nextId = deployMarkers.length
const markers = ref<ChartMarker[]>(deployMarkers.map((x, i) => ({ id: i, x })))
const view = computed(() => markers.value.map((m, i, all) => ({ ...m, old: i < all.length - 4 })))
const count = computed(() => motion?.deployCount.value ?? 4)

if (motion) {
  watch(motion.tick, () => {
    advance()
    markers.value = markers.value.map((m) => ({ ...m, x: m.x - STEP })).filter((m) => m.x > -0.05)
  })
  watch(motion.deployed, () => {
    markers.value = [...markers.value.slice(-5), { id: nextId++, x: 0.975, fresh: true }]
  })
}
</script>

<template>
  <div class="pm-ov">
    <MockAppHeader crumb="Metrics" />
    <section class="pm-ov__card">
      <div class="pm-ov__h"><PhActivity :size="15" />Metrics</div>
      <div class="pm-ov__note">Deploy frequency: {{ count }} in this range. Dashed lines mark real deploy attempts (green succeeded); restarts: 0 in this range.</div>
      <div class="pm-ov__ranges">
        <span class="pm-seg"><b class="on">{{ timeRanges[0] }}</b><b v-for="r in timeRanges.slice(1)" :key="r">{{ r }}</b></span>
        <MockButton :icon="PhArrowClockwise">Refresh</MockButton>
      </div>
      <div class="pm-ov__grid">
        <div v-for="c in charts" :key="c.id" class="pm-ov__chart">
          <div class="pm-ov__title">
            <i :class="`pm-tone--${c.tone}`"></i><b>{{ c.title }}</b><span>{{ c.value }}</span>
          </div>
          <MockAreaChart
            :series="c.series" :y-ticks="c.yTicks" :x-ticks="xTicks"
            :y-min="c.yMin" :y-max="c.yMax" :markers="view" :new-marker="animate" :tick="motion ? motion.tick.value : 0" :live="c.live"
            :width="416" :height="160"
          />
          <div class="pm-ov__legend">
            <span v-for="s in c.series" :key="s.name"><i :class="`pm-tone--${s.tone}`"></i>{{ s.name }}</span>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.pm-ov { flex: 1; min-width: 0; padding: 20px 24px; overflow: hidden; }
.pm-ov__card { padding: 18px 18px 8px; border-radius: 12px; background: var(--pm-surface); box-shadow: 0 0 0 1px var(--pm-border); }
.pm-ov__h { display: flex; align-items: center; gap: 8px; margin: 0; color: var(--pm-head); font-size: 14px; font-weight: 500; }
.pm-ov__note { margin: 6px 0 12px; color: var(--pm-muted); font-size: 12px; }
.pm-ov__ranges { display: flex; gap: 8px; margin-bottom: 14px; }
.pm-seg { display: flex; border-radius: var(--pm-radius); box-shadow: 0 0 0 1px var(--pm-border); overflow: hidden; }
.pm-seg b { padding: 6px 11px; color: var(--pm-muted); font-size: 12px; font-weight: 400; }
.pm-seg b.on { background: var(--pm-primary); color: var(--pm-primary-fg); }
.pm-ov__grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.pm-ov__chart { padding: 14px 14px 8px; border-radius: 10px; box-shadow: 0 0 0 1px var(--pm-border); }
.pm-ov__title { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.pm-ov__title b { color: var(--pm-head); font-size: 13px; font-weight: 500; }
.pm-ov__title span { margin-left: auto; font-family: var(--pm-mono); font-size: 12px; color: var(--pm-head); }
.pm-ov__title i, .pm-ov__legend i { display: inline-block; width: 7px; height: 7px; border-radius: 50%; background: var(--pm-tone); }
.pm-ov__legend { display: flex; justify-content: center; gap: 16px; margin-top: 2px; color: var(--pm-muted); font-size: 11px; }
.pm-ov__legend span { display: inline-flex; align-items: center; gap: 5px; }
.pm-tone--blue { --pm-tone: var(--pm-c-blue); }
.pm-tone--green { --pm-tone: var(--pm-c-green); }
.pm-tone--purple { --pm-tone: var(--pm-c-purple); }
.pm-tone--orange { --pm-tone: var(--pm-c-orange); }
</style>
