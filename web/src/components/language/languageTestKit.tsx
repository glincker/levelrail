import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next, { type i18n as I18n } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../../locales/en/common.json'
import commonEs from '../../locales/es/common.json'
import onboardingEn from '../../locales/en/onboarding.json'
import onboardingEs from '../../locales/es/onboarding.json'

export function makeI18n(lng = 'en'): I18n {
  const i18n = i18next.createInstance()
  void i18n.use(initReactI18next).init({
    lng,
    fallbackLng: 'en',
    supportedLngs: ['en', 'es'],
    nonExplicitSupportedLngs: true,
    ns: ['common', 'onboarding'],
    defaultNS: 'common',
    resources: {
      en: { common: commonEn, onboarding: onboardingEn },
      es: { common: commonEs, onboarding: onboardingEs },
    },
    interpolation: { escapeValue: false },
  })
  return i18n
}

export function makeWrap(i18n: I18n, qc = new QueryClient()) {
  return function Wrap({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={qc}>
        <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
      </QueryClientProvider>
    )
  }
}
