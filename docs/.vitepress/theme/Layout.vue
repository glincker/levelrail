<script setup lang="ts">
import DefaultTheme from 'vitepress/theme'
import { useSidebar } from 'vitepress/theme'
import HeroField from './HeroField.vue'
import CustomTopNav from './CustomTopNav.vue'
import CustomSidebar from './CustomSidebar.vue'
import CustomFooter from './CustomFooter.vue'
import PageActions from './PageActions.vue'
import CopyCommand from './CopyCommand.vue'
import HeroShowcase from './HeroShowcase.vue'
import AnnouncementPill from './AnnouncementPill.vue'
import HomeFeatures from './HomeFeatures.vue'
import HomeProof from './HomeProof.vue'

const INSTALL_COMMAND = 'curl -fsSL https://levelrail.com/install.sh | sudo sh'

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
      <AnnouncementPill />
    </template>
    <template #home-hero-image>
      <HeroField />
      <HeroShowcase
        src="/assets/screenshots/app-overview.png"
        alt="A Levelrail app overview with live CPU, memory and network charts and deploy markers"
        url="levelrail.local"
      />
    </template>
    <template #home-hero-actions-after>
      <div class="hero-install">
        <CopyCommand :command="INSTALL_COMMAND" prompt="$" />
      </div>
      <ul class="trust-strip">
        <li class="trust-strip__item">
          <a href="https://github.com/glincker/levelrail/blob/main/LICENSE" target="_blank" rel="noreferrer">Free, Apache 2.0</a>
        </li>
        <li class="trust-strip__item">Pre-release</li>
        <li class="trust-strip__item">
          <a href="https://github.com/glincker/levelrail" target="_blank" rel="noreferrer">GitHub</a>
        </li>
        <li class="trust-strip__item">Secrets, metrics and logs stay on your servers</li>
      </ul>
    </template>
    <template #home-hero-after>
      <HomeProof />
      <HomeFeatures />
    </template>
    <template #layout-bottom>
      <CustomFooter />
    </template>
  </Layout>
</template>
