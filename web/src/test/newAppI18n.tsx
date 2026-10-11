import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import newAppEn from '../locales/en/newApp.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['newApp'],
  defaultNS: 'newApp',
  resources: { en: { newApp: newAppEn } },
  interpolation: { escapeValue: false },
})

/** Renders children with the real en/newApp.json catalogue. */
export function NewAppI18n({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={testI18n}>{children}</I18nextProvider>
}
