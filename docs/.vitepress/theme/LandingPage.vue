<script setup lang="ts">
import { computed } from 'vue'
import { useData } from 'vitepress'
import {
  PhArrowRight,
  PhBell,
  PhChartLineUp,
  PhClockCounterClockwise,
  PhCube,
  PhDatabase,
  PhCurrencyDollar,
  PhEye,
  PhGauge,
  PhGithubLogo,
  PhGitBranch,
  PhGitPullRequest,
  PhGlobe,
  PhHardDrives,
  PhKey,
  PhLockKey,
  PhNetwork,
  PhPlugsConnected,
  PhRobot,
  PhScroll,
  PhTerminalWindow,
} from '@phosphor-icons/vue'
import TerminalDemo from './TerminalDemo.vue'
import type { LandingData } from './landingTypes'

const icons = {
  bell: PhBell,
  chart: PhChartLineUp,
  history: PhClockCounterClockwise,
  cube: PhCube,
  database: PhDatabase,
  dollar: PhCurrencyDollar,
  eye: PhEye,
  gauge: PhGauge,
  branch: PhGitBranch,
  pullrequest: PhGitPullRequest,
  globe: PhGlobe,
  harddrives: PhHardDrives,
  key: PhKey,
  lockkey: PhLockKey,
  network: PhNetwork,
  plugs: PhPlugsConnected,
  robot: PhRobot,
  scroll: PhScroll,
  terminal: PhTerminalWindow,
} as const
type IconKey = keyof typeof icons
const iconFor = (k?: string) => icons[(k as IconKey) in icons ? (k as IconKey) : 'cube']

const { frontmatter } = useData()
const d = computed(() => frontmatter.value.landing as LandingData)
</script>

