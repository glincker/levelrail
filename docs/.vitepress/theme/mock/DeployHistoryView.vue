<script setup lang="ts">
import { PhArrowCounterClockwise, PhCursor, PhGitCommit } from '@phosphor-icons/vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import MockAppHeader from './MockAppHeader.vue'
import MockBadge from './MockBadge.vue'
import MockButton from './MockButton.vue'
import MockStatusPill from './MockStatusPill.vue'
import MockTable from './MockTable.vue'
import { deploys, rollbackDeploy, rollbackTargetIndex } from './mockData'
import type { MockCol, MockDeploy } from './mockData'

const props = defineProps<{ animate?: boolean }>()

const cols: MockCol[] = [
  { key: 'status', label: 'Status', size: 'md' },
  { key: 'commit', label: 'Commit', size: 'grow' },
  { key: 'image', label: 'Image', size: 'lg' },
  { key: 'when', label: 'Deployed', size: 'sm' },
  { key: 'action', label: '', size: 'md', align: 'end' },
]

// 0 idle, 1 cursor on Rollback, 2 pressed, 3 new row deploying, 4 new row live (also the static end state)
const step = ref(4)
const timers: number[] = []
const FINAL = 4

type RowState = 'deploying' | 'live' | 'superseded'
type Row = MockDeploy & { id: string; state: RowState }

const rows = computed<Row[]>(() => {
  const base: Row[] = deploys.map((d, i) => ({
    ...d,
    state: i === 0 && step.value >= 3 ? 'superseded' : d.status,
  }))
  if (step.value < 3) return base
  return [{ ...rollbackDeploy, state: step.value === 3 ? 'deploying' : 'live' }, ...base].slice(0, 5)
})

function after(ms: number, to: number): void {
  timers.push(window.setTimeout(() => { step.value = to }, ms))
}

function play(): void {
  step.value = 0
  after(1400, 1)
  after(2300, 2)
  after(2700, 3)
  after(4200, FINAL)
}

const rootEl = ref<HTMLElement | null>(null)
let observer: IntersectionObserver | undefined

onMounted(() => {
  const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  if (!props.animate || reduce) return
  step.value = 0
  const el = rootEl.value
  if (!el || !('IntersectionObserver' in window)) return play()
  observer = new IntersectionObserver(([e]) => {
    if (e.isIntersecting) {
      observer?.disconnect()
      play()
    }
  })
  observer.observe(el)
})

watch(() => props.animate, (on) => { if (!on) step.value = FINAL })
onBeforeUnmount(() => {
  timers.forEach(window.clearTimeout)
  observer?.disconnect()
})

const rowClass = (_: unknown, i: number): string | undefined => (i === 0 && step.value >= 3 ? 'pm-row--new' : undefined)
const cursorClass = computed(() => ({ 'is-at': step.value === 1 || step.value === 2, 'is-hidden': step.value >= 3 }))
</script>

<template>
  <div ref="rootEl" class="pm-dh">
    <MockAppHeader crumb="Deploys" />
    <div class="pm-dh__title"><b>Deploy history</b><span>Roll back to any previously built image in one click.</span></div>
    <div class="pm-dh__list">
      <MockTable :cols="cols" :rows="rows" :row-class="rowClass">
        <template #cell-status="{ row }">
          <MockStatusPill
            :status="row.state === 'deploying' ? 'deploying' : row.state === 'live' ? 'healthy' : 'idle'"
            :label="row.state === 'live' ? 'Live' : row.state === 'deploying' ? 'Deploying' : 'Superseded'"
          />
        </template>
        <template #cell-commit="{ row }">
          <div class="pm-commit"><b>{{ row.message }}</b><span><PhGitCommit :size="13" />{{ row.sha }} · {{ row.trigger }} · {{ row.duration }}</span></div>
        </template>
        <template #cell-image="{ row }"><MockBadge mono>{{ row.image.split('/').pop() }}</MockBadge></template>
        <template #cell-when="{ row }">{{ row.when }}</template>
        <template #cell-action="{ row, index }">
          <MockButton v-if="row.state === 'superseded'" small :icon="PhArrowCounterClockwise" :pressed="step === 2 && index === rollbackTargetIndex">Rollback</MockButton>
          <MockBadge v-else-if="row.state === 'live'" tone="ok">Serving traffic</MockBadge>
          <MockBadge v-else tone="info">Waiting for readiness</MockBadge>
        </template>
      </MockTable>
      <PhCursor v-if="animate" :size="22" weight="fill" class="pm-cursor" :class="cursorClass" />
    </div>
  </div>
</template>

<style scoped>
.pm-dh { flex: 1; min-width: 0; padding: 20px 24px; overflow: hidden; }
.pm-dh__title { display: flex; align-items: baseline; gap: 12px; margin-bottom: 12px; }
.pm-dh__title b { color: var(--pm-head); font-size: 14px; font-weight: 500; }
.pm-dh__title span { color: var(--pm-muted); font-size: 12.5px; }
.pm-dh__list { position: relative; }
.pm-commit { display: flex; flex-direction: column; gap: 3px; padding: 10px 0; }
.pm-commit b { color: var(--pm-head); font-weight: 500; font-size: 13.5px; }
.pm-commit span { display: inline-flex; align-items: center; gap: 4px; color: var(--pm-muted); font-family: var(--pm-mono); font-size: 11px; }
.pm-cursor {
  position: absolute;
  top: 300px;
  right: 8px;
  color: var(--pm-head);
  filter: drop-shadow(0 1px 1px rgba(0, 0, 0, 0.35));
  transition: transform 0.9s cubic-bezier(0.3, 0.7, 0.2, 1), opacity 0.3s ease;
  transform: translate(0, 0);
}
.pm-cursor.is-at { transform: translate(-36px, -165px); }
.pm-cursor.is-hidden { opacity: 0; transform: translate(-36px, -165px); }
</style>
