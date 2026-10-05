<script setup lang="ts">
import TerminalDemo from './TerminalDemo.vue'
import HowItWorksFlow from './HowItWorksFlow.vue'
import FeatureTabsSection from './FeatureTabsSection.vue'
import LatestReleasesSection from './LatestReleasesSection.vue'
import LpSection from './landing/LpSection.vue'
import LpCompare from './landing/LpCompare.vue'
import MockCarousel from './mock/MockCarousel.vue'
import LpFaq from './landing/LpFaq.vue'
import LpCta from './landing/LpCta.vue'
import LpStatement from './landing/LpStatement.vue'
import { faqItems } from './faqData'
import { compare, switching } from './homeData'
</script>

<template>
  <div class="lp-scope home-lower">
    <LpSection
      heading="Quickstart"
      lead="The install script checks the host, installs Docker if it is missing, and starts the control plane as a systemd service."
      narrow
    >
      <TerminalDemo v-reveal />
      <p class="lp-note">
        <code>levelrail-cli deploy</code> points an app at an image and cuts traffic over only once the new container's readiness probe passes. To build from a git repository instead, connect it from the dashboard or run <code>levelrail-cli import &lt;repo-url&gt; --deploy</code>. The full walkthrough is in <a href="/getting-started">Getting started</a>.
      </p>
    </LpSection>

    <LpSection
      heading="How it works"
      lead="Four steps, the same ones the reconciler itself runs on every deploy. Select a step to see what it actually does."
    >
      <HowItWorksFlow v-reveal />
    </LpSection>

    <LpSection heading="Explore the platform">
      <FeatureTabsSection v-reveal />
    </LpSection>

    <LpSection
      :heading="compare.heading"
      lead="Many self-hosted platforms in this category manage servers by SSHing in and running docker CLI commands, then parsing the text output. That tends to force polling loops, a common source of flakiness and idle CPU use. Levelrail takes a different route."
    >
      <LpCompare :compare="compare" />
      <nav class="lp-related lp-related--left" aria-label="Switching guides">
        <span class="lp-related__label">Switching?</span>
        <a v-for="r in switching" :key="r.link" :href="r.link">{{ r.text }}</a>
      </nav>
    </LpSection>

    <LpStatement
      :lines="['Not a Kubernetes competitor.', 'Not a Vercel competitor.']"
      context="It is built for running 3 to 50 services on 1 to 10 machines without learning Kubernetes."
    />

    <LpSection heading="See it running">
      <MockCarousel v-reveal />
    </LpSection>

    <LpSection heading="Latest releases">
      <LatestReleasesSection v-reveal />
    </LpSection>

    <LpSection heading="Frequently asked questions">
      <LpFaq :items="faqItems" />
    </LpSection>

    <LpCta
      heading="Run your own platform"
      sub="Install it on one Linux server, deploy an app, and see the logs and metrics without adding anything else."
      :primary="{ text: 'Get started', link: '/getting-started' }"
    />
  </div>
</template>
