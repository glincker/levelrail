<script setup lang="ts">
import DefaultTheme from 'vitepress/theme'
import { useSidebar } from 'vitepress/theme'
import HeroField from './HeroField.vue'
import CustomTopNav from './CustomTopNav.vue'
import CustomSidebar from './CustomSidebar.vue'
import CustomFooter from './CustomFooter.vue'
import PageActions from './PageActions.vue'

const { Layout } = DefaultTheme
const { hasSidebar } = useSidebar()
</script>

<template>
  <Layout>
    <template v-if="hasSidebar" #layout-top>
      <CustomTopNav />
      <CustomSidebar />
    </template>
    <template v-if="hasSidebar" #doc-before>
      <PageActions />
    </template>
    <template #home-hero-info-before>
      <p class="hero-eyebrow">Self-hosted &middot; Apache 2.0</p>
    </template>
    <template #home-hero-image>
      <HeroField />
    </template>
    <template #home-hero-actions-after>
      <div class="trust-strip">
        <a
          class="trust-strip__item"
          href="https://github.com/glincker/levelrail/blob/main/LICENSE"
          target="_blank"
          rel="noreferrer"
        >
          Apache 2.0
        </a>
        <span class="trust-strip__item trust-strip__item--text">
          Runs on your own servers: secrets, metrics, and logs stay node-local, nothing shipped to a third party.
        </span>
      </div>
    </template>
    <template #layout-bottom>
      <CustomFooter />
    </template>
  </Layout>
</template>
