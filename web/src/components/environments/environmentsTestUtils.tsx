import type { ReactElement } from 'react'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import environmentsEn from '../../locales/en/environments.json'
import type { GlobalEnvironment } from '../../types/environment'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['environments'],
  defaultNS: 'environments',
  resources: { en: { environments: environmentsEn } },
  interpolation: { escapeValue: false },
})

export function env(overrides: Partial<GlobalEnvironment>): GlobalEnvironment {
  return {
    id: 'env_x',
    project_id: 'proj_global',
    name: 'Sandbox',
    protected: false,
    created_at: '2026-10-06T00:00:00Z',
    kind: 'custom',
    scope: 'global',
    sort_order: 0,
    app_count: 0,
    database_count: 0,
    ...overrides,
  }
}

export function renderWithProviders(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>{ui}</I18nextProvider>
    </QueryClientProvider>,
  )
}
