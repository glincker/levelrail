<script setup lang="ts">
import { useData } from 'vitepress'
import { VPNavBarSearch, VPSocialLinks } from 'vitepress/theme'
import { PhSun, PhMoon } from '@phosphor-icons/vue'

const { site, theme, isDark } = useData()

function toggleDark() {
  isDark.value = !isDark.value
}
</script>

<template>
  <header class="custom-nav">
    <div class="custom-nav__inner">
      <a class="custom-nav__brand" href="/">
        <svg class="custom-nav__brand-mark" viewBox="0 0 256 256" aria-hidden="true">
          <rect x="18" y="176" width="220" height="46" rx="15" fill="#06232C"/>
          <rect x="18" y="168" width="220" height="46" rx="15" fill="#084F67"/>
          <rect x="48" y="128" width="160" height="46" rx="15" fill="#06232C"/>
          <rect x="48" y="120" width="160" height="46" rx="15" fill="#107292"/>
          <rect x="78" y="80" width="100" height="46" rx="15" fill="#06232C"/>
          <rect x="78" y="72" width="100" height="46" rx="15" fill="#58B1CE"/>
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
        class="custom-nav__icon-btn"
        aria-label="Toggle dark mode"
        @click="toggleDark"
      >
        <PhSun v-if="!isDark" :size="16" weight="bold" />
        <PhMoon v-else :size="16" weight="bold" />
      </button>

      <VPSocialLinks v-if="theme.socialLinks?.length" class="custom-nav__socials" :links="theme.socialLinks" />
    </div>
  </header>
</template>
