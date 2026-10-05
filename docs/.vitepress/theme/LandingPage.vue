<script setup lang="ts">
import { computed } from 'vue'
import { useData } from 'vitepress'
import TerminalDemo from './TerminalDemo.vue'
import LpHero from './landing/LpHero.vue'
import LpFrame from './landing/LpFrame.vue'
import LpStats from './landing/LpStats.vue'
import LpSection from './landing/LpSection.vue'
import LpCard from './landing/LpCard.vue'
import LpCompare from './landing/LpCompare.vue'
import LpSteps from './landing/LpSteps.vue'
import LpGallery from './landing/LpGallery.vue'
import LpFaq from './landing/LpFaq.vue'
import LpCta from './landing/LpCta.vue'
import { landingRelatedLinks } from './landing/relatedLinks'
import type { LandingData } from './landingTypes'

const { frontmatter, page } = useData()
const d = computed(() => frontmatter.value.landing as LandingData)
const related = computed(() => {
  const self = `/${page.value.relativePath.replace(/\.md$/, '')}`
  return d.value.related ?? landingRelatedLinks.filter((r) => r.link !== self)
})
</script>

<template>
  <main v-if="d" class="lp">
    <div class="lp-bg" aria-hidden="true"></div>

    <LpHero :d="d" />

    <section v-if="d.shot" class="lp-shot">
      <LpFrame :src="d.shot.src" :alt="d.shot.alt" eager />
    </section>

    <LpStats v-if="d.stats?.length" :stats="d.stats" />

    <LpSection v-if="d.cards?.length" :heading="d.cardsHeading">
      <div class="lp-cards">
        <LpCard v-for="c in d.cards" :key="c.title" :card="c" />
      </div>
    </LpSection>

    <LpSection v-if="d.terminal" :heading="d.terminal.heading" :lead="d.terminal.intro" narrow>
      <TerminalDemo :lines="d.terminal.lines" :title="d.terminal.title" :aria-label="d.terminal.ariaLabel" />
    </LpSection>

    <LpSection v-if="d.compare" :heading="d.compare.heading" :lead="d.compare.intro">
      <LpCompare :compare="d.compare" />
    </LpSection>

    <LpSection v-if="d.steps?.length" :heading="d.stepsHeading">
      <LpSteps :steps="d.steps" />
    </LpSection>

    <LpSection v-if="d.gallery?.length" :heading="d.galleryHeading">
      <LpGallery :items="d.gallery" />
    </LpSection>

    <LpSection v-if="d.prose?.length" narrow class="lp-prose">
      <template v-for="p in d.prose" :key="p.heading">
        <h2 class="lp-h2">{{ p.heading }}</h2>
        <p v-for="(para, i) in p.paragraphs" :key="i">{{ para }}</p>
      </template>
    </LpSection>

    <LpSection v-if="d.faq?.length" heading="Frequently asked questions" narrow>
      <LpFaq :items="d.faq" />
    </LpSection>

    <LpCta :heading="d.cta.heading" :sub="d.cta.sub" :primary="d.primary" />

    <nav v-if="related.length" class="lp-related" aria-label="Related pages">
      <a v-for="r in related" :key="r.link" :href="r.link">{{ r.text }}</a>
    </nav>
  </main>
</template>
