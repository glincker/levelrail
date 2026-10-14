import { onBeforeUnmount, onMounted } from 'vue'
import type { Directive, Ref } from 'vue'

type Cb = () => void

let io: IntersectionObserver | undefined
const pending = new WeakMap<Element, Cb>()

const motionOk = (): boolean =>
  typeof window !== 'undefined' && 'IntersectionObserver' in window && !window.matchMedia('(prefers-reduced-motion: reduce)').matches

function shared(): IntersectionObserver {
  io ??= new IntersectionObserver((entries) => {
    for (const e of entries) {
      if (!e.isIntersecting) continue
      io?.unobserve(e.target)
      pending.get(e.target)?.()
      pending.delete(e.target)
    }
  }, { rootMargin: '0px 0px -8% 0px' })
  return io
}

/** Calls cb once when el scrolls into view; runs it immediately when motion is off or unsupported. */
export function whenVisible(el: Element, cb: Cb): void {
  if (!motionOk()) return cb()
  pending.set(el, cb)
  shared().observe(el)
}

export function stopWatching(el: Element): void {
  pending.delete(el)
  io?.unobserve(el)
}

/** Hides el until it scrolls into view, then fades it up once. Above-the-fold and no-motion elements stay as rendered. */
export function reveal(el: HTMLElement, index = 0): void {
  if (!motionOk() || el.getBoundingClientRect().top < window.innerHeight * 0.92) return
  el.style.setProperty('--reveal-i', String(index))
  el.classList.add('reveal')
  whenVisible(el, () => {
    el.classList.add('is-in')
    const done = (e: TransitionEvent): void => {
      if (e.target !== el || e.propertyName !== 'opacity') return
      el.removeEventListener('transitionend', done)
      el.classList.remove('reveal', 'is-in')
    }
    el.addEventListener('transitionend', done)
  })
}

export const vReveal: Directive<HTMLElement, number | undefined> = {
  mounted: (el, b) => reveal(el, b.value ?? 0),
  beforeUnmount: (el) => stopWatching(el),
  getSSRProps: () => ({}),
}

export function useReveal(el: Ref<HTMLElement | null>, index = 0): void {
  onMounted(() => { if (el.value) reveal(el.value, index) })
  onBeforeUnmount(() => { if (el.value) stopWatching(el.value) })
}
