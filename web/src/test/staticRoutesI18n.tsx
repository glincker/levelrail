import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../locales/en/common.json'
import staticRoutesEn from '../locales/en/staticRoutes.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'staticRoutes'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, staticRoutes: staticRoutesEn } },
  interpolation: { escapeValue: false },
})

/** Renders children with the real en/staticRoutes.json catalogue. */
export function StaticRoutesI18n({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={testI18n}>{children}</I18nextProvider>
}
