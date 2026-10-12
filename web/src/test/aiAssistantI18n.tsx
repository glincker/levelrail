import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import aiAssistantEn from '../locales/en/aiAssistant.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['aiAssistant'],
  defaultNS: 'aiAssistant',
  resources: { en: { aiAssistant: aiAssistantEn } },
  interpolation: { escapeValue: false },
})

/** Renders children with the real en/aiAssistant.json catalogue. */
export function AiAssistantI18n({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={testI18n}>{children}</I18nextProvider>
}
