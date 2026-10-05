<script setup lang="ts" generic="T extends { id: string }">
import type { MockCol } from './mockData'

defineProps<{ cols: MockCol[]; rows: T[]; rowClass?: (row: T, index: number) => string | undefined }>()
</script>

<template>
  <div class="pm-table">
    <div class="pm-table__head">
      <span v-for="c in cols" :key="c.key" class="pm-table__cell" :class="[`pm-w--${c.size ?? 'md'}`, { 'is-end': c.align }]">{{ c.label }}</span>
    </div>
    <div v-for="(r, i) in rows" :key="r.id" class="pm-table__row" :class="rowClass?.(r, i)">
      <span v-for="c in cols" :key="c.key" class="pm-table__cell" :class="[`pm-w--${c.size ?? 'md'}`, { 'is-end': c.align }]">
        <slot :name="`cell-${c.key}`" :row="r" :index="i" />
      </span>
    </div>
  </div>
</template>

<style scoped>
.pm-table {
  position: relative;
  border-radius: 12px;
  background: var(--pm-surface);
  box-shadow: 0 0 0 1px var(--pm-border);
  overflow: hidden;
}
.pm-table__head,
.pm-table__row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 20px;
}
.pm-table__head {
  height: 40px;
  border-bottom: 1px solid var(--pm-border);
  color: var(--pm-muted);
  font-size: 12px;
}
.pm-table__row {
  min-height: 64px;
  border-bottom: 1px solid var(--pm-border);
}
.pm-table__row:last-child { border-bottom: 0; }
.pm-table__cell { min-width: 0; }
.pm-table__cell.is-end { display: flex; justify-content: flex-end; }
.pm-w--xs { flex: 0 0 36px; }
.pm-w--sm { flex: 0 0 90px; }
.pm-w--md { flex: 0 0 130px; }
.pm-w--lg { flex: 0 0 200px; }
.pm-w--grow { flex: 1; }
</style>
