import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme'
import '@fontsource-variable/plus-jakarta-sans'
import '@fontsource/lora/400.css'
import '@fontsource/lora/500.css'
import '@fontsource/lora/600.css'
import '@fontsource/lora/700.css'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/500.css'
import '@fontsource/jetbrains-mono/700.css'
// Split by concern instead of one monolithic custom.css (was ~2000
// lines). Order matters: brand.css (which pulls in tokens.css) must load
// first since every other file consumes its --vp-c-*/--glinui-* tokens.
import './styles/brand.css'
import './styles/hero.css'
import './styles/features.css'
import './styles/landing.css'
import './styles/latest-releases.css'
import './styles/faq.css'
import './styles/footer.css'
import './styles/doc-chrome.css'
import './styles/nav.css'
import './styles/sidebar.css'
import './styles/feature-tabs.css'
import './styles/accordion.css'
import './styles/steps.css'
import './styles/tabs.css'
import './styles/inline-toc.css'
import './styles/landing-tokens.css'
import './styles/landing-page.css'
import './styles/copy-command.css'
import './styles/hero-showcase.css'
import './styles/announcement-pill.css'
import Layout from './Layout.vue'
import Card from './Card.vue'
import CardGroup from './CardGroup.vue'
import LatestReleasesSection from './LatestReleasesSection.vue'
import FaqSection from './FaqSection.vue'
import FeatureTabsSection from './FeatureTabsSection.vue'
import Accordion from './Accordion.vue'
import AccordionGroup from './AccordionGroup.vue'
import Steps from './Steps.vue'
import Step from './Step.vue'
import Tabs from './Tabs.vue'
import Tab from './Tab.vue'
import InlineToc from './InlineToc.vue'
import LandingPage from './LandingPage.vue'

export default {
  ...DefaultTheme,
  Layout,
  enhanceApp({ app }) {
    app.component('Card', Card)
    app.component('CardGroup', CardGroup)
    app.component('LatestReleasesSection', LatestReleasesSection)
    app.component('FaqSection', FaqSection)
    app.component('FeatureTabsSection', FeatureTabsSection)
    app.component('Accordion', Accordion)
    app.component('AccordionGroup', AccordionGroup)
    app.component('Steps', Steps)
    app.component('Step', Step)
    app.component('Tabs', Tabs)
    app.component('Tab', Tab)
    app.component('InlineToc', InlineToc)
    app.component('landing', LandingPage)
  },
} satisfies Theme
