import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import commonEn from '../locales/en/common.json'
import firewallEn from '../locales/en/firewall.json'
import rulesEn from '../locales/en/rules.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'firewall', 'rules'],
  defaultNS: 'common',
  resources: {
    en: { common: commonEn, firewall: firewallEn, rules: rulesEn },
  },
  interpolation: { escapeValue: false },
})

/** Renders children with the real firewall, rules and common catalogues and a fresh query client. */
export function FirewallTestProviders({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>{children}</I18nextProvider>
    </QueryClientProvider>
  )
}
