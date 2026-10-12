<script setup lang="ts">
import { PhArrowRight, PhCaretLeft, PhCaretRight } from '@phosphor-icons/vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { heroViews, showcaseSlides } from './carouselData'
import type { MockView } from './carouselData'
import { CAROUSEL_MS, SHOWCASE_MS } from './mockTiming'
import ProductMock from './ProductMock.vue'

const props = withDefaults(defineProps<{
  variant?: 'hero' | 'showcase'
  firstLabel?: string
  theme?: 'dark' | 'light' | 'auto'
}>(), { variant: 'showcase', theme: 'auto' })

const hero = computed(() => props.variant === 'hero')
const count = computed(() => (hero.value ? heroViews.length : showcaseSlides.length))
const heroLabels: Record<string, string> = { overview: 'Overview', deploys: 'Deploys', logs: 'Logs' }
const cur = ref(0)
const hover = ref(false)
const focused = ref(false)
const inView = ref(false)
const pageVisible = ref(true)
const touched = ref(false)
const reduced = ref(true)
const root = ref<HTMLElement | null>(null)
const track = ref<HTMLElement | null>(null)
const slideEls = ref<HTMLElement[]>([])

const auto = computed(() => !reduced.value && inView.value && pageVisible.value && !hover.value && !focused.value && !(touched.value && !hero.value))
const heroView = computed<MockView>(() => heroViews[cur.value])

let timer: number | undefined
let io: IntersectionObserver | undefined
let slideIo: IntersectionObserver | undefined

function schedule(): void {
  window.clearTimeout(timer)
  if (!auto.value) return
  timer = window.setTimeout(() => go((cur.value + 1) % count.value, false), hero.value ? CAROUSEL_MS : SHOWCASE_MS)
}
watch([auto, cur], schedule)

function go(i: number, user = true): void {
  const next = (i + count.value) % count.value
  if (user) touched.value = true
  cur.value = next
  const el = track.value
  if (!hero.value && el) el.scrollTo({ left: next * el.clientWidth, behavior: reduced.value ? 'auto' : 'smooth' })
}

function onKey(e: KeyboardEvent): void {
  if (e.key === 'ArrowRight') { e.preventDefault(); go(cur.value + 1) }
  else if (e.key === 'ArrowLeft') { e.preventDefault(); go(cur.value - 1) }
}

const onVis = (): void => { pageVisible.value = !document.hidden }

onMounted(() => {
  reduced.value = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  pageVisible.value = !document.hidden
  document.addEventListener('visibilitychange', onVis)
  if (!('IntersectionObserver' in window)) return
  io = new IntersectionObserver(([e]) => { inView.value = e.isIntersecting })
  if (root.value) io.observe(root.value)
  if (!hero.value && track.value) {
    slideIo = new IntersectionObserver((entries) => {
      for (const e of entries) if (e.isIntersecting) cur.value = slideEls.value.indexOf(e.target as HTMLElement)
    }, { root: track.value, threshold: 0.6 })
    slideEls.value.forEach((el) => slideIo?.observe(el))
  }
})
onBeforeUnmount(() => {
  window.clearTimeout(timer)
  document.removeEventListener('visibilitychange', onVis)
  io?.disconnect()
  slideIo?.disconnect()
})

const near = (i: number): boolean => Math.abs(i - cur.value) <= 1
const dotLabel = (i: number): string => `Show ${hero.value ? heroLabels[heroViews[i]] : showcaseSlides[i].label}`
const setSlideEl = (el: unknown, i: number): void => { if (el) slideEls.value[i] = el as HTMLElement }
</script>

<template>
  <div
    v-if="hero"
    ref="root"
    class="mc mc--hero"
    @mouseenter="hover = true"
    @mouseleave="hover = false"
    @focusin="focused = true"
    @focusout="focused = false"
  >
    <ProductMock :view="heroView" :theme="theme" animate :paused="hover" :label="cur === 0 ? firstLabel : undefined" />
    <div class="mc__dots mc__dots--hero" role="group" aria-label="Choose a product view">
      <button
        v-for="(v, i) in heroViews"
        :key="v"
        type="button"
        class="mc__dot"
        :class="{ 'is-on': i === cur }"
        :aria-label="dotLabel(i)"
        :aria-pressed="i === cur"
        @click="go(i)"
      ><span></span></button>
    </div>
  </div>

  <section
    v-else
    ref="root"
    class="mc mc--showcase"
    role="group"
    aria-roledescription="carousel"
    aria-label="Product screens"
    @mouseenter="hover = true"
    @mouseleave="hover = false"
    @focusin="focused = true"
    @focusout="focused = false"
    @keydown="onKey"
  >
    <div ref="track" class="mc__track" tabindex="0" aria-label="Swipe or use the arrow keys to change screen">
      <figure
        v-for="(s, i) in showcaseSlides"
        :key="s.view"
        :ref="(el) => setSlideEl(el, i)"
        class="mc__slide"
        role="group"
        aria-roledescription="slide"
        :aria-label="`${i + 1} of ${showcaseSlides.length}`"
        :inert="i === cur ? undefined : true"
      >
        <div class="mc__frame">
          <ProductMock v-if="near(i)" :view="s.view" :theme="theme" animate :active="i === cur" :once="false" />
          <div v-else class="mc__ph"></div>
        </div>
        <figcaption class="mc__cap">
          <span class="mc__tag"><component :is="s.icon" :size="16" weight="duotone" />{{ s.label }}</span>
          <span class="mc__text">{{ s.caption }}</span>
          <a class="mc__link" :href="s.link.href">{{ s.link.text }}<PhArrowRight :size="14" weight="bold" /></a>
        </figcaption>
      </figure>
    </div>
    <div class="mc__controls">
      <button type="button" class="mc__btn" aria-label="Previous screen" @click="go(cur - 1)"><PhCaretLeft :size="16" weight="bold" /></button>
      <div class="mc__dots" role="group" aria-label="Choose a screen">
        <button
          v-for="(s, i) in showcaseSlides"
          :key="s.view"
          type="button"
          class="mc__dot"
          :class="{ 'is-on': i === cur }"
          :aria-label="dotLabel(i)"
          :aria-pressed="i === cur"
          @click="go(i)"
        ><span></span></button>
      </div>
      <button type="button" class="mc__btn" aria-label="Next screen" @click="go(cur + 1)"><PhCaretRight :size="16" weight="bold" /></button>
    </div>
  </section>
</template>
