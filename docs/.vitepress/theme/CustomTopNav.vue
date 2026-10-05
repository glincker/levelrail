<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useData, useRoute } from 'vitepress'
import { VPNavBarSearch, VPSocialLinks } from 'vitepress/theme'
import { PhList, PhX } from '@phosphor-icons/vue'
import NavDropdown from './nav/NavDropdown.vue'
import NavMobileMenu from './nav/NavMobileMenu.vue'
import { navEntries } from './nav/navItems'

const { site, theme, isDark, frontmatter } = useData()
const route = useRoute()

function toggleDark() {
  isDark.value = !isDark.value
}

// The home hero stays on its own fixed dark palette regardless of the
// site's light/dark toggle (see custom.css's --hero-* tokens), but the
// nav's own glass tint normally follows the toggle. Unscrolled on the
// home page, the nav floats directly over that always-dark hero, so a
// light-mode glass tint there reads as a muddy wash over dark content.
// --onHero forces the nav to the dark glass treatment in exactly that
// one case; scrolling past the hero (or any non-home page) reverts to
// the toggle's own tint, which is correct once the nav sits over the
// page's own (toggle-respecting) background instead.
//
// isHome must be a computed, not a plain const read once: VitePress is
// an SPA, so this component persists across client-side route changes
// without remounting. A plain `const isHome = frontmatter.value...`
// captured the landing page's value forever (or never attached the
// scroll listener at all if you landed on a doc page first), which is
// why the nav's color didn't update when navigating between the home
// page and doc pages. The scroll listener itself can just always be
// attached -- it's a cheap no-op on pages where isHome is false, so it
// doesn't need its own attach/detach lifecycle tied to route changes.
const scrolled = ref(false)
const isHome = computed(() => frontmatter.value.layout === 'home')

function handleScroll() {
  scrolled.value = window.scrollY > 40
}

const COMPACT_QUERY = '(max-width: 1024px)'
const SCROLL_LOCK_CLASS = 'nav-scroll-locked'

const mounted = ref(false)
const openGroup = ref<string | null>(null)
const mobileOpen = ref(false)
const hamburger = ref<HTMLButtonElement | null>(null)
const mobileMenu = ref<InstanceType<typeof NavMobileMenu> | null>(null)
let compactMq: MediaQueryList | undefined

function closeAll() {
  openGroup.value = null
  mobileOpen.value = false
}

function toggleMobile() {
  openGroup.value = null
  mobileOpen.value = !mobileOpen.value
}

function onHamburgerKeydown(e: KeyboardEvent) {
  if (e.key === 'Tab' && !e.shiftKey && mobileOpen.value) {
    e.preventDefault()
    mobileMenu.value?.focusFirst()
  }
}

function onDocPointerDown(e: PointerEvent) {
  if (!openGroup.value) return
  if (!(e.target as Element | null)?.closest('.nav-dropdown')) openGroup.value = null
}

function onDocKeydown(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  if (mobileOpen.value) {
    mobileOpen.value = false
    hamburger.value?.focus()
  }
}

function onCompactChange(e: MediaQueryListEvent) {
  if (!e.matches) closeAll()
}

watch(mobileOpen, (isOpen) => {
  document.documentElement.classList.toggle(SCROLL_LOCK_CLASS, isOpen)
  if (isOpen) nextTick(() => mobileMenu.value?.focusFirst())
})

watch(() => route.path, closeAll)

onMounted(() => {
  mounted.value = true
  handleScroll()
  window.addEventListener('scroll', handleScroll, { passive: true })
  document.addEventListener('pointerdown', onDocPointerDown)
  document.addEventListener('keydown', onDocKeydown)
  compactMq = window.matchMedia(COMPACT_QUERY)
  compactMq.addEventListener('change', onCompactChange)
})

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll)
  document.removeEventListener('pointerdown', onDocPointerDown)
  document.removeEventListener('keydown', onDocKeydown)
  compactMq?.removeEventListener('change', onCompactChange)
  document.documentElement.classList.remove(SCROLL_LOCK_CLASS)
})
</script>

