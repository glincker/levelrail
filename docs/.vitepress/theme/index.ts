import DefaultTheme from 'vitepress/theme'
import '@fontsource-variable/inter'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/500.css'
import '@fontsource/jetbrains-mono/700.css'
import './custom.css'
import Layout from './Layout.vue'

export default {
  ...DefaultTheme,
  Layout,
}
