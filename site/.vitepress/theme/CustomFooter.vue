<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { gsap } from 'gsap'
import { ScrollTrigger } from 'gsap/ScrollTrigger'
import { PhArrowUp, PhHeart } from '@phosphor-icons/vue'
import { footerColumns, footerLegalLinks } from './footer/footerLinks'

let pluginRegistered = false

const footerRef = ref<HTMLElement | null>(null)
const backToTopRef = ref<HTMLButtonElement | null>(null)
const creditsRef = ref<HTMLElement | null>(null)

let reduced = false
let motionQuery: MediaQueryList | null = null
let triggers: ScrollTrigger[] = []
let magneticCleanups: Array<() => void> = []

// Elastic release back to origin; linear follow while the cursor is near.
function attachMagnetic(el: HTMLElement | null) {
  if (!el) return
  const handleMove = (e: MouseEvent) => {
    const rect = el.getBoundingClientRect()
    const x = e.clientX - rect.left - rect.width / 2
    const y = e.clientY - rect.top - rect.height / 2
    gsap.to(el, { x: x * 0.35, y: y * 0.35, scale: 1.05, duration: 0.4, ease: 'power2.out' })
  }
  const handleLeave = () => {
    gsap.to(el, { x: 0, y: 0, scale: 1, duration: 1, ease: 'elastic.out(1, 0.4)' })
  }
  el.addEventListener('mousemove', handleMove)
  el.addEventListener('mouseleave', handleLeave)
  magneticCleanups.push(() => {
    el.removeEventListener('mousemove', handleMove)
    el.removeEventListener('mouseleave', handleLeave)
    gsap.killTweensOf(el)
    gsap.set(el, { clearProps: 'transform' })
  })
}

function scrollToTop() {
  window.scrollTo({ top: 0, behavior: reduced ? 'auto' : 'smooth' })
}

function handleMotionChange(e: MediaQueryListEvent) {
  reduced = e.matches
}

onMounted(() => {
  if (typeof window === 'undefined' || !footerRef.value) return

  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  reduced = motionQuery.matches
  motionQuery.addEventListener('change', handleMotionChange)

  // Reduced motion: leave everything in its natural, fully visible CSS
  // state and skip GSAP entirely, including the magnetic buttons.
  if (reduced) return

  if (!pluginRegistered) {
    gsap.registerPlugin(ScrollTrigger)
    pluginRegistered = true
  }

  const revealTargets = footerRef.value.querySelectorAll('.custom-footer__reveal')
  if (revealTargets.length) {
    gsap.set(revealTargets, { opacity: 0, y: 32 })
    const tween = gsap.to(revealTargets, {
      opacity: 1,
      y: 0,
      duration: 0.7,
      ease: 'power2.out',
      stagger: 0.07,
      scrollTrigger: {
        trigger: footerRef.value,
        start: 'top 85%',
        toggleActions: 'play none none none',
      },
    })
    if (tween.scrollTrigger) triggers.push(tween.scrollTrigger)
  }

  const giant = footerRef.value.querySelector('.custom-footer__giant')
  if (giant) {
    // Tween to the CSS-declared opacity, not 1: an inline style beats the
    // class rule, so animating to 1 permanently un-dimmed the watermark.
    const watermarkOpacity = parseFloat(getComputedStyle(giant).opacity) || 0.05
    gsap.set(giant, { opacity: 0, y: 20 })
    const giantTween = gsap.to(giant, {
      opacity: watermarkOpacity,
      y: 0,
      duration: 1.1,
      ease: 'power1.out',
      scrollTrigger: {
        trigger: footerRef.value,
        start: 'top 90%',
        toggleActions: 'play none none none',
      },
    })
    if (giantTween.scrollTrigger) triggers.push(giantTween.scrollTrigger)
  }

  attachMagnetic(backToTopRef.value)
  attachMagnetic(creditsRef.value)

  // Self-hosted fonts finish loading after mount and can reflow page
  // height, leaving ScrollTrigger's cached trigger points stale.
  if (typeof document !== 'undefined' && document.fonts) {
    document.fonts.ready.then(() => ScrollTrigger.refresh())
  }
})

onUnmounted(() => {
  motionQuery?.removeEventListener('change', handleMotionChange)
  magneticCleanups.forEach((fn) => fn())
  magneticCleanups = []
  triggers.forEach((trigger) => trigger.kill())
  triggers = []
})
</script>

<template>
  <footer ref="footerRef" class="custom-footer">
    <div class="custom-footer__backdrop" aria-hidden="true">
      <div class="custom-footer__glow" />
      <div class="custom-footer__grid" />
    </div>

    <div class="custom-footer__marquee" aria-hidden="true">
      <div class="custom-footer__marquee-track">
        <span v-for="n in 2" :key="n" class="custom-footer__marquee-group">
          <span class="custom-footer__marquee-item">Self-hosted</span>
          <span class="custom-footer__marquee-dot">&#9670;</span>
          <span class="custom-footer__marquee-item">Apache 2.0</span>
          <span class="custom-footer__marquee-dot">&#9670;</span>
          <span class="custom-footer__marquee-item">Secrets, metrics, and logs stay node-local</span>
          <span class="custom-footer__marquee-dot">&#9670;</span>
          <span class="custom-footer__marquee-item">Nothing shipped to a third party</span>
          <span class="custom-footer__marquee-dot">&#9670;</span>
        </span>
      </div>
    </div>

    <div class="custom-footer__giant" aria-hidden="true">Levelrail</div>

    <div class="custom-footer__inner">
      <div class="custom-footer__brand custom-footer__reveal">
        <p class="custom-footer__wordmark">Levelrail</p>
        <p class="custom-footer__blurb">A self-hosted deployment platform. Free and open source under Apache 2.0.</p>
        <a href="https://glincker.com" target="_blank" rel="noreferrer" class="custom-footer__badge">
          A GLINCKER project
        </a>
      </div>
      <nav
        v-for="col in footerColumns"
        :key="col.heading"
        class="custom-footer__col custom-footer__reveal"
        :aria-label="col.heading"
      >
        <p class="custom-footer__heading">{{ col.heading }}</p>
        <a
          v-for="l in col.links"
          :key="l.link"
          :href="l.link"
          :target="l.external && l.link.startsWith('http') ? '_blank' : undefined"
          :rel="l.external && l.link.startsWith('http') ? 'noreferrer' : undefined"
        >{{ l.text }}</a>
      </nav>
    </div>

    <div class="custom-footer__bottom custom-footer__reveal">
      <nav class="custom-footer__legal" aria-label="Legal">
        <a v-for="l in footerLegalLinks" :key="l.link" :href="l.link">{{ l.text }}</a>
      </nav>
      <div ref="creditsRef" class="custom-footer__credits">
        <span>Crafted with</span>
        <PhHeart class="custom-footer__heart" :size="14" weight="fill" />
        <span>by</span>
        <a href="https://glincker.com" target="_blank" rel="noreferrer">GLINCKER</a>
      </div>
      <button
        ref="backToTopRef"
        type="button"
        class="custom-footer__to-top"
        aria-label="Back to top"
        @click="scrollToTop"
      >
        <PhArrowUp :size="16" weight="bold" />
      </button>
    </div>
  </footer>
</template>