<template>
  <header
    class="custom-nav"
    :class="{ 'custom-nav--on-hero': isHome && !scrolled, 'custom-nav--scrolled': scrolled }"
  >
    <div class="custom-nav__inner">
      <a class="custom-nav__brand" href="/">
        <svg class="custom-nav__brand-mark" viewBox="0 0 256 256" aria-hidden="true">
          <rect x="10" y="174" width="236" height="68" rx="22" fill="#06232C"/>
          <rect x="10" y="162" width="236" height="68" rx="22" fill="#084F67"/>
          <rect x="42" y="103" width="172" height="68" rx="22" fill="#06232C"/>
          <rect x="42" y="91" width="172" height="68" rx="22" fill="#107292"/>
          <rect x="75" y="32" width="106" height="68" rx="22" fill="#06232C"/>
          <rect x="75" y="20" width="106" height="68" rx="22" fill="#58B1CE"/>
        </svg>
        <span class="custom-nav__brand-text">{{ site.title }}</span>
      </a>

      <nav class="custom-nav__links" aria-label="Main">
        <template v-for="entry in navEntries" :key="entry.kind === 'group' ? entry.group.label : entry.link.link">
          <NavDropdown
            v-if="entry.kind === 'group'"
            :id="`nav-${entry.group.label.toLowerCase()}`"
            :group="entry.group"
            :open="openGroup === entry.group.label"
            :any-open="openGroup !== null"
            @open="openGroup = entry.group.label"
            @close="openGroup === entry.group.label && (openGroup = null)"
          />
          <a v-else class="custom-nav__link" :href="entry.link.link">
            <span class="nav-dropdown__dot" aria-hidden="true" />
            {{ entry.link.text }}
          </a>
        </template>
      </nav>

      <div class="custom-nav__spacer" />

      <div class="custom-nav__search">
        <VPNavBarSearch />
      </div>

      <button
        type="button"
        class="custom-nav__icon-btn custom-nav__theme-btn"
        :class="{ 'custom-nav__theme-btn--dark': isDark }"
        aria-label="Toggle dark mode"
        @click="toggleDark"
      >
        <Transition name="theme-icon" mode="out-in">
          <svg v-if="!isDark" key="sun" width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <line x1="12" y1="1" x2="12" y2="4" stroke="#f59e0b" stroke-width="2" stroke-linecap="round" />
            <line x1="12" y1="20" x2="12" y2="23" stroke="#f59e0b" stroke-width="2" stroke-linecap="round" />
            <line x1="4.22" y1="4.22" x2="6.34" y2="6.34" stroke="#f59e0b" stroke-width="2" stroke-linecap="round" />
            <line x1="17.66" y1="17.66" x2="19.78" y2="19.78" stroke="#f59e0b" stroke-width="2" stroke-linecap="round" />
            <line x1="1" y1="12" x2="4" y2="12" stroke="#f59e0b" stroke-width="2" stroke-linecap="round" />
            <line x1="20" y1="12" x2="23" y2="12" stroke="#f59e0b" stroke-width="2" stroke-linecap="round" />
            <line x1="4.22" y1="19.78" x2="6.34" y2="17.66" stroke="#f59e0b" stroke-width="2" stroke-linecap="round" />
            <line x1="17.66" y1="6.34" x2="19.78" y2="4.22" stroke="#f59e0b" stroke-width="2" stroke-linecap="round" />
            <circle cx="12" cy="12" r="5" fill="url(#navSunGrad)" />
            <defs>
              <radialGradient id="navSunGrad" cx="0.35" cy="0.35" r="0.65">
                <stop offset="0%" stop-color="#fde68a" />
                <stop offset="100%" stop-color="#f59e0b" />
              </radialGradient>
            </defs>
          </svg>
          <svg v-else key="moon" width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path
              d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"
              fill="url(#navMoonGrad)"
              stroke="#a78bfa"
              stroke-width="1"
            />
            <circle cx="19" cy="5" r="1" fill="#c4b5fd" />
            <circle cx="22" cy="9" r="0.7" fill="#c4b5fd" />
            <circle cx="17" cy="2" r="0.5" fill="#c4b5fd" />
            <defs>
              <linearGradient id="navMoonGrad" x1="11" y1="3" x2="21" y2="13">
                <stop offset="0%" stop-color="#c4b5fd" />
                <stop offset="100%" stop-color="#8b5cf6" />
              </linearGradient>
            </defs>
          </svg>
        </Transition>
      </button>

      <VPSocialLinks v-if="theme.socialLinks?.length" class="custom-nav__socials" :links="theme.socialLinks" />

      <button
        ref="hamburger"
        type="button"
        class="custom-nav__icon-btn custom-nav__menu-btn"
        :aria-label="mobileOpen ? 'Close navigation menu' : 'Open navigation menu'"
        :aria-expanded="mobileOpen"
        aria-controls="nav-mobile-sheet"
        @click="toggleMobile"
        @keydown="onHamburgerKeydown"
      >
        <PhX v-if="mobileOpen" :size="18" weight="bold" aria-hidden="true" />
        <PhList v-else :size="18" weight="bold" aria-hidden="true" />
      </button>
    </div>

    <NavMobileMenu
      v-if="mounted"
      ref="mobileMenu"
      :entries="navEntries"
      :open="mobileOpen"
      :path="route.path"
      @close="mobileOpen = false"
      @wrap="hamburger?.focus()"
    />
  </header>
</template>
