import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../locales/en/common.json'
import modelsEn from '../locales/en/models.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'models'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, models: modelsEn } },
  interpolation: { escapeValue: false },
})

/** Renders children with the real en/common.json and en/models.json catalogues. */
export function ModelsI18n({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={testI18n}>{children}</I18nextProvider>
}
