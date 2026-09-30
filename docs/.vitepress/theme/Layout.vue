<script setup lang="ts">
import DefaultTheme from 'vitepress/theme'
import { useSidebar } from 'vitepress/theme'
import HeroField from './HeroField.vue'
import CustomTopNav from './CustomTopNav.vue'
import CustomSidebar from './CustomSidebar.vue'
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
      <div class="site-footer-extra">
        <div class="site-footer-extra__inner">
          <div class="site-footer-extra__col">
            <p class="site-footer-extra__heading">Product</p>
            <a href="/getting-started">Getting started</a>
            <a href="/comparison">Compare</a>
            <a href="/roadmap">Roadmap</a>
            <a href="/troubleshooting">Troubleshooting</a>
          </div>
          <div class="site-footer-extra__col">
            <p class="site-footer-extra__heading">Reference</p>
            <a href="/app-spec-reference">App spec reference</a>
            <a href="/cli-reference">CLI reference</a>
            <a href="/api-reference">API reference</a>
            <a href="/security">Security overview</a>
          </div>
          <div class="site-footer-extra__col">
            <p class="site-footer-extra__heading">Community</p>
            <a href="https://github.com/glincker/levelrail" target="_blank" rel="noreferrer">GitHub</a>
            <a href="https://github.com/glincker/levelrail/issues" target="_blank" rel="noreferrer">Issues</a>
            <a href="https://github.com/glincker/levelrail/discussions" target="_blank" rel="noreferrer">Discussions</a>
            <a href="mailto:support@levelrail.com">support@levelrail.com</a>
          </div>
          <div class="site-footer-extra__col">
            <p class="site-footer-extra__heading">Levelrail</p>
            <p class="site-footer-extra__blurb">
              A self-hosted deployment platform. Apache 2.0.
            </p>
            <a href="https://glincker.com" target="_blank" rel="noreferrer" class="site-footer-extra__badge">
              A GLINCKER project
            </a>
            <a href="mailto:support@glincker.com">support@glincker.com</a>
          </div>
        </div>
      </div>
    </template>
  </Layout>
</template>
