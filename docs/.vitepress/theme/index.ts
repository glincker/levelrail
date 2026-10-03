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
import Layout from './Layout.vue'
import Card from './Card.vue'
import CardGroup from './CardGroup.vue'
import LatestReleasesSection from './LatestReleasesSection.vue'
import FaqSection from './FaqSection.vue'
import FeatureTabsSection from './FeatureTabsSection.vue'

export default {
  ...DefaultTheme,
  Layout,
  enhanceApp({ app }) {
    app.component('Card', Card)
    app.component('CardGroup', CardGroup)
    app.component('LatestReleasesSection', LatestReleasesSection)
    app.component('FaqSection', FaqSection)
    app.component('FeatureTabsSection', FeatureTabsSection)
  },
} satisfies Theme
