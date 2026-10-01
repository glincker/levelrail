import type { Theme } from 'vitepress'
import DefaultTheme from 'vitepress/theme'
import '@fontsource-variable/inter'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/500.css'
import '@fontsource/jetbrains-mono/700.css'
import '@fontsource/hanken-grotesk/800.css'
import './custom.css'
import Layout from './Layout.vue'
import Card from './Card.vue'
import CardGroup from './CardGroup.vue'

export default {
  ...DefaultTheme,
  Layout,
  enhanceApp({ app }) {
    app.component('Card', Card)
    app.component('CardGroup', CardGroup)
  },
} satisfies Theme
