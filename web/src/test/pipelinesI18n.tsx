import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import pipelinesEn from '../locales/en/pipelines.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['pipelines'],
  defaultNS: 'pipelines',
  resources: { en: { pipelines: pipelinesEn } },
  interpolation: { escapeValue: false },
})

/** Renders children with the real en/pipelines.json catalogue. */
export function PipelinesI18n({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={testI18n}>{children}</I18nextProvider>
}
