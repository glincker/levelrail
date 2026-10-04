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
    <template #layout-top>
      <!-- CustomTopNav renders on every page, including home: it has no
           sidebar dependency of its own. CustomSidebar stays gated on
           hasSidebar -- the home page has no sidebar at all. Previously
           both were bundled under the same v-if, so CustomTopNav (and
           its floating-pill/glass styling) never actually mounted on the
           home page; only VitePress's own unstyled default nav did. -->
      <CustomTopNav />
      <CustomSidebar v-if="hasSidebar" />
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
