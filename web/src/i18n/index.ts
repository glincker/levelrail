import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import { lazyBackend } from './lazyBackend'
import './resources'

// NAMESPACES lists every namespace that exists under src/locales/en/, so
// a new feature area's namespace just gets added here; it does not need
// to be eagerly loaded here too, since lazyBackend fetches a namespace's
// JSON only when a component first calls useTranslation(ns) for it.
export const NAMESPACES = ['common', 'deploys'] as const

// Only English ships today (see docs/i18n.md): this is the foundation
// for future languages, not a multi-language launch. LanguageDetector is
// wired now so a real language switcher is additive later, not another
// migration.
void i18n
  .use(lazyBackend)
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    fallbackLng: 'en',
    supportedLngs: ['en'],
    ns: ['common'],
    defaultNS: 'common',
    interpolation: {
      // React already escapes rendered output.
      escapeValue: false,
    },
    react: {
      useSuspense: true,
    },
  })

export default i18n
