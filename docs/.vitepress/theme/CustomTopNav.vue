<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useData } from 'vitepress'
import { VPNavBarSearch, VPSocialLinks } from 'vitepress/theme'

const { site, theme, isDark, frontmatter } = useData()

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

onMounted(() => {
  handleScroll()
  window.addEventListener('scroll', handleScroll, { passive: true })
})

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll)
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
        <a
          v-for="item in theme.nav ?? []"
          :key="item.text"
          class="custom-nav__link"
          :href="item.link"
        >
          {{ item.text }}
        </a>
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
    </div>
  </header>
</template>
