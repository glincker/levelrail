import { computed, inject, onBeforeUnmount, onMounted, provide, reactive, ref, watch } from 'vue'
import type { InjectionKey, Ref } from 'vue'
import { STORY_MS, TICK_MS } from './mockTiming'

export type CursorTarget = 'redeploy' | 'rollback'

export interface MockMotion {
  tick: Ref<number>
  deployed: Ref<number>
  status: Ref<'healthy' | 'deploying'>
  pressed: Ref<boolean>
  toast: Ref<boolean>
  deployCount: Ref<number>
  cursor: { x: number; y: number; show: boolean; click: boolean }
  moveCursor: (target: CursorTarget) => void
  clickCursor: () => void
  hideCursor: () => void
}

export const motionKey: InjectionKey<MockMotion> = Symbol('mock-motion')
export const useMotion = (): MockMotion | null => inject(motionKey, null)

interface Opts {
  root: Ref<HTMLElement | null>
  stage: Ref<HTMLElement | null>
  enabled: Ref<boolean>
  story: Ref<boolean>
  once: Ref<boolean>
  onRunning: (v: boolean) => void
}

const REST = { x: 820, y: 430 }
const BASE_DEPLOYS = 4

export function useMockLoop(o: Opts): MockMotion {
  const tick = ref(0)
  const deployed = ref(0)
  const status = ref<'healthy' | 'deploying'>('healthy')
  const pressed = ref(false)
  const toast = ref(false)
  const deployCount = ref(BASE_DEPLOYS)
  const cursor = reactive({ ...REST, show: false, click: false })

  const reduced = typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const inView = ref(false)
  const pageVisible = ref(true)
  const finished = ref(false)
  const running = computed(() => o.enabled.value && !reduced && inView.value && pageVisible.value && !finished.value)

  let tickTimer: number | undefined
  let stopTimer: number | undefined
  const timers: number[] = []
  let io: IntersectionObserver | undefined
  let loops = 0

  function point(name: CursorTarget): void {
    const stage = o.stage.value
    const el = stage?.querySelector<HTMLElement>(`[data-pm="${name}"]`)
    if (!stage || !el) return
    const s = stage.getBoundingClientRect()
    const r = el.getBoundingClientRect()
    const k = s.width / 1280 || 1
    cursor.x = (r.left - s.left) / k + (r.width / k) * 0.55
    cursor.y = (r.top - s.top) / k + (r.height / k) * 0.5
  }

  function moveCursor(target: CursorTarget): void {
    point(target)
    cursor.show = true
  }
  function clickCursor(): void {
    cursor.click = true
    timers.push(window.setTimeout(() => { cursor.click = false }, 500))
  }
  function hideCursor(): void {
    cursor.show = false
    timers.push(window.setTimeout(() => { cursor.x = REST.x; cursor.y = REST.y }, 400))
  }

  function at(ms: number, fn: () => void): void {
    timers.push(window.setTimeout(fn, ms))
  }

  function resetStory(): void {
    timers.forEach(window.clearTimeout)
    timers.length = 0
    status.value = 'healthy'
    pressed.value = false
    toast.value = false
    cursor.show = false
    cursor.click = false
    cursor.x = REST.x
    cursor.y = REST.y
  }

  function runStory(): void {
    at(600, () => moveCursor('redeploy'))
    at(1700, () => { pressed.value = true; clickCursor() })
    at(1950, () => { pressed.value = false; status.value = 'deploying'; hideCursor() })
    at(3900, () => {
      status.value = 'healthy'
      loops += 1
      deployCount.value = BASE_DEPLOYS + 1 + (loops % 3)
      deployed.value += 1
      toast.value = true
    })
    at(6700, () => { toast.value = false })
    at(STORY_MS, () => { resetStory(); deployCount.value = BASE_DEPLOYS + (loops % 3); runStory() })
  }

  function start(): void {
    tickTimer = window.setInterval(() => { tick.value += 1 }, TICK_MS)
    if (o.story.value) runStory()
    if (o.once.value) {
      stopTimer = window.setTimeout(() => { finished.value = true }, STORY_MS + 500)
    }
  }
  function stop(): void {
    window.clearInterval(tickTimer)
    window.clearTimeout(stopTimer)
    tickTimer = undefined
    resetStory()
  }

  watch(running, (on) => {
    o.onRunning(on)
    if (on) start()
    else stop()
  })
  watch(o.story, () => {
    if (!running.value) return
    resetStory()
    if (o.story.value) runStory()
  })

  const onVis = (): void => { pageVisible.value = !document.hidden }

  onMounted(() => {
    pageVisible.value = !document.hidden
    document.addEventListener('visibilitychange', onVis)
    if (reduced || !('IntersectionObserver' in window) || !o.root.value) return
    io = new IntersectionObserver(([e]) => { inView.value = e.isIntersecting })
    io.observe(o.root.value)
  })
  onBeforeUnmount(() => {
    document.removeEventListener('visibilitychange', onVis)
    io?.disconnect()
    stop()
  })

  const motion: MockMotion = { tick, deployed, status, pressed, toast, deployCount, cursor, moveCursor, clickCursor, hideCursor }
  provide(motionKey, motion)
  return motion
}
