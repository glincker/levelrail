<script setup lang="ts">
import { computed } from 'vue'
import { smoothPath, toPoints } from './chartPath'
import type { MockTone } from './mockData'

const props = withDefaults(defineProps<{
  series: { name: string; tone: MockTone; values: number[]; area?: boolean }[]
  yTicks: string[]
  xTicks: string[]
  yMin: number
  yMax: number
  markers?: number[]
  newMarker?: boolean
  width?: number
  height?: number
}>(), { width: 440, height: 190 })

const L = 48
const R = 8
const T = 8
const B = 24

const plotBottom = computed(() => props.height - B)
const lines = computed(() => props.series.map((s) => {
  const pts = toPoints(s.values, L, props.width - R, T, plotBottom.value, props.yMin, props.yMax)
  const d = smoothPath(pts)
  return { ...s, d, area: s.area ? `${d} L${props.width - R} ${plotBottom.value} L${L} ${plotBottom.value} Z` : '' }
}))
const gridY = computed(() => props.yTicks.map((t, i) => ({
  t,
  y: T + (i / (props.yTicks.length - 1)) * (plotBottom.value - T),
})))
const gridX = computed(() => props.xTicks.map((t, i) => ({
  t,
  x: L + (i / (props.xTicks.length - 1)) * (props.width - R - L),
})))
const marks = computed(() => (props.markers ?? []).map((m, i, all) => ({
  x: L + m * (props.width - R - L),
  fresh: !!props.newMarker && i === all.length - 1,
})))
</script>

<template>
  <svg class="pm-chart" :width="width" :height="height" :viewBox="`0 0 ${width} ${height}`">
    <g class="pm-chart__grid">
      <line v-for="g in gridY" :key="g.t" :x1="L" :x2="width - R" :y1="g.y" :y2="g.y" />
      <line v-for="g in gridX" :key="g.t" :x1="g.x" :x2="g.x" :y1="T" :y2="plotBottom" />
    </g>
    <g class="pm-chart__ticks">
      <text v-for="g in gridY" :key="g.t" :x="L - 7" :y="g.y + 3.5" text-anchor="end">{{ g.t }}</text>
      <text v-for="(g, i) in gridX" :key="g.t" :x="g.x" :y="height - 6" :text-anchor="i === 0 ? 'start' : i === gridX.length - 1 ? 'end' : 'middle'">{{ g.t }}</text>
    </g>
    <g v-for="s in lines" :key="s.name" :class="`pm-tone--${s.tone}`">
      <path v-if="s.area" class="pm-chart__area pm-fade" :d="s.area" />
      <path class="pm-chart__line pm-draw" :d="s.d" pathLength="1" />
    </g>
    <line
      v-for="(m, i) in marks"
      :key="i"
      class="pm-chart__marker"
      :class="{ 'pm-marker--new': m.fresh }"
      :x1="m.x" :x2="m.x" :y1="T" :y2="plotBottom"
    />
  </svg>
</template>

<style scoped>
.pm-chart { display: block; overflow: visible; }
.pm-chart__grid line { stroke: var(--pm-grid); stroke-width: 1; stroke-dasharray: 3 3; }
.pm-chart__ticks text { fill: var(--pm-muted); font-family: var(--pm-mono); font-size: 9.5px; }
.pm-chart__line { fill: none; stroke: var(--pm-tone); stroke-width: 1.7; stroke-linecap: round; stroke-linejoin: round; }
.pm-chart__area { fill: var(--pm-tone); fill-opacity: 0.1; }
.pm-chart__marker { stroke: var(--pm-ok); stroke-width: 1.2; stroke-dasharray: 4 3; stroke-opacity: 0.85; }
.pm-tone--blue { --pm-tone: var(--pm-c-blue); }
.pm-tone--green { --pm-tone: var(--pm-c-green); }
.pm-tone--purple { --pm-tone: var(--pm-c-purple); }
.pm-tone--orange { --pm-tone: var(--pm-c-orange); }
</style>
