import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import accountEn from '../../locales/en/account.json'
import commonEn from '../../locales/en/common.json'
import onboardingEn from '../../locales/en/onboarding.json'
import settingsEn from '../../locales/en/settings.json'
import uiSharedEn from '../../locales/en/uiShared.json'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common', 'onboarding', 'account', 'settings', 'uiShared'],
  defaultNS: 'common',
  resources: {
    en: {
      common: commonEn,
      onboarding: onboardingEn,
      account: accountEn,
      settings: settingsEn,
      uiShared: uiSharedEn,
    },
  },
  interpolation: { escapeValue: false },
})

/** Wrap gives a test a fresh query client and the English strings the onboarding steps read. */
export function Wrap({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
    </QueryClientProvider>
  )
}