<template>
  <main v-if="d" class="lp">
    <div class="lp-bg" aria-hidden="true"></div>
    <section class="lp-hero">
      <p v-if="d.eyebrow" class="lp-eyebrow">{{ d.eyebrow }}</p>
      <h1 class="lp-title">{{ d.headline }}</h1>
      <p class="lp-sub">{{ d.sub }}</p>
      <div class="lp-actions">
        <a class="lp-btn lp-btn--primary" :href="d.primary.link">
          {{ d.primary.text }}<PhArrowRight weight="bold" class="lp-btn__icon" />
        </a>
        <a
          v-if="d.secondary"
          class="lp-btn lp-btn--ghost"
          :href="d.secondary.link"
          :target="d.secondary.link.startsWith('http') ? '_blank' : undefined"
          rel="noreferrer"
        >
          <PhGithubLogo v-if="d.secondary.link.includes('github.com')" weight="fill" class="lp-btn__icon lp-btn__icon--lead" />
          {{ d.secondary.text }}
        </a>
      </div>
      <p v-if="d.install" class="lp-install"><code>{{ d.install }}</code></p>
    </section>

    <section v-if="d.shot" class="lp-shot">
      <div class="lp-frame">
        <img :src="d.shot.src" :alt="d.shot.alt" width="1280" height="800" loading="eager" />
      </div>
    </section>

    <section v-if="d.stats?.length" class="lp-stats">
      <div v-for="s in d.stats" :key="s.label" class="lp-stat">
        <span class="lp-stat__value">{{ s.value }}</span>
        <span class="lp-stat__label">{{ s.label }}</span>
      </div>
    </section>

    <section v-if="d.cards?.length" class="lp-section">
      <h2 v-if="d.cardsHeading" class="lp-h2">{{ d.cardsHeading }}</h2>
      <div class="lp-cards">
        <article v-for="c in d.cards" :key="c.title" class="lp-card">
          <span class="lp-card__icon"><component :is="iconFor(c.icon)" weight="duotone" :size="22" /></span>
          <div v-if="c.visual" class="lp-card__visual" aria-hidden="true">
            <div v-for="(l, i) in c.visual" :key="i" class="lp-vline" :class="`lp-vline--${l.k}`">{{ l.t }}</div>
          </div>
          <h3 class="lp-card__title">{{ c.title }}</h3>
          <p class="lp-card__body">{{ c.body }}</p>
          <a v-if="c.link" class="lp-card__link" :href="c.link.href">{{ c.link.text }}</a>
        </article>
      </div>
    </section>

    <section v-if="d.terminal" class="lp-section lp-section--narrow">
      <h2 v-if="d.terminal.heading" class="lp-h2">{{ d.terminal.heading }}</h2>
      <p v-if="d.terminal.intro" class="lp-lead">{{ d.terminal.intro }}</p>
      <TerminalDemo :lines="d.terminal.lines" :title="d.terminal.title" :aria-label="d.terminal.ariaLabel" />
    </section>

    <section v-if="d.compare" class="lp-section">
      <h2 class="lp-h2">{{ d.compare.heading }}</h2>
      <p v-if="d.compare.intro" class="lp-lead">{{ d.compare.intro }}</p>
      <div class="lp-table" role="table" :aria-label="d.compare.heading">
        <div class="lp-row lp-row--head" role="row">
          <span class="lp-cell lp-cell--label" role="columnheader"></span>
          <span class="lp-cell" role="columnheader">{{ d.compare.left }}</span>
          <span class="lp-cell lp-cell--us" role="columnheader">{{ d.compare.right }}</span>
        </div>
        <div v-for="r in d.compare.rows" :key="r.label" class="lp-row" role="row">
          <span class="lp-cell lp-cell--label" role="rowheader">{{ r.label }}</span>
          <span class="lp-cell" role="cell">{{ r.left }}</span>
          <span class="lp-cell lp-cell--us" role="cell">{{ r.right }}</span>
        </div>
      </div>
      <a v-if="d.compare.more" class="lp-more" :href="d.compare.more.href">{{ d.compare.more.text }}</a>
    </section>

    <section v-if="d.steps?.length" class="lp-section">
      <h2 v-if="d.stepsHeading" class="lp-h2">{{ d.stepsHeading }}</h2>
      <ol class="lp-steps">
        <li v-for="(s, i) in d.steps" :key="s.title" class="lp-step">
          <span class="lp-step__n">{{ i + 1 }}</span>
          <div>
            <h3 class="lp-step__title">{{ s.title }}</h3>
            <p class="lp-step__body">{{ s.body }}</p>
          </div>
        </li>
      </ol>
    </section>

    <section v-if="d.gallery?.length" class="lp-section">
      <h2 v-if="d.galleryHeading" class="lp-h2">{{ d.galleryHeading }}</h2>
      <div class="lp-gallery">
        <figure v-for="g in d.gallery" :key="g.src" class="lp-figure">
          <div class="lp-frame lp-frame--sm">
            <img :src="g.src" :alt="g.alt" width="1280" height="800" loading="lazy" />
          </div>
          <figcaption>{{ g.caption }}</figcaption>
        </figure>
      </div>
    </section>

    <section v-if="d.prose?.length" class="lp-section lp-section--narrow lp-prose">
      <template v-for="p in d.prose" :key="p.heading">
        <h2 class="lp-h2">{{ p.heading }}</h2>
        <p v-for="(para, i) in p.paragraphs" :key="i">{{ para }}</p>
      </template>
    </section>

    <section v-if="d.faq?.length" class="lp-section lp-section--narrow">
      <h2 class="lp-h2">Frequently asked questions</h2>
      <div class="lp-faq">
        <details v-for="(f, i) in d.faq" :key="f.q" class="lp-faq__item" :open="i === 0">
          <summary>{{ f.q }}</summary>
          <p>{{ f.a }}</p>
        </details>
      </div>
    </section>

    <section class="lp-cta">
      <h2 class="lp-cta__title">{{ d.cta.heading }}</h2>
      <p class="lp-cta__sub">{{ d.cta.sub }}</p>
      <div class="lp-actions">
        <a class="lp-btn lp-btn--primary" :href="d.primary.link">{{ d.primary.text }}</a>
        <a
          class="lp-btn lp-btn--ghost"
          href="https://github.com/glincker/levelrail"
          target="_blank"
          rel="noreferrer"
        ><PhGithubLogo weight="fill" class="lp-btn__icon lp-btn__icon--lead" />Star on GitHub</a>
      </div>
    </section>

    <nav v-if="d.related?.length" class="lp-related" aria-label="Related pages">
      <a v-for="r in d.related" :key="r.link" :href="r.link">{{ r.text }}</a>
    </nav>
  </main>
</template>
