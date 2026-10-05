<script setup lang="ts">
import { computed } from 'vue'
import { smoothPath, toPoints } from './chartPath'

const props = withDefaults(defineProps<{
  values: number[]
  tone?: 'blue' | 'green' | 'purple' | 'orange'
  width?: number
  height?: number
}>(), { tone: 'green', width: 96, height: 26 })

const line = computed(() => {
  const min = Math.min(...props.values)
  const max = Math.max(...props.values)
  return toPoints(props.values, 1, props.width - 1, 3, props.height - 3, min, max)
})
const d = computed(() => smoothPath(line.value))
const area = computed(() => `${d.value} L${props.width - 1} ${props.height} L1 ${props.height} Z`)
</script>

<template>
  <svg class="pm-spark" :class="`pm-tone--${tone}`" :width="width" :height="height" :viewBox="`0 0 ${width} ${height}`">
    <path class="pm-spark__area pm-fade" :d="area" />
    <path class="pm-spark__line pm-draw" :d="d" pathLength="1" />
  </svg>
</template>

<style scoped>
.pm-spark { display: block; overflow: visible; }
.pm-spark__line { fill: none; stroke: var(--pm-tone); stroke-width: 1.6; stroke-linecap: round; }
.pm-spark__area { fill: var(--pm-tone); fill-opacity: 0.12; }
.pm-tone--blue { --pm-tone: var(--pm-c-blue); }
.pm-tone--green { --pm-tone: var(--pm-c-green); }
.pm-tone--purple { --pm-tone: var(--pm-c-purple); }
.pm-tone--orange { --pm-tone: var(--pm-c-orange); }
</style>
