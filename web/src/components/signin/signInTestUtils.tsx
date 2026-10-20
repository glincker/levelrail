import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import signInEn from '../../locales/en/signIn.json'
import { BrandContext } from '../../lib/brandContext'
import type { Brand } from '../../types/brand'
import type { ReactNode } from 'react'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['signIn'],
  defaultNS: 'signIn',
  resources: { en: { signIn: signInEn } },
  interpolation: { escapeValue: false },
})

export function stubFetch(
  routes: Record<string, { status?: number; body?: unknown }>,
) {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string, init?: RequestInit) => {
      const key = `${init?.method ?? 'GET'} ${input}`
      calls.push(key)
      const r = routes[key]
      if (!r) {
        return Promise.resolve(new Response(null, { status: 204 }))
      }
      return Promise.resolve(
        new Response(JSON.stringify(r.body ?? {}), {
          status: r.status ?? 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
  return calls
}

export function renderWithProviders(ui: ReactNode) {
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
