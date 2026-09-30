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
        <span class="custom-nav__brand-mark" aria-hidden="true" />
        {{ site.title }}
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
