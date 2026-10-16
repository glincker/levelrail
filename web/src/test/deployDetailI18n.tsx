import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import deployDetailEn from '../locales/en/deployDetail.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['deployDetail'],
  defaultNS: 'deployDetail',
  resources: { en: { deployDetail: deployDetailEn } },
  interpolation: { escapeValue: false },
})

/** Renders children with the real en/deployDetail.json catalogue. */
export function DeployDetailI18n({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={testI18n}>{children}</I18nextProvider>
}
