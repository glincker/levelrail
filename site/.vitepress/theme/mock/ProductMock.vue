<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useData } from 'vitepress'
import AppOverviewView from './AppOverviewView.vue'
import AppsListView from './AppsListView.vue'
import DeployHistoryView from './DeployHistoryView.vue'
import LogsView from './LogsView.vue'
import MockCursor from './MockCursor.vue'
import MockSidebar from './MockSidebar.vue'
import MockToast from './MockToast.vue'
import MockTopbar from './MockTopbar.vue'
import MockWindow from './MockWindow.vue'
import { hostUrl } from './mockData'
import { SLIDE_MS } from './mockTiming'
import { useMockLoop } from './useMockLoop'

const props = withDefaults(defineProps<{
  view: 'apps' | 'overview' | 'deploys' | 'logs'
  theme?: 'dark' | 'light' | 'auto'
  animate?: boolean
  active?: boolean
  paused?: boolean
  once?: boolean
  cropped?: boolean
  label?: string
}>(), { theme: 'auto', active: true })

const emit = defineEmits<{ running: [on: boolean] }>()

const { isDark } = useData()
const resolved = computed(() => (props.theme === 'auto' ? (isDark.value ? 'dark' : 'light') : props.theme))

const views = { apps: AppsListView, overview: AppOverviewView, deploys: DeployHistoryView, logs: LogsView }
const meta = {
  apps: { mode: 'global', active: 'apps', url: `${hostUrl}/apps`, what: 'the apps list with healthy services' },
  overview: { mode: 'app', active: 'metrics', url: `${hostUrl}/apps/marketing-site/metrics`, what: 'an app overview with live metrics and deploy markers' },
  deploys: { mode: 'app', active: 'deploys', url: `${hostUrl}/apps/marketing-site/deploys`, what: 'deploy history with one-click rollback' },
  logs: { mode: 'app', active: 'logs', url: `${hostUrl}/apps/marketing-site/logs`, what: 'live application logs' },
} as const

const m = computed(() => meta[props.view])
const ariaLabel = computed(() => props.label ?? `Illustrative rendering of the Levelrail dashboard showing ${m.value.what}.`)

const root = ref<HTMLElement | null>(null)
const stage = ref<HTMLElement | null>(null)
const ready = ref(false)
const motion = useMockLoop({
  root,
  stage,
  enabled: computed(() => !!props.animate && props.active && !props.paused),
  story: computed(() => props.view === 'overview'),
  once: computed(() => !!props.once),
  onRunning: (on) => emit('running', on),
})
let ro: ResizeObserver | undefined

function fit(): void {
  const el = root.value
  if (!el) return
  const factor = props.cropped ? 1.38 : 1
  el.style.setProperty('--pm-scale', String((el.clientWidth * factor) / 1280))
  el.style.setProperty('--pm-dur-slide', `${SLIDE_MS}ms`)
}

onMounted(() => {
  fit()
  ready.value = true
  if ('ResizeObserver' in window && root.value) {
    ro = new ResizeObserver(fit)
    ro.observe(root.value)
  }
})
onBeforeUnmount(() => ro?.disconnect())
</script>

<template>
  <div
    ref="root"
    class="pm"
    :class="[`pm--${resolved}`, { 'pm--animate': animate, 'pm--cropped': cropped, 'pm--ready': ready }]"
    role="img"
    :aria-label="ariaLabel"
  >
    <div class="pm-viewport" aria-hidden="true">
      <div ref="stage" class="pm-stage">
        <MockWindow :url="m.url">
          <MockSidebar :mode="m.mode" :active="m.active" />
          <div class="pm-col">
            <MockTopbar />
            <Transition name="pm-view" mode="out-in">
              <component :is="views[view]" :key="view" :animate="animate" />
            </Transition>
          </div>
        </MockWindow>
        <MockToast :show="motion.toast.value" />
        <MockCursor v-bind="motion.cursor" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.pm-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
</style>
