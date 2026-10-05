<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { stopWatching, whenVisible } from './reveal'

const props = withDefaults(defineProps<{ value: string; delay?: number }>(), { delay: 0 })

const m = /^(\d[\d.]*)\s*(.*)$/.exec(props.value)
const target = m ? Number(m[1]) : 0
const decimals = m?.[1].split('.')[1]?.length ?? 0
const unit = m?.[2] ?? ''
const shown = ref(m ? m[1] : props.value)
const el = ref<HTMLElement | null>(null)
let raf = 0
let delayTimer = 0

const fmt = (n: number): string => n.toFixed(decimals)

function run(): void {
  const t0 = performance.now()
  const frame = (now: number): void => {
    const p = Math.min(1, (now - t0) / 900)
    shown.value = fmt(target * (1 - (1 - p) ** 3))
    if (p < 1) raf = requestAnimationFrame(frame)
    else shown.value = m ? m[1] : props.value
  }
  raf = requestAnimationFrame(frame)
}

onMounted(() => {
  if (!m || !el.value || window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
  shown.value = fmt(0)
  whenVisible(el.value, () => { delayTimer = window.setTimeout(run, props.delay) })
})
onBeforeUnmount(() => {
  cancelAnimationFrame(raf)
  window.clearTimeout(delayTimer)
  if (el.value) stopWatching(el.value)
})
</script>

<template>
  <span ref="el" class="count">
    <span class="count__sr">{{ value }}</span>
    <span class="count__num" aria-hidden="true">{{ shown }}</span><span v-if="unit" class="count__unit" aria-hidden="true">{{ unit }}</span>
  </span>
</template>
