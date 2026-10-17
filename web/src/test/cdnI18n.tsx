import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import cdnEn from '../locales/en/cdn.json'
import commonEn from '../locales/en/common.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'cdn'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, cdn: cdnEn } },
  interpolation: { escapeValue: false },
})

/** Renders children with the real en/common.json and en/cdn.json catalogues. */
export function CdnI18n({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={testI18n}>{children}</I18nextProvider>
}
