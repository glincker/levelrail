import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import { lazyBackend } from './lazyBackend'
import './resources'

// New namespace: add here only. lazyBackend loads it on first useTranslation(ns) call.
export const NAMESPACES = [
  'common',
  'deploys',
  'settings',
  'dashboard',
  'auditLog',
  'streams',
  'networkProxy',
  'databases',
  'https',
  'nodes',
  'access',
  'environments',
  'migration',
  'domains',
  'attention',
] as const

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
