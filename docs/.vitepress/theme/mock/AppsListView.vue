<script setup lang="ts">
import { PhDatabase, PhDotsThree, PhGlobe, PhPlus, PhRows, PhSquaresFour, PhBookmarkSimple } from '@phosphor-icons/vue'
import MockBadge from './MockBadge.vue'
import MockButton from './MockButton.vue'
import MockSparkline from './MockSparkline.vue'
import MockStatusPill from './MockStatusPill.vue'
import MockTable from './MockTable.vue'
import { appFilters, apps } from './mockData'
import type { MockCol } from './mockData'

const cols: MockCol[] = [
  { key: 'check', label: '', size: 'xs' },
  { key: 'name', label: 'Name', size: 'grow' },
  { key: 'traffic', label: 'Traffic 1h', size: 'lg' },
  { key: 'p95', label: 'p95', size: 'sm' },
  { key: 'errors', label: 'Errors', size: 'sm' },
  { key: 'deployed', label: 'Deployed', size: 'sm' },
  { key: 'menu', label: '', size: 'xs', align: 'end' },
]
</script>

<template>
  <div class="pm-apps">
    <div class="pm-apps__head">
      <div class="pm-apps__h">Apps</div>
      <span class="pm-apps__count">{{ apps.length }} apps</span>
      <MockButton>Select all</MockButton>
      <span class="pm-apps__toggle"><i class="on"><PhRows :size="15" /></i><i><PhSquaresFour :size="15" /></i></span>
      <MockButton :icon="PhDatabase">New database</MockButton>
      <MockButton variant="primary" :icon="PhPlus">New app</MockButton>
    </div>

    <div class="pm-apps__filters">
      <span v-for="f in appFilters" :key="f.label" class="pm-chip">
        <i :class="`pm-chip__dot--${f.tone}`"></i><b>{{ f.count }}</b>{{ f.label }}
      </span>
    </div>
    <div class="pm-apps__search">
      <span class="pm-apps__input">Search apps</span>
      <MockButton :icon="PhBookmarkSimple">Views</MockButton>
    </div>

    <MockTable :cols="cols" :rows="apps">
      <template #cell-check><i class="pm-check"></i></template>
      <template #cell-name="{ row }">
        <div class="pm-app">
          <span class="pm-tile" :class="`pm-tile--${row.tile}`">{{ row.name.charAt(0).toUpperCase() }}</span>
          <span class="pm-app__text">
            <MockStatusPill :status="row.status" :label="row.name" plain class="pm-app__name" />
            <MockBadge v-if="row.domain" mono><PhGlobe :size="10" />{{ row.domain }}</MockBadge>
          </span>
        </div>
      </template>
      <template #cell-traffic="{ row }">
        <div class="pm-traffic"><MockSparkline :values="row.traffic" :width="84" :height="24" /><span>{{ row.rpm }}</span></div>
      </template>
      <template #cell-p95="{ row }">{{ row.p95 }}</template>
      <template #cell-errors="{ row }">{{ row.errors }}</template>
      <template #cell-deployed="{ row }">{{ row.deployed }}</template>
      <template #cell-menu><PhDotsThree :size="18" /></template>
    </MockTable>
  </div>
</template>

<style scoped>
.pm-apps { flex: 1; min-width: 0; padding: 24px; }
.pm-apps__head { display: flex; align-items: center; gap: 10px; margin-bottom: 16px; }
.pm-apps__h { margin: 0 auto 0 0; color: var(--pm-head); font-size: 20px; font-weight: 600; letter-spacing: -0.01em; }
.pm-apps__count { color: var(--pm-muted); font-size: 12.5px; }
.pm-apps__toggle { display: flex; padding: 2px; border-radius: var(--pm-radius); box-shadow: 0 0 0 1px var(--pm-border); }
.pm-apps__toggle i { display: grid; place-items: center; width: 30px; height: 26px; border-radius: 4px; color: var(--pm-muted); }
.pm-apps__toggle i.on { background: var(--pm-sunken); color: var(--pm-head); }
.pm-apps__filters { display: flex; gap: 8px; margin-bottom: 12px; }
.pm-chip { display: inline-flex; align-items: center; gap: 7px; height: 28px; padding: 0 12px; border-radius: var(--pm-pill); box-shadow: 0 0 0 1px var(--pm-border); color: var(--pm-muted); font-size: 12.5px; }
.pm-chip b { color: var(--pm-head); font-weight: 500; }
.pm-chip i { width: 7px; height: 7px; border-radius: 50%; }
.pm-chip__dot--ok { background: var(--pm-ok); }
.pm-chip__dot--warn { background: var(--pm-c-orange); }
.pm-chip__dot--bad { background: var(--pm-bad); }
.pm-chip__dot--muted { background: var(--pm-muted); opacity: 0.5; }
.pm-apps__search { display: flex; gap: 8px; margin-bottom: 14px; }
.pm-apps__input { width: 224px; height: 30px; display: flex; align-items: center; padding: 0 10px; border-radius: var(--pm-radius); box-shadow: 0 0 0 1px var(--pm-border); background: var(--pm-surface); color: var(--pm-muted); font-size: 12.5px; }
.pm-check { display: block; width: 14px; height: 14px; border-radius: 4px; box-shadow: 0 0 0 1px var(--pm-border); }
.pm-app { display: flex; align-items: center; gap: 12px; }
.pm-app__text { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; padding: 10px 0; }
.pm-app__name { color: var(--pm-head); font-size: 14px; font-weight: 500; }
.pm-tile { display: grid; place-items: center; flex: none; width: 32px; height: 32px; border-radius: 8px; font-size: 13px; font-weight: 700; color: #fff; background: var(--pm-info); }
.pm-tile--web { background: #1f9d55; }
.pm-tile--api { background: #6d5bd0; }
.pm-tile--cache { background: #d94a3a; }
.pm-tile--docs { background: #107292; }
.pm-traffic { display: flex; align-items: center; gap: 12px; color: var(--pm-muted); font-size: 12px; }
</style>
