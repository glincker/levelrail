import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import type { ReactNode } from 'react'
import signInEn from '../../locales/en/signIn.json'
import updatesEn from '../../locales/en/updates.json'
import commonEn from '../../locales/en/common.json'
import { BrandContext } from '../../lib/brandContext'
import type { Brand } from '../../types/brand'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['signIn', 'updates', 'common'],
  defaultNS: 'signIn',
  resources: {
    en: { signIn: signInEn, updates: updatesEn, common: commonEn },
  },
  interpolation: { escapeValue: false },
})

export function renderCli(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const brand = { BinaryName: 'testcli' } as Brand
  return render(
    <QueryClientProvider client={client}>
      <BrandContext.Provider value={brand}>
        <I18nextProvider i18n={testI18n}>{ui}</I18nextProvider>
      </BrandContext.Provider>
    </QueryClientProvider>,
  )
}
