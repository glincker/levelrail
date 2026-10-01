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
